"""
Async tool execution layer for invoking BFF endpoints.
Uses httpx.AsyncClient to make concurrent requests with auth and tracing headers.
"""

import httpx
import logging
from typing import Dict, Any, Optional

from config import settings

logger = logging.getLogger(__name__)

# BFF service configuration
BFF_BASE_URL = settings.BFF_BASE_URL
REQUEST_TIMEOUT = settings.BFF_TIMEOUT

# Singleton HTTP client for connection pooling
_http_client: Optional[httpx.AsyncClient] = None


async def get_http_client() -> httpx.AsyncClient:
    """
    Get or create the singleton HTTP client for connection pooling.
    
    This ensures connection reuse across multiple requests, improving performance
    by avoiding the overhead of creating new sessions for each request.
    """
    global _http_client
    if _http_client is None:
        _http_client = httpx.AsyncClient(timeout=REQUEST_TIMEOUT)
    return _http_client


async def close_http_client() -> None:
    """
    Close the singleton HTTP client.
    
    Should be called during application shutdown to gracefully close connections.
    """
    global _http_client
    if _http_client is not None:
        await _http_client.aclose()
        _http_client = None


async def execute_tool(
    tool_name: str,
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Route tool calls to the appropriate handler.
    
    Args:
        tool_name: Name of the tool to execute
        params: Parameters for the tool
        auth_header: JWT Authorization header value
        correlation_id: X-Correlation-ID for tracing
        
    Returns:
        Result dictionary with success status, data, and error.
    """
    tool_handlers = {
        "get_sales": get_sales,
        "get_top_products": get_top_products,
        "get_low_stock": get_low_stock,
        "get_failed_payments": get_failed_payments,
    }
    
    handler = tool_handlers.get(tool_name)
    if not handler:
        logger.warning(f"Unknown tool requested: {tool_name}")
        return {
            "tool": tool_name,
            "success": False,
            "data": None,
            "error": f"Unknown tool: {tool_name}",
        }
    
    try:
        result = await handler(params, auth_header, correlation_id, user_id, user_role)
        return {
            "tool": tool_name,
            "success": True,
            "data": result,
            "error": None,
        }
    except Exception as e:
        logger.error(f"Tool execution failed for {tool_name}: {str(e)}")
        return {
            "tool": tool_name,
            "success": False,
            "data": None,
            "error": str(e),
        }


async def get_sales(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch sales data from the BFF.
    
    Params expected:
        - range: String like "7d", "30d", "1y"
    """
    headers = _build_headers(auth_header, correlation_id, user_id, user_role)
    client = await get_http_client()
    
    response = await client.get(
        f"{BFF_BASE_URL}/bff/admin/reports/sales",
        headers=headers,
    )
    response.raise_for_status()
    return response.json()


async def get_top_products(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch top-selling products from the BFF.
    
    Params expected:
        - range: String like "7d", "30d", "1y"
        - limit: Number of products to return (default: 5)
    """
    limit = params.get("limit", 5)
    
    headers = _build_headers(auth_header, correlation_id, user_id, user_role)
    client = await get_http_client()
    
    # Current BFF exposes top products via aggregated admin dashboard payload.
    response = await client.get(
        f"{BFF_BASE_URL}/bff/admin/dashboard",
        headers=headers,
    )
    response.raise_for_status()
    payload = response.json()
    data = payload.get("data", payload)
    top_products = data.get("topProducts", []) if isinstance(data, dict) else []
    return {
        "limit": limit,
        "products": top_products[:limit],
    }


async def get_low_stock(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch low-stock inventory items from the BFF.
    
    Params expected:
        - threshold: Integer threshold for low stock
    """
    threshold = int(params.get("threshold", 10))

    headers = _build_headers(auth_header, correlation_id, user_id, user_role)
    client = await get_http_client()
    
    response = await client.get(
        f"{BFF_BASE_URL}/bff/admin/reports/inventory",
        headers=headers,
    )
    response.raise_for_status()
    payload = response.json()
    data = payload.get("data", payload)

    items = []
    if isinstance(data, dict):
        raw_items = data.get("items") or data.get("inventory") or []
        if isinstance(raw_items, list):
            for item in raw_items:
                if not isinstance(item, dict):
                    continue
                stock = item.get("current_stock", item.get("stock", item.get("quantity")))
                if isinstance(stock, int) and stock <= threshold:
                    items.append(item)

    return {
        "threshold": threshold,
        "items": items,
        "raw": data,
    }


async def get_failed_payments(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch failed payment transactions from the BFF.
    
    No parameters required.
    """
    headers = _build_headers(auth_header, correlation_id, user_id, user_role)
    client = await get_http_client()
    
    # Current BFF does not expose a dedicated failed-payments admin endpoint.
    # Derive likely failed payment events from dashboard recent activity.
    response = await client.get(
        f"{BFF_BASE_URL}/bff/admin/dashboard",
        headers=headers,
    )
    response.raise_for_status()
    payload = response.json()
    data = payload.get("data", payload)
    activities = data.get("recentActivity", []) if isinstance(data, dict) else []

    failed = []
    for activity in activities:
        if not isinstance(activity, dict):
            continue
        text = f"{activity.get('type', '')} {activity.get('description', '')}".lower()
        if "payment" in text and ("fail" in text or "declin" in text or activity.get("variant") == "error"):
            failed.append(activity)

    return {
        "count": len(failed),
        "payments": failed,
    }


def _build_headers(
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, str]:
    """
    Build HTTP headers for proxying auth and tracing information.
    
    Acts as a transparent proxy, forwarding the original JWT and correlation ID.
    """
    headers = {
        "Content-Type": "application/json",
    }
    
    if auth_header:
        headers["Authorization"] = auth_header
    
    if correlation_id:
        headers["X-Correlation-ID"] = correlation_id

    if user_id:
        headers["X-User-ID"] = user_id

    if user_role:
        headers["X-User-Role"] = user_role
    
    return headers
