from typing import AsyncIterator, Dict, List

from app.agent.schemas import ToolResult
from app.core.logging import CorrelationAdapter
from app.llm.client import call_llm, stream_llm


SYNTHESIS_PROMPT = """You are an e-commerce operations and personal shopping assistant.
Use only the provided formatted summaries and conversation context.
Rules:
- Do not perform fresh calculations.
- Do not invent numbers, products, or categories.
- Never output placeholder tokens such as [number], [value], or similar brackets.
- Keep responses concise and helpful unless the user asks for deep detail.
- If a tool failed, acknowledge that clearly and suggest retrying.
"""


def _summary_block(tool_results: List[ToolResult]) -> str:
    if not tool_results:
        return "No tools were executed."
    lines: List[str] = []
    for result in tool_results:
        status = "success" if result.success else "failed"
        lines.append(f"[{result.tool} | {status}]\n{result.summary or 'No summary available.'}")
    return "\n\n".join(lines)


def _build_synthesis_messages(prompt: str, history: List[Dict[str, str]], summaries: str) -> List[Dict[str, str]]:
    return [{"role": "system", "content": SYNTHESIS_PROMPT}] + history + [
        {"role": "user", "content": prompt},
        {
            "role": "assistant",
            "content": f"Tool summaries:\n\n{summaries}",
        },
        {
            "role": "user",
            "content": "Provide the final user response based strictly on these summaries.",
        },
    ]


async def synthesize_answer(
    prompt: str,
    history: List[Dict[str, str]],
    tool_results: List[ToolResult],
    logger: CorrelationAdapter,
) -> str:
    summaries = _summary_block(tool_results)
    logger.info("Synthesizer started", extra={"event": "synthesis.start"})

    messages = _build_synthesis_messages(prompt, history, summaries)

    try:
        answer = await call_llm(messages, logger, json_mode=False)
        final = answer.strip() or "I could not generate a final response from the tool summaries."
        logger.info("Synthesizer complete", extra={"event": "synthesis.complete"})
        return final
    except Exception as exc:
        logger.error(f"Synthesis failed: {exc}", extra={"event": "synthesis.failure"}, exc_info=True)
        return summaries


async def stream_synthesize_answer(
    prompt: str,
    history: List[Dict[str, str]],
    tool_results: List[ToolResult],
    logger: CorrelationAdapter,
) -> AsyncIterator[str]:
    summaries = _summary_block(tool_results)
    logger.info("Stream synthesizer started", extra={"event": "synthesis.stream_start"})

    messages = _build_synthesis_messages(prompt, history, summaries)

    yielded_any = False
    try:
        async for chunk in stream_llm(messages, logger):
            if chunk:
                yielded_any = True
                yield chunk
    except Exception as exc:
        logger.warning(
            f"Streaming synthesis encountered an issue, falling back to summaries: {exc}",
            extra={"event": "synthesis.stream_fallback"},
        )
        if not yielded_any:
            # Yield summaries chunk by chunk if nothing was yielded yet
            for word in summaries.split(" "):
                yield word + " "
