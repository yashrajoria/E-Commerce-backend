"""Autonomous Merchant Ops Watchdog Sentry.

Runs periodic or on-demand health inspections across PostgreSQL domain tables
(payments, outbox_events, orders) to proactively identify operational anomalies.
When an anomaly is detected, it automatically creates a pending remediation
proposal in `agent_audit_log` so an admin can approve and execute with 1 click.
"""

from __future__ import annotations

import json
import time
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional
from uuid import uuid4

import asyncpg

from app.audit.repository import record_pending_mutation
from app.core.db import get_pool
from app.core.logging import logger


async def check_payment_failures(
    pool: asyncpg.Pool, threshold: int = 5
) -> Optional[Dict[str, Any]]:
    """Detects spikes in payment failures in the last 60 minutes."""
    try:
        row = await pool.fetchrow(
            """
            SELECT count(*) AS failed_count
            FROM payments
            WHERE status = 'failed'
              AND created_at > now() - interval '1 hour'
            """
        )
        failed_count = int(row["failed_count"]) if row else 0
        if failed_count >= threshold:
            return {
                "type": "payment_failure_spike",
                "severity": "high",
                "count": failed_count,
                "message": (
                    f"Payment failure spike detected: {failed_count} failed "
                    f"transactions in the last hour (threshold: {threshold})."
                ),
                "tool": "acknowledge_incident",
                "arguments": {
                    "incident_id": f"pay-spike-{int(time.time())}",
                    "action_taken": "triage_payment_failures",
                    "notes": f"Observed {failed_count} failures in the last hour.",
                },
            }
    except Exception as exc:
        logger.warning(f"Watchdog payment check skipped or failed: {exc}")
    return None


async def check_stuck_outbox(
    pool: asyncpg.Pool, max_age_minutes: int = 5
) -> Optional[Dict[str, Any]]:
    """Detects unpublished outbox events older than max_age_minutes."""
    try:
        row = await pool.fetchrow(
            """
            SELECT count(*) AS stuck_count
            FROM outbox_events
            WHERE published = false
              AND created_at < now() - make_interval(mins => $1)
            """,
            max_age_minutes,
        )
        stuck_count = int(row["stuck_count"]) if row else 0
        if stuck_count > 0:
            return {
                "type": "stuck_outbox_events",
                "severity": "medium",
                "count": stuck_count,
                "message": (
                    f"Transactional outbox lag: {stuck_count} events pending older "
                    f"than {max_age_minutes} minutes. Downstream consumers may be stalled."
                ),
                "tool": "redrive_stuck_outbox",
                "arguments": {
                    "batch_size": min(stuck_count, 100),
                },
            }
    except Exception as exc:
        logger.warning(f"Watchdog outbox check skipped or failed: {exc}")
    return None


async def _has_active_pending_mutation(pool: asyncpg.Pool, tool: str) -> bool:
    """Prevents duplicate spam if an identical pending mutation is already awaiting admin review."""
    row = await pool.fetchrow(
        """
        SELECT id FROM agent_audit_log
        WHERE mutating = true
          AND status = 'pending_confirmation'
          AND tool = $1
        LIMIT 1
        """,
        tool,
    )
    return row is not None


async def run_watchdog_scan(
    *,
    payment_failure_threshold: int = 5,
    outbox_max_age_minutes: int = 5,
    custom_pool: Optional[asyncpg.Pool] = None,
) -> Dict[str, Any]:
    """Executes a full operational watchdog health scan.

    Finds operational anomalies, deduplicates against existing pending proposals,
    and creates new rows in `agent_audit_log` with status='pending_confirmation'.
    """
    pool = custom_pool or (await get_pool())
    anomalies: List[Dict[str, Any]] = []

    # 1. Run checks
    p_check = await check_payment_failures(pool, threshold=payment_failure_threshold)
    if p_check:
        anomalies.append(p_check)

    o_check = await check_stuck_outbox(pool, max_age_minutes=outbox_max_age_minutes)
    if o_check:
        anomalies.append(o_check)

    created_incidents: List[Dict[str, Any]] = []

    # 2. Record new proposals into agent_audit_log
    for anomaly in anomalies:
        tool_name = anomaly["tool"]
        is_duplicate = await _has_active_pending_mutation(pool, tool_name)
        if not is_duplicate:
            req_id = uuid4()
            prompt = f"[OPS WATCHDOG] {anomaly['message']}"
            await record_pending_mutation(
                request_id=req_id,
                user_id=None,
                prompt=prompt,
                tool=tool_name,
                arguments=anomaly["arguments"],
            )
            created_incidents.append(
                {
                    "request_id": str(req_id),
                    "type": anomaly["type"],
                    "severity": anomaly["severity"],
                    "tool": tool_name,
                    "prompt": prompt,
                    "arguments": anomaly["arguments"],
                }
            )
            logger.info(
                f"Ops Watchdog created incident proposal: {tool_name} (req={req_id})"
            )

    return {
        "scanned_at": datetime.now(timezone.utc).isoformat(),
        "anomalies_detected": len(anomalies),
        "incidents_created": len(created_incidents),
        "details": anomalies,
        "proposals": created_incidents,
    }
