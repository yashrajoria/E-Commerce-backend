from __future__ import annotations

from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional

from app.tools.handlers.base import _extract_data, _request


def _created_at(order: Dict[str, Any]) -> Optional[datetime]:
    raw = order.get("CreatedAt") or order.get("created_at")
    if not raw or not isinstance(raw, str):
        return None
    try:
        return datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except ValueError:
        return None


def _range_to_days(value: Any) -> Optional[int]:
    if isinstance(value, int):
        return value if value > 0 else None
    if not isinstance(value, str):
        return None
    text = value.strip().lower()
    if text.endswith("d") and text[:-1].isdigit():
        days = int(text[:-1])
        return days if days > 0 else None
    return None


async def get_sales(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    range_ = params.get("range", "30d")
    payload = await _request(
        "GET",
        "/bff/admin/reports/sales",
        params={"range": range_},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _extract_data(payload)
    if isinstance(data, dict):
        return {
            "range": range_,
            "revenue": data.get("revenue") or data.get("total_revenue") or 0,
            "orders": data.get("orders") or data.get("order_count") or 0,
        }
    return {"range": range_, "revenue": 0, "orders": 0}


async def get_orders(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    range_days = _range_to_days(params.get("days")) or _range_to_days(params.get("range"))
    payload = await _request(
        "GET",
        "/bff/admin/orders",
        params={
            "status": params.get("status"),
            "page": params.get("page", 1),
            "page_size": params.get("limit", 20),
        },
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _extract_data(payload)
    if not isinstance(data, dict):
        return data

    orders = data.get("orders", [])
    if not isinstance(orders, list):
        orders = []

    if range_days is not None:
        cutoff = datetime.now(timezone.utc) - timedelta(days=range_days)
        filtered_orders = []
        for order in orders:
            if not isinstance(order, dict):
                continue
            created = _created_at(order)
            if created is not None and created >= cutoff:
                filtered_orders.append(order)
    else:
        filtered_orders = [order for order in orders if isinstance(order, dict)]

    meta = data.get("meta") if isinstance(data.get("meta"), dict) else {}
    normalized_meta = dict(meta)
    normalized_meta["total_orders"] = len(filtered_orders)
    normalized_meta["page"] = int(params.get("page", 1) or 1)
    normalized_meta["limit"] = int(params.get("limit", len(filtered_orders) or 20) or 20)
    normalized_meta["total_pages"] = 1
    normalized_meta["has_more"] = False
    if range_days is not None:
        normalized_meta["range"] = f"{range_days}d"

    normalized = dict(data)
    normalized["orders"] = filtered_orders
    normalized["meta"] = normalized_meta
    return normalized


async def get_revenue_breakdown(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    payload = await _request(
        "GET",
        "/bff/admin/reports/sales",
        params={
            "group_by": params.get("group_by", "category"),
            "range": params.get("range", "30d"),
        },
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _extract_data(payload)


async def cancel_order(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """Mutating tool: cancels an order via the admin cancel endpoint."""
    order_id = params["order_id"]
    body: Dict[str, Any] = {}
    if params.get("reason"):
        body["reason"] = params["reason"]

    payload = await _request(
        "PUT",
        f"/bff/admin/orders/{order_id}/cancel",
        json_body=body,
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _extract_data(payload)


async def track_order(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    order_id = params.get("order_id")
    if order_id:
        res = await _request(
            "GET",
            f"/orders/{order_id}",
            auth_header=auth_header,
            cookie_header=cookie_header,
            correlation_id=correlation_id,
            user_id=user_id,
            user_role=user_role,
        )
        data = _extract_data(res)
        return {"order": data, "summary": f"Order {order_id} status retrieved."}

    res = await _request(
        "GET",
        "/orders",
        params={"page": 1, "limit": 5},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _extract_data(res)
    orders = data.get("orders", []) if isinstance(data, dict) else (data if isinstance(data, list) else [])
    if not orders:
        return {"orders": [], "summary": "No recent orders found for this account."}

    latest = orders[0]
    oid = latest.get("id") or latest.get("_id") or "recent"
    status = latest.get("status", "processing")
    return {
        "latest_order": latest,
        "recent_count": len(orders),
        "summary": f"Your latest order #{oid} is currently {status}.",
    }

