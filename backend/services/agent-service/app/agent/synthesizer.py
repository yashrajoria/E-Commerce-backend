from typing import Dict, List

from app.agent.schemas import ToolResult
from app.core.logging import CorrelationAdapter
from app.llm.client import call_ollama


SYNTHESIS_PROMPT = """You are an e-commerce operations assistant.
Use only the provided formatted summaries and conversation context.
Rules:
- Do not perform fresh calculations.
- Do not invent numbers, products, or categories.
- Never output placeholder tokens such as [number], [value], or similar brackets.
- Keep responses under 200 words unless the user asks for deep detail.
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


async def synthesize_answer(
    prompt: str,
    history: List[Dict[str, str]],
    tool_results: List[ToolResult],
    logger: CorrelationAdapter,
) -> str:
    summaries = _summary_block(tool_results)
    logger.info("Synthesizer started", extra={"event": "synthesis.start"})

    messages = [{"role": "system", "content": SYNTHESIS_PROMPT}] + history + [
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

    try:
        answer = await call_ollama(messages, logger, json_mode=False)
        final = answer.strip() or "I could not generate a final response from the tool summaries."
        logger.info("Synthesizer complete", extra={"event": "synthesis.complete"})
        return final
    except Exception as exc:
        logger.error(f"Synthesis failed: {exc}", extra={"event": "synthesis.failure"}, exc_info=True)
        return summaries
