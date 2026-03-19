from __future__ import annotations

import time
from datetime import datetime, timedelta, timezone
from typing import Any, Callable, Coroutine, Dict, Optional

import httpx

from app.core.config import settings
from app.core.logging import logger


_TIMEOUT = settings.BFF_TIMEOUT
_API_BASE = (settings.TOOL_API_BASE_URL or "http://api-gateway:8080").rstrip("/")

_http_client: Optional[httpx.AsyncClient] = None


async def get_http_client() -> httpx.AsyncClient:
    global _http_client
    if _http_client is None:
        _http_client = httpx.AsyncClient(timeout=_TIMEOUT, follow_redirects=True)
    return _http_client


async def close_http_client() -> None:
    global _http_client
    if _http_client is not None:
        await _http_client.aclose()
        _http_client = None


def _headers(
    auth_header: Optional[str],
    cookie_header: Optional[str],
    correlation_id: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
) -> Dict[str, str]:
    headers: Dict[str, str] = {"Content-Type": "application/json"}
    if auth_header:
        headers["Authorization"] = auth_header
    if cookie_header:
        headers["Cookie"] = cookie_header
    if correlation_id:
        headers["X-Correlation-ID"] = correlation_id
    if user_id:
        headers["X-User-ID"] = user_id
    if user_role:
        headers["X-User-Role"] = user_role
    return headers


