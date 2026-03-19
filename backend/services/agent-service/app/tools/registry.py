import json

TOOL_REGISTRY = {
    "get_sales": {
        "description": "Fetch sales numeric data (revenue, order count, etc.).",
        "params": {"range": "string — e.g. '7d', '30d', '1y'"},
    },
    "get_top_products": {
        "description": "Fetch the top-selling products.",
        "params": {
            "range": "string — e.g. '7d', '30d'",
            "limit": "integer — number of products to return",
        },
    },
    "get_low_stock": {
        "description": "List products with inventory below a threshold.",
        "params": {"threshold": "integer — stock count below which a product is 'low'"},
    },
    "get_failed_payments": {
        "description": "Retrieve recent failed payment transactions.",
        "params": {},
    },
    "get_orders": {
        "description": "Fetch order list with optional status filter.",
        "params": {
            "status": "string — e.g. 'pending', 'shipped', 'cancelled', 'all'",
            "range": "string — e.g. '7d', '30d'",
            "limit": "integer",
        },
    },
    "get_customers": {
        "description": "Fetch customer stats or list.",
        "params": {
            "range": "string — e.g. '30d', '1y'",
            "metric": "string — e.g. 'new', 'returning', 'all'",
        },
    },
    "get_revenue_breakdown": {
        "description": "Break down revenue by category, channel, or region.",
        "params": {
            "group_by": "string — e.g. 'category', 'channel', 'region'",
            "range": "string — e.g. '30d', '1y'",
        },
    },
    "search_products": {
        "description": "Search products by name, SKU, or category.",
        "params": {"query": "string — search term"},
    },
    "get_product_count": {
        "description": "Get the total number of products in the store and a breakdown by category.",
        "params": {},
    },
}

def get_tool_registry_text() -> str:
    return "\n".join(
        f"- `{name}`: {meta['description']}\n"
        f"  Params: {json.dumps(meta['params']) if meta['params'] else 'none'}"
        for name, meta in TOOL_REGISTRY.items()
    )
