import pytest
from app.agent.streaming import (
    format_sse,
    sse_status,
    sse_tool_start,
    sse_tool_result,
    sse_token,
    sse_action_card,
    sse_done,
    sse_error,
)
from app.agent.orchestrator import run_agent_stream
from app.core.logging import get_logger


def test_sse_formatters():
    status = sse_status("planning", "Thinking...")
    assert status.startswith("event: status\ndata: ")
    assert "planning" in status

    token = sse_token("Hello ")
    assert token.startswith("event: token\ndata: ")
    assert "Hello " in token

    action_card = sse_action_card({"type": "bundle", "title": "Test Bundle"})
    assert action_card.startswith("event: action_card\ndata: ")
    assert "Test Bundle" in action_card

    done = sse_done("sess-1", "corr-1", "Done answer")
    assert done.startswith("event: done\ndata: ")
    assert "Done answer" in done

    err = sse_error("Something went wrong")
    assert err.startswith("event: error\ndata: ")
    assert "Something went wrong" in err


@pytest.mark.asyncio
async def test_run_agent_stream_generator():
    logger = get_logger("test-corr-id")
    events = []
    async for chunk in run_agent_stream(
        prompt="how many products do we have?",
        session_id="test-session-stream",
        auth_header=None,
        cookie_header=None,
        user_id=None,
        user_role="guest",
        logger=logger,
        correlation_id="test-corr-id",
    ):
        events.append(chunk)

    assert len(events) >= 2
    # First event should be planning status
    assert "event: status" in events[0]
    # At least one event should be done
    assert any("event: done" in ev for ev in events)