async def _request(
    method: str,
    path: str,
    *,
    params: Optional[Dict[str, Any]] = None,
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Any:
    client = await get_http_client()
    url = f"{_API_BASE}{path}"
    req_params = {k: v for k, v in (params or {}).items() if v is not None}

    start = time.perf_counter()
    response = await client.request(
        method=method,
        url=url,
        params=req_params,
        headers=_headers(auth_header, cookie_header, correlation_id, user_id, user_role),
    )
    duration_ms = int((time.perf_counter() - start) * 1000)

    logger.info(
        f"API call {method} {path}",
        extra={
            "event": "api.call",
            "path": path,
            "status_code": response.status_code,
            "duration_ms": duration_ms,
            "correlation_id": correlation_id or "N/A",
        },
    )

    response.raise_for_status()
    body = response.json()
    size = len(response.content)
    logger.info(
        f"API response size={size}",
        extra={"event": "api.response", "path": path, "correlation_id": correlation_id or "N/A"},
    )
    return body


async def _request_with_fallback(
    method: str,
    paths: list[str],
    *,
    params: Optional[Dict[str, Any]] = None,
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Any:
    last_exc: Optional[Exception] = None
    for path in paths:
        try:
            return await _request(
                method,
                path,
                params=params,
                auth_header=auth_header,
                cookie_header=cookie_header,
                correlation_id=correlation_id,
                user_id=user_id,
                user_role=user_role,
            )
        except httpx.HTTPStatusError as exc:
            last_exc = exc
            if exc.response.status_code == 404:
                continue
            raise
    if last_exc:
        raise last_exc
    raise RuntimeError("No endpoint paths were provided")


def _extract_data(payload: Any) -> Any:
    if isinstance(payload, dict) and "data" in payload:
        return payload["data"]
    return payload


async def get_product_count(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    products_payload = await _request(
        "GET",
        "/bff/admin/products",
        params={"page": 1, "page_size": 100},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    categories_payload = await _request(
        "GET",
        "/bff/admin/categories",
        params={"page": 1, "page_size": 200},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )

    data = _extract_data(products_payload)
    categories_data = _extract_data(categories_payload)

    if not isinstance(data, dict):
        return {"total_products": 0, "category_breakdown": {}}

    products = data.get("products", []) if isinstance(data.get("products", []), list) else []
    meta = data.get("meta", {}) if isinstance(data.get("meta", {}), dict) else {}

    if isinstance(categories_data, list):
        category_list = categories_data
    elif isinstance(categories_data, dict):
        category_list = categories_data.get("categories", [])
    else:
        category_list = []
    category_map: Dict[str, str] = {}
    for category in category_list:
        if isinstance(category, dict) and category.get("_id"):
            category_map[str(category.get("_id"))] = str(category.get("name") or "Unknown")

    breakdown: Dict[str, int] = {}
    for product in products:
        if not isinstance(product, dict):
            continue
        ids = product.get("category_ids", [])
        if isinstance(ids, list) and ids:
            for cid in ids:
                name = category_map.get(str(cid), "Unknown")
                breakdown[name] = breakdown.get(name, 0) + 1
        else:
            cat = product.get("category")
            if isinstance(cat, dict) and cat.get("name"):
                name = str(cat.get("name"))
            else:
                name = "Unknown"
            breakdown[name] = breakdown.get(name, 0) + 1

    total = int(meta.get("total") or len(products))
    return {"total_products": total, "category_breakdown": breakdown}


async def get_top_products(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    limit = int(params.get("limit", 5))
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
    products = data.get("topProducts", []) if isinstance(data, dict) else []
    if not isinstance(products, list):
        products = []
    return {"limit": limit, "products": products}


async def get_low_stock(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    threshold = int(params.get("threshold", 10))
    payload = await _request(
        "GET",
        "/bff/admin/reports/inventory",
        params={},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _extract_data(payload)
    if isinstance(data, dict):
        raw_items = data.get("items") or data.get("products") or data.get("inventory") or []
        items = [item for item in raw_items if isinstance(item, dict) and _stock(item) < threshold]
    else:
        items = data if isinstance(data, list) else []
    return {"threshold": threshold, "count": len(items), "items": items}


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


def _stock(item: Dict[str, Any]) -> int:
    for key in ("stock", "quantity", "current_stock", "inventory", "available"):
        value = item.get(key)
        if isinstance(value, (int, float)):
            return int(value)
    return 0


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


async def search_products(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    query = params.get("query", "")
    payload = await _request(
        "GET",
        "/bff/admin/products",
        params={"page": 1, "page_size": 100},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    data = _extract_data(payload)
    products = data.get("products", []) if isinstance(data, dict) else []
    if not query:
        return data if isinstance(data, dict) else {"products": products}

    q = str(query).lower().strip()
    filtered = []
    for product in products:
        if not isinstance(product, dict):
            continue
        name = str(product.get("name", "")).lower()
        sku = str(product.get("sku", "")).lower()
        if q in name or q in sku:
            filtered.append(product)

    if isinstance(data, dict):
        data = dict(data)
        data["products"] = filtered
        return data
    return {"products": filtered}


_ToolHandler = Callable[..., Coroutine[Any, Any, Dict[str, Any]]]

_TOOL_HANDLERS: Dict[str, _ToolHandler] = {
    "get_sales": get_sales,
    "get_top_products": get_top_products,
    "get_low_stock": get_low_stock,
    "get_failed_payments": get_failed_payments,
    "get_orders": get_orders,
    "get_customers": get_customers,
    "get_revenue_breakdown": get_revenue_breakdown,
    "search_products": search_products,
    "get_product_count": get_product_count,
}


async def execute_tool(
    tool_name: str,
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    handler = _TOOL_HANDLERS.get(tool_name)
    if not handler:
        return {
            "tool": tool_name,
            "success": False,
            "data": None,
            "error": f"Unknown tool: {tool_name}",
        }

    try:
        data = await handler(
            params=params,
            auth_header=auth_header,
            cookie_header=cookie_header,
            correlation_id=correlation_id,
            user_id=user_id,
            user_role=user_role,
        )
        return {"tool": tool_name, "success": True, "data": data, "error": None}
    except httpx.HTTPStatusError as exc:
        error = f"Client error '{exc.response.status_code}' for url '{exc.request.url}'"
        logger.error(
            f"Tool API status error: {tool_name} | {error}",
            extra={
                "event": "api.error",
                "tool": tool_name,
                "status_code": exc.response.status_code,
                "correlation_id": correlation_id or "N/A",
            },
            exc_info=True,
        )
        return {"tool": tool_name, "success": False, "data": None, "error": error}
    except httpx.RequestError as exc:
        error = f"Network error while calling backend API: {exc}"
        logger.error(
            f"Tool API request error: {tool_name} | {error}",
            extra={"event": "api.error", "tool": tool_name, "correlation_id": correlation_id or "N/A"},
            exc_info=True,
        )
        return {"tool": tool_name, "success": False, "data": None, "error": error}
    except Exception as exc:
        error = str(exc)
        logger.error(
            f"Tool execution exception: {tool_name} | {error}",
            extra={"event": "tool.exception", "tool": tool_name, "correlation_id": correlation_id or "N/A"},
            exc_info=True,
        )
        return {"tool": tool_name, "success": False, "data": None, "error": error}
