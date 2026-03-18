"""
Async tool execution layer for the ShopSwift agent service.

Each tool maps to a BFF endpoint and is called directly (not via the gateway)
to avoid circular routing. Auth headers from the original admin request are
forwarded transparently so the BFF can authorise the call.

Tool registry (must stay in sync with TOOL_REGISTRY in agent_service.py):
    get_sales               → GET  /bff/admin/reports/sales
    get_top_products        → GET  /bff/admin/dashboard        (extracts topProducts)
    get_low_stock           → GET  /bff/admin/reports/inventory
    get_failed_payments     → GET  /bff/admin/dashboard        (extracts recentActivity)
    get_orders              → GET  /bff/admin/reports/orders
    get_customers           → GET  /bff/admin/reports/customers
    get_revenue_breakdown   → GET  /bff/admin/reports/revenue
    search_products         → GET  /bff/admin/products
"""

from __future__ import annotations

import logging
from typing import Any, Callable, Coroutine, Dict, Optional

import httpx

from config import settings

logger = logging.getLogger(__name__)

# ── HTTP client ───────────────────────────────────────────────────────────────
# Direct connection to BFF — NOT through the gateway.
# Routing through the gateway would create a circular call:
#   agent-service → api-gateway → agent-service → ...
# BFF_BASE_URL must be set to "http://bff-service:8088" in config/env.
_BFF_BASE = (settings.BFF_BASE_URL or "http://bff-service:8088").rstrip("/")
_TIMEOUT = settings.BFF_TIMEOUT

_http_client: Optional[httpx.AsyncClient] = None


async def get_http_client() -> httpx.AsyncClient:
    """Return (or lazily create) the singleton connection-pooled HTTP client."""
    global _http_client
    if _http_client is None:
        _http_client = httpx.AsyncClient(
            timeout=_TIMEOUT,
            # Raise immediately on 4xx/5xx so callers get a clear exception.
            # Individual tools call response.raise_for_status() explicitly
            # so we leave follow_redirects on and keep the default.
            follow_redirects=True,
        )
    return _http_client


async def close_http_client() -> None:
    """Gracefully close the singleton client on application shutdown."""
    global _http_client
    if _http_client is not None:
        await _http_client.aclose()
        _http_client = None


# ── Tool dispatcher ───────────────────────────────────────────────────────────

# Type alias for a tool handler coroutine
_ToolHandler = Callable[..., Coroutine[Any, Any, Dict[str, Any]]]

_TOOL_HANDLERS: Dict[str, _ToolHandler] = {}  # populated below after definitions


