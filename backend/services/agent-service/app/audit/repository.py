"""asyncpg-backed audit trail for every tool invocation the agent proposes.

One row per tool call. Read tools land straight in `completed`/`failed`. A
mutating tool starts at `pending_confirmation` and the same row is updated
in place through `confirmed`/`rejected`/`executed`/`failed` — this table is
both the audit log and the pending-mutation store (single source of truth,
per ROADMAP.md Phase 2).
"""

import json
from typing import Any, Dict, Optional
from uuid import UUID

import asyncpg

from app.core.db import get_pool


async def record_tool_call(
    *,
    request_id: UUID,
    user_id: Optional[str],
    prompt: str,
    tool: str,
    arguments: Dict[str, Any],
    mutating: bool,
    status: str,
    result: Optional[Any] = None,
    error: Optional[str] = None,
) -> None:
    pool = await get_pool()
    await pool.execute(
        """
        INSERT INTO agent_audit_log
            (id, request_id, user_id, prompt, tool, arguments, mutating, status, result, error)
        VALUES ($1, $1, $2, $3, $4, $5, $6, $7, $8, $9)
        """,
        request_id,
        _to_uuid_or_none(user_id),
        prompt,
        tool,
        json.dumps(arguments),
        mutating,
        status,
        json.dumps(result) if result is not None else None,
        error,
    )


async def record_pending_mutation(
    *,
    request_id: UUID,
    user_id: Optional[str],
    prompt: str,
    tool: str,
    arguments: Dict[str, Any],
) -> None:
    await record_tool_call(
        request_id=request_id,
        user_id=user_id,
        prompt=prompt,
        tool=tool,
        arguments=arguments,
        mutating=True,
        status="pending_confirmation",
    )


async def get_pending(request_id: UUID) -> Optional[asyncpg.Record]:
    pool = await get_pool()
    return await pool.fetchrow(
        "SELECT * FROM agent_audit_log WHERE request_id = $1 AND mutating = true",
        request_id,
    )


async def mark_rejected(request_id: UUID, confirmed_by: Optional[str]) -> None:
    pool = await get_pool()
    await pool.execute(
        """
        UPDATE agent_audit_log
        SET status = 'rejected', confirmed_by = $2, confirmed_at = now(), updated_at = now()
        WHERE request_id = $1
        """,
        request_id,
        _to_uuid_or_none(confirmed_by),
    )


async def mark_confirmed_and_executed(
    request_id: UUID,
    confirmed_by: Optional[str],
    result: Optional[Any],
    error: Optional[str],
) -> None:
    pool = await get_pool()
    status = "failed" if error else "executed"
    await pool.execute(
        """
        UPDATE agent_audit_log
        SET status = $2, confirmed_by = $3, confirmed_at = now(),
            result = $4, error = $5, updated_at = now()
        WHERE request_id = $1
        """,
        request_id,
        status,
        _to_uuid_or_none(confirmed_by),
        json.dumps(result) if result is not None else None,
        error,
    )


def _to_uuid_or_none(value: Optional[str]) -> Optional[UUID]:
    if not value:
        return None
    try:
        return UUID(value)
    except (ValueError, AttributeError):
        return None
