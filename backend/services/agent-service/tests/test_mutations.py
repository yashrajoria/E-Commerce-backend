import logging
from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.agent.executor import execute_concurrent
from app.agent.schemas import ToolCall
import app.api.routes as routes


class _FakeRequest:
    def __init__(self, headers=None, correlation_id="cid-1"):
        self.headers = headers or {}
        self.state = SimpleNamespace(correlation_id=correlation_id)


def _admin_headers(user_id="00000000-0000-0000-0000-000000000001"):
    return {"x-user-role": "admin", "x-user-id": user_id, "authorization": "Bearer t"}


@pytest.mark.asyncio
async def test_propose_mutation_creates_pending_row_without_calling_handler(monkeypatch):
    recorded = {}

    async def fake_record_pending_mutation(**kwargs):
        recorded.update(kwargs)

    monkeypatch.setattr("app.agent.executor.record_pending_mutation", fake_record_pending_mutation)

    handler_called = False

    async def fake_cancel_order(**kwargs):
        nonlocal handler_called
        handler_called = True
        return {}

    fake_registry = {"cancel_order": SimpleNamespace(mutating=True, min_role="admin", handler=fake_cancel_order)}
    monkeypatch.setattr("app.agent.executor.TOOL_REGISTRY", fake_registry)
    monkeypatch.setattr("app.agent.executor.can_execute_tool", lambda tool, role, logger: True)

    calls = [ToolCall(tool="cancel_order", params={"order_id": "o1", "reason": None})]
    results = await execute_concurrent(
        tool_calls=calls,
        auth_header="Bearer t",
        cookie_header=None,
        user_id="u1",
        user_role="admin",
        logger=logging.getLogger("test"),
        prompt="cancel order o1",
    )

    assert len(results) == 1
    assert results[0].success is True
    assert results[0].data["requires_confirmation"] is True
    assert recorded["tool"] == "cancel_order"
    assert handler_called is False


@pytest.mark.asyncio
async def test_confirm_approve_invokes_handler_once_and_marks_executed(monkeypatch):
    request_id = uuid4()
    row = {
        "status": "pending_confirmation",
        "tool": "cancel_order",
        "arguments": '{"order_id": "o1", "reason": null}',
    }

    async def fake_get_pending(rid):
        return row

    calls_made = []

    async def fake_handler(**kwargs):
        calls_made.append(kwargs)
        return {"order_id": "o1", "status": "cancelled"}

    executed = {}

    async def fake_mark_confirmed_and_executed(rid, confirmed_by, result, error):
        executed.update(rid=rid, confirmed_by=confirmed_by, result=result, error=error)

    monkeypatch.setattr(routes, "get_pending", fake_get_pending)
    monkeypatch.setattr(routes, "mark_confirmed_and_executed", fake_mark_confirmed_and_executed)
    monkeypatch.setattr(routes, "TOOL_REGISTRY", {"cancel_order": SimpleNamespace(handler=fake_handler)})

    req = _FakeRequest(headers=_admin_headers())
    body = routes.MutationConfirmRequest(approve=True)

    response = await routes.confirm_mutation(str(request_id), body, req)

    assert response["status"] == "executed"
    assert len(calls_made) == 1
    assert executed["error"] is None


@pytest.mark.asyncio
async def test_confirm_reject_never_invokes_handler(monkeypatch):
    request_id = uuid4()
    row = {"status": "pending_confirmation", "tool": "cancel_order", "arguments": "{}"}

    async def fake_get_pending(rid):
        return row

    handler_called = False

    async def fake_handler(**kwargs):
        nonlocal handler_called
        handler_called = True
        return {}

    rejected = {}

    async def fake_mark_rejected(rid, confirmed_by):
        rejected.update(rid=rid, confirmed_by=confirmed_by)

    monkeypatch.setattr(routes, "get_pending", fake_get_pending)
    monkeypatch.setattr(routes, "mark_rejected", fake_mark_rejected)
    monkeypatch.setattr(routes, "TOOL_REGISTRY", {"cancel_order": SimpleNamespace(handler=fake_handler)})

    req = _FakeRequest(headers=_admin_headers())
    body = routes.MutationConfirmRequest(approve=False)

    response = await routes.confirm_mutation(str(request_id), body, req)

    assert response["status"] == "rejected"
    assert handler_called is False
    assert rejected["rid"] == request_id


@pytest.mark.asyncio
async def test_confirm_on_already_terminal_row_returns_409(monkeypatch):
    request_id = uuid4()
    row = {"status": "executed", "tool": "cancel_order", "arguments": "{}"}

    async def fake_get_pending(rid):
        return row

    monkeypatch.setattr(routes, "get_pending", fake_get_pending)

    req = _FakeRequest(headers=_admin_headers())
    body = routes.MutationConfirmRequest(approve=True)

    with pytest.raises(Exception) as exc_info:
        await routes.confirm_mutation(str(request_id), body, req)
    assert "409" in str(exc_info.value) or getattr(exc_info.value, "status_code", None) == 409


@pytest.mark.asyncio
async def test_confirm_requires_admin_role():
    req = _FakeRequest(headers={"x-user-role": "customer"})
    body = routes.MutationConfirmRequest(approve=True)

    with pytest.raises(Exception) as exc_info:
        await routes.confirm_mutation(str(uuid4()), body, req)
    assert getattr(exc_info.value, "status_code", None) == 403