async def execute_tool(
    tool_name: str,
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Route a tool call to its handler and wrap the result in a standard envelope.

    Returns a dict always containing:
        tool    : str   — tool name
        success : bool
        data    : Any   — payload on success, None on failure
        error   : str | None
    """
    handler = _TOOL_HANDLERS.get(tool_name)
    if not handler:
        logger.warning(f"Unknown tool requested: {tool_name}")
        return _err(tool_name, f"Unknown tool: {tool_name}")

    try:
        data = await handler(
            params,
            auth_header=auth_header,
            cookie_header=cookie_header,
            correlation_id=correlation_id,
            user_id=user_id,
            user_role=user_role,
        )
        return {"tool": tool_name, "success": True, "data": data, "error": None}
    except httpx.HTTPStatusError as exc:
        msg = (
            f"BFF returned {exc.response.status_code} for "
            f"{exc.request.method} {exc.request.url}"
        )
        logger.error(f"[{tool_name}] {msg}")
        return _err(tool_name, msg)
    except httpx.RequestError as exc:
        msg = f"Network error reaching BFF: {exc}"
        logger.error(f"[{tool_name}] {msg}")
        return _err(tool_name, msg)
    except Exception as exc:
        logger.error(f"[{tool_name}] Unexpected error: {exc}", exc_info=True)
        return _err(tool_name, str(exc))


def _err(tool: str, message: str) -> Dict[str, Any]:
    return {"tool": tool, "success": False, "data": None, "error": message}


# ── Shared helpers ────────────────────────────────────────────────────────────

def _headers(
    auth_header: Optional[str],
    cookie_header: Optional[str],
    correlation_id: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
) -> Dict[str, str]:
    """Build the forwarded-auth + tracing header dict."""
    h: Dict[str, str] = {"Content-Type": "application/json"}
    if auth_header:
        h["Authorization"] = auth_header
    if cookie_header:
        h["Cookie"] = cookie_header
    if correlation_id:
        h["X-Correlation-ID"] = correlation_id
    if user_id:
        h["X-User-ID"] = user_id
    if user_role:
        h["X-User-Role"] = user_role
    return h


async def _get(
    path: str,
    params: Optional[Dict[str, Any]] = None,
    **header_kwargs: Any,
) -> Any:
    """
    Perform a GET request to the BFF, raise on HTTP errors, return parsed JSON.
    `header_kwargs` are forwarded to _headers().
    """
    client = await get_http_client()
    response = await client.get(
        f"{_BFF_BASE}{path}",
        params={k: v for k, v in (params or {}).items() if v is not None},
        headers=_headers(**header_kwargs),
    )
    response.raise_for_status()
    return response.json()


def _data(payload: Any) -> Any:
    """Unwrap a {"data": ...} envelope if present."""
    if isinstance(payload, dict):
        return payload.get("data", payload)
    return payload


# ── Tool implementations ──────────────────────────────────────────────────────

async def get_sales(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch sales KPIs from the BFF.
    Params: range (str) — "7d" | "30d" | "1y"
    """
    range_ = params.get("range", "30d")
    payload = await _get(
        "/bff/admin/reports/sales",
        params={"range": range_},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _data(payload)


async def get_top_products(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch top-selling products via the admin dashboard endpoint.
    Params: range (str), limit (int)
    """
    limit = int(params.get("limit", 5))
    range_ = params.get("range", "30d")
    payload = await _get(
        "/bff/admin/dashboard",
        params={"range": range_},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _data(payload)
    top_products = (data.get("topProducts", []) if isinstance(data, dict) else [])[:limit]
    return {"range": range_, "limit": limit, "products": top_products}


async def get_low_stock(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    List products whose stock is at or below the threshold.
    Params: threshold (int, default 10)
    """
    threshold = int(params.get("threshold", 10))
    payload = await _get(
        "/bff/admin/reports/inventory",
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _data(payload)
    raw_items: list = []
    if isinstance(data, dict):
        raw_items = data.get("items") or data.get("inventory") or data.get("products") or []
    elif isinstance(data, list):
        raw_items = data

    low = [
        item for item in raw_items
        if isinstance(item, dict) and _stock_value(item) <= threshold
    ]
    return {"threshold": threshold, "count": len(low), "items": low}


def _stock_value(item: dict) -> int:
    """Extract the stock integer from various field-name conventions."""
    for key in ("current_stock", "stock", "quantity", "inventory"):
        val = item.get(key)
        if isinstance(val, (int, float)):
            return int(val)
    return 0


async def get_failed_payments(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Return failed payment events derived from the admin dashboard activity feed.
    No params required.
    """
    payload = await _get(
        "/bff/admin/dashboard",
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _data(payload)
    activities = (data.get("recentActivity", []) if isinstance(data, dict) else [])
    failed = [
        a for a in activities
        if isinstance(a, dict) and _is_failed_payment(a)
    ]
    return {"count": len(failed), "payments": failed}


def _is_failed_payment(activity: dict) -> bool:
    text = f"{activity.get('type', '')} {activity.get('description', '')}".lower()
    return "payment" in text and (
        "fail" in text or "declin" in text or activity.get("variant") == "error"
    )


async def get_orders(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch order list with optional status filter.
    Params: status (str), range (str), limit (int)
    """
    payload = await _get(
        "/bff/admin/reports/orders",
        params={
            "status": params.get("status", "all"),
            "range":  params.get("range", "30d"),
            "limit":  params.get("limit", 20),
        },
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _data(payload)


async def get_customers(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch customer statistics.
    Params: range (str), metric ("new" | "returning" | "all")
    """
    payload = await _get(
        "/bff/admin/reports/customers",
        params={
            "range":  params.get("range", "30d"),
            "metric": params.get("metric", "all"),
        },
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _data(payload)


async def get_revenue_breakdown(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Break down revenue by category, channel, or region.
    Params: group_by (str), range (str)
    """
    payload = await _get(
        "/bff/admin/reports/revenue",
        params={
            "group_by": params.get("group_by", "category"),
            "range":    params.get("range", "30d"),
        },
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _data(payload)


async def search_products(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Search / list products by name, SKU, or category.
    Params: query (str)
    """
    payload = await _get(
        "/bff/admin/products",
        params={"search": params.get("query", ""), "q": params.get("query", "")},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    return _data(payload)



async def get_product_count(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Return total product count and category breakdown.
    Calls the admin products list endpoint and extracts pagination totals.
    No params required.
    """
    payload = await _get(
        "/bff/admin/products",
        params={"page": 1, "limit": 1},  # minimal data, we only need the total
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _data(payload)
    # BFF typically returns {"total": N, "products": [...], "page": 1}
    total = None
    if isinstance(data, dict):
        total = (
            data.get("total")
            or data.get("count")
            or data.get("totalCount")
            or data.get("total_count")
        )
    return {"total_products": total, "raw": data}

# ── Register all handlers (must come after function definitions) ──────────────
_TOOL_HANDLERS = {
    "get_sales":             get_sales,
    "get_top_products":      get_top_products,
    "get_low_stock":         get_low_stock,
    "get_failed_payments":   get_failed_payments,
    "get_orders":            get_orders,
    "get_customers":         get_customers,
    "get_revenue_breakdown": get_revenue_breakdown,
    "search_products":       search_products,
    "get_product_count":     get_product_count,
}