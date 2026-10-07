from typing import Any, AsyncIterator, Dict, List, Optional, Tuple
from uuid import uuid4

from app.agent.executor import execute_concurrent
from app.agent.formatter import format_all_results
from app.agent.intent import map_intent
from app.agent.schemas import ToolResult
from app.agent.streaming import (
    sse_action_card,
    sse_done,
    sse_error,
    sse_status,
    sse_steps,
    sse_token,
    sse_tool_result,
    sse_tool_start,
)
from app.agent.synthesizer import stream_synthesize_answer, synthesize_answer
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
        prompt=prompt,
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

    action_card = None
    steps: List[str] = []
    for r in formatted_results:
        if r.tool == "build_bundle" and r.success and isinstance(r.data, dict):
            bundle_data = r.data
            action_card = {
                "type": "bundle",
                "title": bundle_data.get("title") or "Curated Essentials Bundle",
                "theme": bundle_data.get("theme"),
                "budget_cents": bundle_data.get("budget_cents"),
                "bundle_price_cents": bundle_data.get("final_price_cents"),
                "original_price_cents": bundle_data.get("subtotal_cents"),
                "discount_cents": bundle_data.get("discount_cents"),
                "coupon_code": bundle_data.get("coupon_code"),
                "savings_cents": bundle_data.get("savings_cents"),
                "items": bundle_data.get("items", []),
            }
            steps = list(bundle_data.get("steps_taken", []))
            break
        elif r.tool == "get_best_coupon" and r.success and isinstance(r.data, dict) and not steps:
            coupon_data = r.data
            code = coupon_data.get("best_coupon")
            disc = coupon_data.get("discount_cents", 0)
            steps.append(f"🏷️ Verified active promo `{code}` for ${(disc / 100):.2f} discount")

    response = {
        "success": success,
        "answer": answer,
        "tool_results": formatted_results,
        "tools_called": [r.tool for r in formatted_results],
        "session_id": session_id,
        "correlation_id": cid,
        "error": "; ".join(errors) if errors else None,
        "action_card": action_card,
        "steps": steps,
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


async def run_agent_stream(
    prompt: str,
    session_id: str,
    auth_header: Optional[str],
    cookie_header: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    logger: CorrelationAdapter,
    correlation_id: Optional[str] = None,
) -> AsyncIterator[str]:
    cid = correlation_id or logger.extra.get("correlation_id") or str(uuid4())
    history = get_history(session_id)

    logger.info("Streaming pipeline started", extra={"event": "pipeline.stream_start"})
    yield sse_status("planning", "Analyzing query and planning actions...")

    try:
        tool_calls = await map_intent(prompt, history, logger)

        if tool_calls:
            for call in tool_calls:
                yield sse_tool_start(call.tool, call.params)

            yield sse_status("executing", f"Executing {len(tool_calls)} operations...")
            tool_results = await execute_concurrent(
                tool_calls=tool_calls,
                auth_header=auth_header,
                cookie_header=cookie_header,
                user_id=user_id,
                user_role=user_role,
                logger=logger,
                prompt=prompt,
            )
            formatted_results = format_all_results(tool_results)
            for res in formatted_results:
                yield sse_tool_result(res.tool, res.success, res.summary)
        else:
            formatted_results = []

        action_card = None
        steps: List[str] = []
        for r in formatted_results:
            if r.tool == "build_bundle" and r.success and isinstance(r.data, dict):
                bundle_data = r.data
                action_card = {
                    "type": "bundle",
                    "title": bundle_data.get("title") or "Curated Essentials Bundle",
                    "theme": bundle_data.get("theme"),
                    "budget_cents": bundle_data.get("budget_cents"),
                    "bundle_price_cents": bundle_data.get("final_price_cents"),
                    "original_price_cents": bundle_data.get("subtotal_cents"),
                    "discount_cents": bundle_data.get("discount_cents"),
                    "coupon_code": bundle_data.get("coupon_code"),
                    "savings_cents": bundle_data.get("savings_cents"),
                    "items": bundle_data.get("items", []),
                }
                steps = list(bundle_data.get("steps_taken", []))
                yield sse_action_card(action_card)
                if steps:
                    yield sse_steps(steps)
                break
            elif r.tool == "get_best_coupon" and r.success and isinstance(r.data, dict) and not steps:
                coupon_data = r.data
                code = coupon_data.get("best_coupon")
                disc = coupon_data.get("discount_cents", 0)
                step_text = f"🏷️ Verified active promo `{code}` for ${(disc / 100):.2f} discount"
                steps.append(step_text)
                yield sse_steps(steps)

        yield sse_status("synthesizing", "Generating answer...")
        collected_tokens: List[str] = []

        async for token_chunk in stream_synthesize_answer(prompt, history, formatted_results, logger):
            collected_tokens.append(token_chunk)
            yield sse_token(token_chunk)

        full_answer = "".join(collected_tokens).strip() or _fallback_answer(formatted_results)

        push_history(session_id, "user", prompt)
        push_history(session_id, "assistant", full_answer)

        yield sse_done(session_id, cid, full_answer, action_card)
        logger.info("Streaming pipeline complete", extra={"event": "pipeline.stream_complete"})

    except Exception as exc:
        logger.error(
            f"Streaming pipeline crash: {exc}",
            extra={"event": "pipeline.stream_failure"},
            exc_info=True,
        )
        yield sse_error(f"Failed to process query: {str(exc)}")

