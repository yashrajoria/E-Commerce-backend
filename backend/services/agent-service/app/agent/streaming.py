import json
from typing import Any, Dict, Optional


def format_sse(event: str, data: Any) -> str:
    """Format data payload as a Server-Sent Event (SSE)."""
    payload_str = json.dumps(data) if not isinstance(data, str) else json.dumps({"text": data})
    return f"event: {event}\ndata: {payload_str}\n\n"


def sse_status(step: str, message: str) -> str:
    return format_sse("status", {"step": step, "message": message})


def sse_tool_start(tool: str, params: Optional[Dict[str, Any]] = None) -> str:
    return format_sse("tool_start", {"tool": tool, "params": params or {}})


def sse_tool_result(tool: str, success: bool, summary: Optional[str] = None) -> str:
    return format_sse("tool_result", {"tool": tool, "success": success, "summary": summary or ""})


def sse_token(delta: str) -> str:
    return format_sse("token", {"delta": delta})


def sse_action_card(card: Dict[str, Any]) -> str:
    return format_sse("action_card", card)


def sse_steps(steps: list[str]) -> str:
    return format_sse("steps", {"steps": steps})


def sse_done(session_id: str, correlation_id: str, full_answer: str, action_card: Optional[Dict[str, Any]] = None) -> str:
    return format_sse(
        "done",
        {
            "session_id": session_id,
            "correlation_id": correlation_id,
            "answer": full_answer,
            "action_card": action_card,
        },
    )


def sse_error(error_message: str) -> str:
    return format_sse("error", {"error": error_message})
