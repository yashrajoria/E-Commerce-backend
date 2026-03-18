"""
Async tool execution layer for invoking BFF endpoints.
Uses httpx.AsyncClient to make concurrent requests with auth and tracing headers.
"""

import httpx
import logging
from typing import Dict, Any, Optional
from datetime import datetime

logger = logging.getLogger(__name__)

# BFF service configuration
BFF_BASE_URL = "http://bff-service:8080"
REQUEST_TIMEOUT = 15.0

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
        result = await handler(params, auth_header, correlation_id)
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
) -> Dict[str, Any]:
    """
    Fetch sales data from the BFF.
    
    Params expected:
        - range: String like "7d", "30d", "1y"
    """
    range_val = params.get("range", "7d")
    
    headers = _build_headers(auth_header, correlation_id)
    client = await get_http_client()
    
    response = await client.get(
        f"{BFF_BASE_URL}/admin/sales",
        params={"range": range_val},
        headers=headers,
    )
    response.raise_for_status()
    return response.json()


async def get_top_products(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch top-selling products from the BFF.
    
    Params expected:
        - range: String like "7d", "30d", "1y"
        - limit: Number of products to return (default: 5)
    """
    range_val = params.get("range", "7d")
    limit = params.get("limit", 5)
    
    headers = _build_headers(auth_header, correlation_id)
    client = await get_http_client()
    
    response = await client.get(
        f"{BFF_BASE_URL}/admin/top-products",
        params={"range": range_val, "limit": limit},
        headers=headers,
    )
    response.raise_for_status()
    return response.json()


async def get_low_stock(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch low-stock inventory items from the BFF.
    
    Params expected:
        - threshold: Integer threshold for low stock
    """
    threshold = params.get("threshold", 10)
    
    headers = _build_headers(auth_header, correlation_id)
    client = await get_http_client()
    
    response = await client.get(
        f"{BFF_BASE_URL}/admin/low-stock",
        params={"threshold": threshold},
        headers=headers,
    )
    response.raise_for_status()
    return response.json()


async def get_failed_payments(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Fetch failed payment transactions from the BFF.
    
    No parameters required.
    """
    headers = _build_headers(auth_header, correlation_id)
    client = await get_http_client()
    
    response = await client.get(
        f"{BFF_BASE_URL}/admin/failed-payments",
        headers=headers,
    )
    response.raise_for_status()
    return response.json()


def _build_headers(
    auth_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
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
    
    return headers
