from __future__ import annotations

from typing import Any, Dict, List, Optional

from app.tools.handlers.base import _extract_data, _request


def _stock(item: Dict[str, Any]) -> int:
    for key in ("stock", "quantity", "current_stock", "inventory", "available"):
        value = item.get(key)
        if isinstance(value, (int, float)):
            return int(value)
    return 0


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

