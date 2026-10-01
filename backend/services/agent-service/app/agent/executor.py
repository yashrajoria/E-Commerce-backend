import asyncio
import time
from typing import List, Optional
from uuid import uuid4

from app.agent.formatter import format_tool_result
from app.agent.schemas import ToolCall, ToolResult
from app.audit.repository import record_pending_mutation, record_tool_call
from app.core.config import settings
from app.core.logging import CorrelationAdapter
from app.core.security import can_execute_tool
from app.tools.executor import execute_tool
from app.tools.registry import TOOL_REGISTRY

async def execute_concurrent(
    tool_calls: List[ToolCall],
    auth_header: Optional[str],
    cookie_header: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    logger: CorrelationAdapter,
    prompt: str = "",
) -> List[ToolResult]:
    if not tool_calls:
        return []

    async def _safe_execute(call: ToolCall) -> ToolResult:
        if not can_execute_tool(call.tool, user_role, logger):
            result = ToolResult(tool=call.tool, success=False, error="Unauthorized")
            result.summary = format_tool_result(result)
            return result

        spec = TOOL_REGISTRY.get(call.tool)
        if spec is not None and spec.mutating:
            return await _propose_mutation(call, user_id, prompt, logger)

        started = time.perf_counter()
        logger.info(
            f"Tool start: {call.tool}",
            extra={"event": "tool.start", "tool": call.tool},
        )
        try:
            raw_res = await asyncio.wait_for(
                execute_tool(call.tool, call.params, auth_header, cookie_header, logger.extra.get('correlation_id'), user_id, user_role),
                timeout=settings.TOOL_TIMEOUT
            )
            raw_res.setdefault("tool", call.tool)
            result = ToolResult(**raw_res)
        except asyncio.TimeoutError:
            result = ToolResult(tool=call.tool, success=False, error="Execution timed out")
        except Exception as e:
            result = ToolResult(tool=call.tool, success=False, error=f"Execution error: {repr(e)}")

        result.summary = format_tool_result(result)
        duration_ms = int((time.perf_counter() - started) * 1000)
        logger.info(
            f"Tool end: {call.tool} success={result.success}",
            extra={
                "event": "tool.end",
                "tool": call.tool,
                "duration_ms": duration_ms,
            },
        )
        if not result.success:
            logger.error(
                f"Tool failed: {call.tool} | {result.error}",
                extra={"event": "tool.failure", "tool": call.tool},
            )

        try:
            await record_tool_call(
                request_id=uuid4(),
                user_id=user_id,
                prompt=prompt,
                tool=call.tool,
                arguments=call.params,
                mutating=False,
                status="completed" if result.success else "failed",
                result=result.data,
                error=result.error,
            )
        except Exception as audit_exc:
            logger.error(
                f"Audit write failed for {call.tool}: {audit_exc}",
                extra={"event": "audit.failure", "tool": call.tool},
            )

        return result

    async def _propose_mutation(call: ToolCall, user_id: Optional[str], prompt: str, logger: CorrelationAdapter) -> ToolResult:
        request_id = uuid4()
        try:
            await record_pending_mutation(
                request_id=request_id,
                user_id=user_id,
                prompt=prompt,
                tool=call.tool,
                arguments=call.params,
            )
        except Exception as audit_exc:
            logger.error(
                f"Audit write failed proposing mutation {call.tool}: {audit_exc}",
                extra={"event": "audit.failure", "tool": call.tool},
            )
            result = ToolResult(tool=call.tool, success=False, error="Failed to record mutation proposal")
            result.summary = format_tool_result(result)
            return result

        result = ToolResult(
            tool=call.tool,
            success=True,
            data={
                "requires_confirmation": True,
                "request_id": str(request_id),
                "tool": call.tool,
                "arguments": call.params,
            },
        )
        result.summary = (
            f"This action ({call.tool}) requires admin confirmation. "
            f"Approve or reject via POST /agent/mutations/{request_id}/confirm."
        )
        logger.info(
            f"Mutation proposed: {call.tool} | request_id={request_id}",
            extra={"event": "mutation.proposed", "tool": call.tool},
        )
        return result

    tasks = [_safe_execute(call) for call in tool_calls]
    gathered = await asyncio.gather(*tasks, return_exceptions=True)

    results: List[ToolResult] = []
    for i, item in enumerate(gathered):
        if isinstance(item, Exception):
            call = tool_calls[i]
            failed = ToolResult(tool=call.tool, success=False, error=f"Execution error: {repr(item)}")
            failed.summary = format_tool_result(failed)
            results.append(failed)
            logger.error(
                f"Tool task raised exception: {call.tool}",
                extra={"event": "tool.exception", "tool": call.tool},
            )
        else:
            results.append(item)

    successes = sum(1 for r in results if r.success)
    logger.info(
        f"Executor complete | {successes}/{len(results)} successful",
        extra={"event": "executor.complete"},
    )

    return results
