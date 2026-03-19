from typing import Any, Dict, List, Optional, Tuple
from uuid import uuid4

from app.agent.executor import execute_concurrent
from app.agent.formatter import format_all_results
from app.agent.intent import map_intent
from app.agent.schemas import ToolResult
from app.agent.synthesizer import synthesize_answer
from app.core.logging import CorrelationAdapter
from app.core.session import get_history, push_history


def _fallback_answer(results: List[ToolResult]) -> str:
    summaries = [r.summary for r in results if r.summary]
    if summaries:
        return "\n\n".join(summaries)
    return "I could not complete the request right now. Please try again."


async def run_agent(
    prompt: str,
    session_id: str,
    auth_header: Optional[str],
    cookie_header: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    logger: CorrelationAdapter,
    correlation_id: Optional[str] = None,
) -> Dict[str, Any]:
    cid = correlation_id or logger.extra.get("correlation_id") or str(uuid4())
    history = get_history(session_id)

    logger.info("Pipeline started", extra={"event": "pipeline.start"})

    tool_calls = await map_intent(prompt, history, logger)
    tool_results = await execute_concurrent(
        tool_calls=tool_calls,
        auth_header=auth_header,
        cookie_header=cookie_header,
        user_id=user_id,
        user_role=user_role,
        logger=logger,
    )
    formatted_results = format_all_results(tool_results)

    try:
        answer = await synthesize_answer(prompt, history, formatted_results, logger)
    except Exception as exc:
        logger.error(
            f"Synthesis crashed, returning formatter fallback: {exc}",
            extra={"event": "pipeline.synthesis_fallback"},
            exc_info=True,
        )
        answer = _fallback_answer(formatted_results)

    push_history(session_id, "user", prompt)
    push_history(session_id, "assistant", answer)

    errors = [r.error for r in formatted_results if r.error]
    success = all(r.success for r in formatted_results) if formatted_results else True

    response = {
        "success": success,
        "answer": answer,
        "tool_results": formatted_results,
        "tools_called": [r.tool for r in formatted_results],
        "session_id": session_id,
        "correlation_id": cid,
        "error": "; ".join(errors) if errors else None,
    }

    logger.info(
        "Pipeline complete",
        extra={"event": "pipeline.complete"},
    )
    return response


async def run_agent_workflow(
    prompt: str,
    session_id: str,
    auth_header: Optional[str],
    cookie_header: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    logger: CorrelationAdapter,
) -> Tuple[str, List[ToolResult]]:
    response = await run_agent(
        prompt=prompt,
        session_id=session_id,
        auth_header=auth_header,
        cookie_header=cookie_header,
        user_id=user_id,
        user_role=user_role,
        logger=logger,
        correlation_id=logger.extra.get("correlation_id"),
    )
    return response["answer"], response["tool_results"]
