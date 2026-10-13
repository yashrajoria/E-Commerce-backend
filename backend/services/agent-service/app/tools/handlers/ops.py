from __future__ import annotations

from datetime import datetime, timezone
from typing import Any, Dict, Optional

from app.tools.handlers.base import _extract_data, _request


async def get_failed_payments(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    payload = await _request(
        "GET",
        "/bff/admin/dashboard",
        params={},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _extract_data(payload)
    activities = data.get("recentActivity", []) if isinstance(data, dict) else []
    payments = []
    for item in activities:
        if not isinstance(item, dict):
            continue
        text = f"{item.get('type', '')} {item.get('description', '')}".lower()
        if "payment" in text and ("fail" in text or "declin" in text or item.get("variant") == "error"):
            payments.append(item)
    return {"count": len(payments), "payments": payments}


async def get_customers(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    payload = await _request(
        "GET",
        "/bff/admin/reports/users",
        params={
            "range": params.get("range", "30d"),
            "metric": params.get("metric", "all"),
        },
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _extract_data(payload)


async def create_restock_request(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """Mutating tool: increases a product's available stock."""
    product_id = params["product_id"]
    quantity = params["quantity"]

    current = await _request(
        "GET",
        f"/inventory/{product_id}",
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    current_available = 0
    if isinstance(current, dict):
        current_available = int(current.get("available") or current.get("Available") or 0)

    new_available = current_available + quantity
    return await _request(
        "PUT",
        f"/inventory/{product_id}",
        json_body={"available": new_available},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )


async def redrive_stuck_outbox(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    batch_size = int(params.get("batch_size") or 50)
    return await _request(
        "POST",
        "/orders/admin/outbox/redrive",
        json_body={"batch_size": batch_size},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )


async def acknowledge_incident(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    incident_id = params.get("incident_id")
    action_taken = params.get("action_taken", "acknowledged")
    notes = params.get("notes")
    return {
        "status": "incident_resolved",
        "incident_id": incident_id,
        "action_taken": action_taken,
        "notes": notes,
        "acknowledged_by": user_id or "admin",
        "timestamp": datetime.now(timezone.utc).isoformat(),
    }

