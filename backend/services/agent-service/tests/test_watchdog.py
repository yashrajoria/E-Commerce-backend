from __future__ import annotations

import logging
from types import SimpleNamespace
from uuid import uuid4

import pytest

from app.watchdog.sentry import (
    check_payment_failures,
    check_stuck_outbox,
    run_watchdog_scan,
)
import app.api.routes as routes


class _FakeRequest:
    def __init__(self, headers=None, correlation_id="cid-watchdog-1"):
        self.headers = headers or {}
        self.state = SimpleNamespace(correlation_id=correlation_id)


def _admin_headers(user_id="00000000-0000-0000-0000-000000000001"):
    return {"x-user-role": "admin", "x-user-id": user_id, "authorization": "Bearer t"}


class _FakePool:
    def __init__(self, fetchrow_map=None):
        self.fetchrow_map = fetchrow_map or {}
        self.executed_queries = []

    async def fetchrow(self, query, *args):
        for k, v in self.fetchrow_map.items():
            if k in query:
                return v
        return None

    async def execute(self, query, *args):
        self.executed_queries.append((query, args))


@pytest.mark.asyncio
async def test_check_payment_failures_detects_anomaly():
    pool = _FakePool(fetchrow_map={"FROM payments": {"failed_count": 14}})
    res = await check_payment_failures(pool, threshold=5)
    assert res is not None
    assert res["type"] == "payment_failure_spike"
    assert res["count"] == 14
    assert res["tool"] == "acknowledge_incident"
    assert res["severity"] == "high"


@pytest.mark.asyncio
async def test_check_payment_failures_below_threshold():
    pool = _FakePool(fetchrow_map={"FROM payments": {"failed_count": 2}})
    res = await check_payment_failures(pool, threshold=5)
    assert res is None


@pytest.mark.asyncio
async def test_check_stuck_outbox_detects_lag():
    pool = _FakePool(fetchrow_map={"FROM outbox_events": {"stuck_count": 7}})
    res = await check_stuck_outbox(pool, max_age_minutes=5)
    assert res is not None
    assert res["type"] == "stuck_outbox_events"
    assert res["count"] == 7
    assert res["tool"] == "redrive_stuck_outbox"


@pytest.mark.asyncio
async def test_run_watchdog_scan_creates_proposals_and_deduplicates(monkeypatch):
    recorded_proposals = []

    async def fake_record_pending_mutation(**kwargs):
        recorded_proposals.append(kwargs)

    monkeypatch.setattr(
        "app.watchdog.sentry.record_pending_mutation", fake_record_pending_mutation
    )

    fake_pool = _FakePool(
        fetchrow_map={
            "FROM payments": {"failed_count": 8},
            "FROM outbox_events": {"stuck_count": 5},
            "SELECT id FROM agent_audit_log": None,  # no duplicate currently
        }
    )

    result = await run_watchdog_scan(custom_pool=fake_pool)
    assert result["anomalies_detected"] == 2
    assert result["incidents_created"] == 2
    assert len(recorded_proposals) == 2
    tools = {p["tool"] for p in recorded_proposals}
    assert "acknowledge_incident" in tools
    assert "redrive_stuck_outbox" in tools


@pytest.mark.asyncio
async def test_watchdog_deduplication_prevents_duplicate_proposals(monkeypatch):
    recorded_proposals = []

    async def fake_record_pending_mutation(**kwargs):
        recorded_proposals.append(kwargs)

    monkeypatch.setattr(
        "app.watchdog.sentry.record_pending_mutation", fake_record_pending_mutation
    )

    # When active proposal already exists in audit log:
    fake_pool = _FakePool(
        fetchrow_map={
            "FROM payments": {"failed_count": 8},
            "FROM outbox_events": {"stuck_count": 5},
            "SELECT id FROM agent_audit_log": {"id": uuid4()},  # already pending!
        }
    )

    result = await run_watchdog_scan(custom_pool=fake_pool)
    assert result["anomalies_detected"] == 2
    assert result["incidents_created"] == 0
    assert len(recorded_proposals) == 0


@pytest.mark.asyncio
async def test_api_list_pending_mutations(monkeypatch):
    fake_row = {
        "id": uuid4(),
        "request_id": uuid4(),
        "user_id": None,
        "prompt": "[OPS WATCHDOG] payment alert",
        "tool": "acknowledge_incident",
        "arguments": '{"incident_id": "pay-1"}',
        "mutating": True,
        "status": "pending_confirmation",
        "created_at": None,
        "updated_at": None,
        "confirmed_at": None,
    }

    async def fake_list_pending(limit=50):
        return [fake_row]

    monkeypatch.setattr("app.api.routes.list_pending_mutations", fake_list_pending)

    req = _FakeRequest(headers=_admin_headers())
    res = await routes.list_pending(req, limit=10)
    assert len(res) == 1
    assert res[0]["tool"] == "acknowledge_incident"
    assert res[0]["status"] == "pending_confirmation"
