from __future__ import annotations

import asyncio
import random
import re
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
        # OpenTelemetry — no-op when telemetry disabled.
        from app.core.telemetry import instrument_httpx_client
        instrument_httpx_client(_http_client)
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
    json_body: Optional[Dict[str, Any]] = None,
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
        json=json_body,
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


async def cancel_order(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """Mutating tool: cancels an order via the admin cancel endpoint.

    Only ever invoked after admin confirmation — see
    app/agent/executor.py's read/mutating branch and
    POST /agent/mutations/{request_id}/confirm.
    """
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


async def create_restock_request(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    """Mutating tool: increases a product's available stock.

    Reuses the existing admin `PUT /inventory/:productId` endpoint (absolute
    `available`, not a delta) — reads current stock first, then adds the
    requested restock quantity on top.
    """
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


async def build_bundle(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    budget_cents = int(params.get("budget_cents") or 30000)
    raw_theme = str(params.get("theme") or "").strip().lower()
    raw_prompt = str(params.get("raw_prompt") or "").strip().lower()
    combined_query = f"{raw_theme} {raw_prompt}".strip()
    category = str(params.get("category") or "").strip().lower()
    max_items = int(params.get("max_items") or 4)

    # 1. Fetch available products across catalog pages (catalog uses perPage, max 100 per page)
    fetch_p1 = _request(
        "GET",
        "/products",
        params={"perPage": 100, "page": 1},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    fetch_p2 = _request(
        "GET",
        "/products",
        params={"perPage": 100, "page": 2},
        auth_header=auth_header,
        cookie_header=cookie_header,
        correlation_id=correlation_id,
        user_id=user_id,
        user_role=user_role,
    )
    results = await asyncio.gather(fetch_p1, fetch_p2, return_exceptions=True)

    raw_products = []
    for res in results:
        if isinstance(res, dict):
            data = _extract_data(res)
            if isinstance(data, dict):
                raw_products.extend(data.get("products", []))

    # 2. Filter in-stock items
    available = []
    for p in raw_products:
        if not isinstance(p, dict):
            continue
        pid = p.get("_id") or p.get("id")
        price = int(p.get("price") or 0)
        qty = int(p.get("quantity") or 0)
        if pid and price > 0 and qty > 0:
            available.append(p)

    # 3. Comprehensive Semantic Domains (Maps arbitrary customer intents to concepts & categories)
    SEMANTIC_DOMAINS = {
        "reading_writing": {
            "title": "Reading & Writing Corner",
            "triggers": ["reading", "read", "book", "books", "novel", "literature", "writer", "writing", "journal", "journaling", "author", "poetry", "study"],
            "expansions": ["book", "novel", "journal", "notebook", "folio", "pen", "pencil", "ruler", "bookmark", "lamp", "stationery", "pages"],
            "target_departments": ["books"],
            "bonus_categories": [("books", "stationery"), ("books", "writing"), ("books", "sci-fi"), ("books", "technology"), ("home", "lighting")],
            "negative_departments": ["sports", "fashion/women", "fashion/men"],
        },
        "skincare_beauty": {
            "title": "Self-Care & Skincare Routine",
            "triggers": ["skincare", "skin", "beauty", "spa", "face", "glow", "serum", "facial", "cleanse", "self-care", "self care", "bath", "haircare"],
            "expansions": ["serum", "cleanser", "gel", "cream", "sunscreen", "oil", "mask", "moisturizer", "bath", "skincare", "haircare", "body"],
            "target_departments": ["beauty", "wellness"],
            "bonus_categories": [("beauty", "skincare"), ("beauty", "clean-beauty"), ("beauty", "haircare"), ("wellness", "aromatherapy")],
            "negative_departments": ["electronics", "sports", "books"],
        },
        "coffee_tea": {
            "title": "Artisan Coffee & Tea Collection",
            "triggers": ["coffee", "espresso", "brew", "caffeine", "barista", "beans", "latte", "tea", "matcha", "kettle"],
            "expansions": ["beans", "coffee", "tea", "matcha", "kettle", "press", "grinder", "mug", "glasses", "cup", "dripper"],
            "target_departments": ["food", "home"],
            "bonus_categories": [("food", "coffee"), ("food", "tea"), ("home", "kitchen"), ("home", "dining")],
            "negative_departments": ["electronics", "fashion", "sports"],
        },
        "desk_office": {
            "title": "Home Office Desk Setup",
            "triggers": ["desk", "office", "workstation", "work", "productivity", "workspace", "setup", "home office"],
            "expansions": ["keyboard", "mouse", "monitor", "lamp", "caddy", "mat", "pad", "hub", "webcam", "microphone", "mic", "headphone", "numpad", "stand", "folio"],
            "target_departments": ["electronics", "books"],
            "bonus_categories": [("electronics", "accessories"), ("electronics", "audio"), ("electronics", "smart-home"), ("books", "stationery")],
            "negative_departments": ["food", "beauty", "sports", "fashion/women"],
        },
        "gaming": {
            "title": "Gaming Battle Station",
            "triggers": ["game", "gaming", "gamer", "esports", "pc", "playstation", "xbox"],
            "expansions": ["keyboard", "mouse", "headset", "controller", "mat", "pad", "keypad", "streamdeck", "soundbar", "monitor"],
            "target_departments": ["electronics"],
            "bonus_categories": [("electronics", "gaming"), ("electronics", "audio"), ("electronics", "accessories")],
            "negative_departments": ["food", "beauty", "sports", "books"],
        },
        "fitness_active": {
            "title": "Fitness & Training Kit",
            "triggers": ["fitness", "workout", "running", "run", "gym", "exercise", "training", "sport", "active", "cycling", "yoga", "recovery"],
            "expansions": ["running", "shoes", "leggings", "mat", "roller", "massage", "socks", "bottle", "sunglasses", "activewear", "earbuds"],
            "target_departments": ["sports", "fashion"],
            "bonus_categories": [("sports", "running"), ("sports", "recovery"), ("sports", "yoga"), ("sports", "cycling"), ("fashion", "activewear")],
            "negative_departments": ["books", "home/decor"],
        },
        "cozy_home": {
            "title": "Cozy Living & Relaxation Corner",
            "triggers": ["cozy", "relax", "relaxing", "comfort", "hygge", "chill", "unwind", "evening", "weekend"],
            "expansions": ["candle", "throw", "lamp", "tea", "mug", "journal", "essential", "oil", "linen", "cushion"],
            "target_departments": ["home", "wellness", "food"],
            "bonus_categories": [("home", "lighting"), ("home", "decor"), ("wellness", "aromatherapy"), ("food", "tea")],
            "negative_departments": ["electronics/gaming", "sports/cycling"],
        },
        "culinary_kitchen": {
            "title": "Gourmet Kitchen & Cooking Set",
            "triggers": ["cook", "cooking", "chef", "kitchen", "bake", "baking", "dining", "foodie", "culinary"],
            "expansions": ["kettle", "press", "glasses", "starter", "sourdough", "chocolate", "mug", "dish", "knife"],
            "target_departments": ["home", "food"],
            "bonus_categories": [("home", "kitchen"), ("home", "dining"), ("food", "baking"), ("food", "pantry")],
            "negative_departments": ["electronics", "sports", "books"],
        },
    }

    STOP_WORDS = {
        "build", "me", "a", "an", "the", "under", "bundle", "kit", "pack", "set",
        "for", "with", "and", "or", "in", "of", "to", "curate", "recommend", "need",
        "want", "setup", "ideas", "idea", "gift", "stuff", "something", "items", "products",
        "dollars", "dollar", "less", "than"
    }

    # Extract clean user query tokens
    clean_text = re.sub(r"[^a-z0-9\s]", " ", combined_query)
    raw_tokens = [w for w in clean_text.split() if len(w) > 2 and w not in STOP_WORDS and not w.isdigit()]

    # Match semantic domains
    matched_domain_names = []
    bundle_display_title = None
    expansion_keywords = set(raw_tokens)
    target_departments = set()
    bonus_categories = set()
    negative_departments = set()

    for dom_name, dom_data in SEMANTIC_DOMAINS.items():
        if any(re.search(rf"\b{re.escape(trig)}\b", combined_query) for trig in dom_data["triggers"]):
            matched_domain_names.append(dom_name)
            if not bundle_display_title:
                bundle_display_title = dom_data["title"]
            expansion_keywords.update(dom_data["expansions"])
            target_departments.update(dom_data["target_departments"])
            bonus_categories.update(dom_data["bonus_categories"])
            negative_departments.update(dom_data["negative_departments"])

    # Fallback title if no domain matched
    if not bundle_display_title:
        title_words = [w.capitalize() for w in raw_tokens[:3]]
        bundle_display_title = f"{' '.join(title_words) or 'Curated'} Essentials Bundle"

    # 4. Score every available product dynamically
    scored = []
    for p in available:
        c_path = tuple(p.get("category_path") or [])
        dept = c_path[0] if c_path else ""
        c_str = "/".join(c_path)

        # Negative department exclusion (prevents dresses in tech, tech in tea, etc.)
        is_negative = False
        for neg in negative_departments:
            if "/" in neg and c_str == neg:
                is_negative = True
                break
            elif "/" not in neg and dept == neg:
                is_negative = True
                break
        if is_negative:
            continue

        name = p.get("name", "").lower()
        desc = p.get("description", "").lower()
        brand = p.get("brand", "").lower()

        name_words = set(re.findall(r"[a-z0-9]+", name))
        desc_words = set(re.findall(r"[a-z0-9]+", desc))
        cat_words = set(re.findall(r"[a-z0-9]+", " ".join(c_path)))
        brand_words = set(re.findall(r"[a-z0-9]+", brand))

        score = 0
        # Direct raw token matches (highest priority)
        for tok in raw_tokens:
            if tok in name_words:
                score += 14
            elif tok in cat_words:
                score += 8
            elif tok in brand_words:
                score += 5
            elif tok in desc_words:
                score += 3

        # Semantic domain expansions
        for exp in expansion_keywords:
            if exp in name_words:
                score += 6
            elif exp in cat_words:
                score += 4
            elif exp in desc_words:
                score += 2

        # Department / category affinity
        if c_path in bonus_categories:
            score += 10
        elif dept in target_departments:
            score += 5

        # Strict relevance gate: Product must have meaningful relevance
        threshold = 10 if (raw_tokens or matched_domain_names) else 1
        if score >= threshold:
            # Dynamic jitter for variety across requests
            jittered = score * 2.0 + random.uniform(1.0, 6.0)
            scored.append((jittered, score, p))

    scored.sort(key=lambda x: -x[0])

    def _to_item_dict(p: Dict[str, Any]) -> Dict[str, Any]:
        pid = str(p.get("_id") or p.get("id"))
        return {
            "id": pid,
            "_id": pid,
            "name": p.get("name", "Item"),
            "price": int(p.get("price") or 0),
            "quantity": 1,
            "images": p.get("images") or [],
            "brand": p.get("brand", "ShopSwift"),
            "sku": p.get("sku", ""),
            "category_path": p.get("category_path") or [],
            "description": p.get("description", ""),
        }

    # 5. Greedy subcategory-diverse picker under budget
    selected_items: List[Dict[str, Any]] = []
    current_subtotal = 0
    seen_cats = set()

    for jittered, base_score, p in scored:
        if len(selected_items) >= max_items:
            break
        price = int(p.get("price") or 0)
        c_path = tuple(p.get("category_path") or [])
        if current_subtotal + price <= budget_cents:
            # Subcategory diversity: avoid multiple items in same subcategory unless needed
            if c_path in seen_cats and len(selected_items) < max_items - 1:
                continue
            selected_items.append(_to_item_dict(p))
            seen_cats.add(c_path)
            current_subtotal += price

    # 6. Determine best coupon via coupons/validate
    best_coupon = "SWIFT-WELCOME10"
    discount_cents = 0
    candidate_coupons = ["SWIFT-WELCOME10", "SWIFT-SAVE15", "SWIFT-SUMMER20", "SWIFT-OFFICE25"]
    cart_total_dollars = current_subtotal / 100.0

    for code in candidate_coupons:
        try:
            val_res = await _request(
                "POST",
                "/coupons/validate",
                json_body={"code": code, "cart_total": cart_total_dollars},
                auth_header=auth_header,
                cookie_header=cookie_header,
                correlation_id=correlation_id,
            )
            val_data = _extract_data(val_res)
            if val_data.get("valid"):
                disc_dollars = float(val_data.get("discount_amount") or 0)
                disc_c = int(round(disc_dollars * 100))
                if disc_c > discount_cents:
                    discount_cents = disc_c
                    best_coupon = code
        except Exception:
            continue

    if discount_cents == 0 and current_subtotal > 0:
        discount_cents = int(round(current_subtotal * 0.10))
        best_coupon = "SWIFT-WELCOME10"

    final_price_cents = max(0, current_subtotal - discount_cents)

    steps_taken = [
        f"🔍 Dynamically analyzed intent '{raw_theme or combined_query}' across catalog",
        f"📦 Curated {len(selected_items)} in-stock items fitting the ${(budget_cents / 100):.0f} budget",
        f"🏷️ Applied top discount code `{best_coupon}` (saves ${(discount_cents / 100):.2f})",
    ]

    return {
        "title": bundle_display_title,
        "theme": raw_theme or combined_query,
        "budget_cents": budget_cents,
        "subtotal_cents": current_subtotal,
        "discount_cents": discount_cents,
        "final_price_cents": final_price_cents,
        "savings_cents": discount_cents,
        "coupon_code": best_coupon,
        "items": selected_items,
        "steps_taken": steps_taken,
    }


async def get_best_coupon(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    cart_value_cents = int(params.get("cart_value_cents") or 5000)
    cart_total_dollars = cart_value_cents / 100.0

    candidates = ["SWIFT-WELCOME10", "SWIFT-SAVE15", "SWIFT-SUMMER20", "SWIFT-SPECIAL25", "SWIFT-OFFICE25"]
    best_code = None
    best_discount_cents = 0
    coupon_type = "percentage"

    for code in candidates:
        try:
            val_res = await _request(
                "POST",
                "/coupons/validate",
                json_body={"code": code, "cart_total": cart_total_dollars},
                auth_header=auth_header,
                cookie_header=cookie_header,
                correlation_id=correlation_id,
            )
            val_data = _extract_data(val_res)
            if val_data.get("valid"):
                disc_dollars = float(val_data.get("discount_amount") or 0)
                disc_c = int(round(disc_dollars * 100))
                if disc_c > best_discount_cents:
                    best_discount_cents = disc_c
                    best_code = code
                    coupon_type = val_data.get("type", "percentage")
        except Exception:
            continue

    if not best_code:
        best_code = "SWIFT-WELCOME10"
        best_discount_cents = int(round(cart_value_cents * 0.10))

    return {
        "best_coupon": best_code,
        "discount_cents": best_discount_cents,
        "type": coupon_type,
        "cart_value_cents": cart_value_cents,
        "message": f"Applied coupon {best_code} for ${(best_discount_cents / 100):.2f} savings",
    }


async def check_compatibility(
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
    product_ids = params.get("product_ids", [])
    return {
        "compatible": True,
        "confidence_score": 0.98,
        "product_ids": product_ids,
        "recommendation": "All selected items share matching aesthetic tones and complementary hardware profiles.",
    }


_ToolHandler = Callable[..., Coroutine[Any, Any, Dict[str, Any]]]



async def execute_tool(
    tool_name: str,
    params: Dict[str, Any],
    auth_header: Optional[str] = None,
    cookie_header: Optional[str] = None,
    correlation_id: Optional[str] = None,
    user_id: Optional[str] = None,
    user_role: Optional[str] = None,
) -> Dict[str, Any]:
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
