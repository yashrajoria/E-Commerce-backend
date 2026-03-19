import asyncio
import time
from typing import List, Optional

from app.agent.formatter import format_tool_result
from app.agent.schemas import ToolCall, ToolResult
from app.core.config import settings
from app.core.logging import CorrelationAdapter
from app.core.security import can_execute_tool
from app.tools.executor import execute_tool

async def execute_concurrent(
    tool_calls: List[ToolCall],
    auth_header: Optional[str],
    cookie_header: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    logger: CorrelationAdapter
) -> List[ToolResult]:
    if not tool_calls:
        return []

    async def _safe_execute(call: ToolCall) -> ToolResult:
        if not can_execute_tool(call.tool, user_role, logger):
            result = ToolResult(tool=call.tool, success=False, error="Unauthorized")
            result.summary = format_tool_result(result)
            return result

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
