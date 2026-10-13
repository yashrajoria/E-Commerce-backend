from __future__ import annotations

import asyncio
import time
from typing import Any, Dict, List, Optional

import httpx

from app.core.config import settings
from app.core.logging import logger

_TIMEOUT = settings.BFF_TIMEOUT
_API_BASE = (settings.TOOL_API_BASE_URL or "http://api-gateway:8080").rstrip("/")

_http_client: Optional[httpx.AsyncClient] = None
_http_client_lock = asyncio.Lock()


async def get_http_client() -> httpx.AsyncClient:
    global _http_client
    async with _http_client_lock:
        if _http_client is None:
            _http_client = httpx.AsyncClient(
                timeout=httpx.Timeout(_TIMEOUT, connect=min(_TIMEOUT, 5.0)),
                follow_redirects=False,
                trust_env=False,
            )
            # OpenTelemetry — no-op when telemetry disabled.
            from app.core.telemetry import instrument_httpx_client
            instrument_httpx_client(_http_client)
        return _http_client


async def close_http_client() -> None:
    global _http_client
    async with _http_client_lock:
        if _http_client is not None:
            await _http_client.aclose()
            _http_client = None


def _headers(
    auth_header: Optional[str],
    cookie_header: Optional[str],
    correlation_id: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    store_id: Optional[str] = None,
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
    if store_id:
        headers["X-Store-ID"] = store_id
    return headers


async def _request(
    method: str,
    path: str,
    *,
    params: Optional[Dict[str, Any]] = None,
    json_body: Optional[Dict[str, Any]] = None,
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
    store_id: Optional[str] = None,
) -> Any:
    client = await get_http_client()
    url = f"{_API_BASE}{path}"
    req_params = {k: v for k, v in (params or {}).items() if v is not None}
    resolved_store_id = store_id or (params.get("store_id") if params else None)

    start = time.perf_counter()
    response = await client.request(
        method=method,
        url=url,
        params=req_params,
        json=json_body,
        headers=_headers(auth_header, cookie_header, correlation_id, user_id, user_role, resolved_store_id),
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
    store_id: Optional[str] = None,
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
                store_id=store_id,
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

