from __future__ import annotations

from typing import Any, Callable, Coroutine, Dict, Optional

import httpx

from app.core.logging import logger
from app.tools.handlers.base import (
    close_http_client,
    get_http_client,
    _headers,
    _request,
    _request_with_fallback,
    _extract_data,
)
from app.tools.handlers.bundle import (
    build_bundle,
    get_best_coupon,
)
from app.tools.handlers.catalog import (
    check_compatibility,
    get_low_stock,
    get_product_count,
    get_top_products,
    search_products,
)
from app.tools.handlers.ops import (
    acknowledge_incident,
    create_restock_request,
    get_customers,
    get_failed_payments,
    redrive_stuck_outbox,
)
from app.tools.handlers.orders import (
    cancel_order,
    get_orders,
    get_revenue_breakdown,
    get_sales,
    track_order,
)

__all__ = [
    "get_http_client",
    "close_http_client",
    "_headers",
    "_request",
    "_request_with_fallback",
    "_extract_data",
    "get_product_count",
    "get_top_products",
    "get_low_stock",
    "get_sales",
    "get_failed_payments",
    "get_orders",
    "get_customers",
    "get_revenue_breakdown",
    "search_products",
    "cancel_order",
    "create_restock_request",
    "redrive_stuck_outbox",
    "acknowledge_incident",
    "build_bundle",
    "get_best_coupon",
    "check_compatibility",
    "track_order",
    "execute_tool",
]

_ToolHandler = Callable[..., Coroutine[Any, Any, Dict[str, Any]]]


async def execute_tool(
    tool_name: str,
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
    store_id: Optional[str] = None,
) -> Dict[str, Any]:
    if store_id and "store_id" not in params:
        params["store_id"] = store_id

    # Deferred import: app.tools.registry imports this module's handler
    # functions at module load time, so importing it back at module scope
    # here would be circular. TOOL_REGISTRY is the single source of truth
    # for tool -> handler mapping (see app/tools/registry.py).
    from app.tools.registry import TOOL_REGISTRY

    spec = TOOL_REGISTRY.get(tool_name)
    handler = spec.handler if spec else None
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
