"""Single source of truth for every tool the agent can call.

Each `ToolSpec` binds a typed Pydantic params model to its HTTP handler, a
mutating/read classification, and a minimum role. Descriptions shown to the
LLM are generated from `params_model.model_json_schema()` so the prompt text
can never drift from the actual validation schema.
"""

import json
from dataclasses import dataclass
from typing import Callable, Coroutine, Dict, Optional, Type

from pydantic import BaseModel

from app.tools import executor
from app.tools import schemas


@dataclass(frozen=True)
class ToolSpec:
    description: str
    params_model: Type[BaseModel]
    handler: Callable[..., Coroutine]
    mutating: bool = False
    min_role: Optional[str] = None


TOOL_REGISTRY: Dict[str, ToolSpec] = {
    "get_sales": ToolSpec(
        description="Fetch sales numeric data (revenue, order count, etc.).",
        params_model=schemas.GetSalesParams,
        handler=executor.get_sales,
    ),
    "get_top_products": ToolSpec(
        description="Fetch the top-selling products.",
        params_model=schemas.GetTopProductsParams,
        handler=executor.get_top_products,
    ),
    "get_low_stock": ToolSpec(
        description="List products with inventory below a threshold.",
        params_model=schemas.GetLowStockParams,
        handler=executor.get_low_stock,
    ),
    "get_failed_payments": ToolSpec(
        description="Retrieve recent failed payment transactions.",
        params_model=schemas.EmptyParams,
        handler=executor.get_failed_payments,
        min_role="admin",
    ),
    "get_orders": ToolSpec(
        description="Fetch order list with optional status filter.",
        params_model=schemas.GetOrdersParams,
        handler=executor.get_orders,
    ),
    "get_customers": ToolSpec(
        description="Fetch customer stats or list.",
        params_model=schemas.GetCustomersParams,
        handler=executor.get_customers,
    ),
    "get_revenue_breakdown": ToolSpec(
        description="Break down revenue by category, channel, or region.",
        params_model=schemas.GetRevenueBreakdownParams,
        handler=executor.get_revenue_breakdown,
        min_role="admin",
    ),
    "search_products": ToolSpec(
        description="Search products by name, SKU, or category.",
        params_model=schemas.SearchProductsParams,
        handler=executor.search_products,
    ),
    "get_product_count": ToolSpec(
        description="Get the total number of products in the store and a breakdown by category.",
        params_model=schemas.EmptyParams,
        handler=executor.get_product_count,
    ),
    "cancel_order": ToolSpec(
        description="Cancel an order. MUTATING — requires admin confirmation before it executes.",
        params_model=schemas.CancelOrderParams,
        handler=executor.cancel_order,
        mutating=True,
        min_role="admin",
    ),
    "create_restock_request": ToolSpec(
        description="Increase a product's available stock by a quantity. MUTATING — requires admin confirmation before it executes.",
        params_model=schemas.CreateRestockRequestParams,
        handler=executor.create_restock_request,
        mutating=True,
        min_role="admin",
    ),
}

READ_TOOLS: Dict[str, ToolSpec] = {name: spec for name, spec in TOOL_REGISTRY.items() if not spec.mutating}
MUTATING_TOOLS: Dict[str, ToolSpec] = {name: spec for name, spec in TOOL_REGISTRY.items() if spec.mutating}


def get_tool_registry_text() -> str:
    lines = []
    for name, spec in TOOL_REGISTRY.items():
        schema = spec.params_model.model_json_schema()
        properties = schema.get("properties", {})
        lines.append(f"- `{name}`: {spec.description}")
        lines.append(f"  Params: {json.dumps(properties) if properties else 'none'}")
    return "\n".join(lines)


def get_tool_registry_json() -> Dict[str, Dict]:
    """JSON-serializable view of the registry for the /tools endpoint."""
    return {
        name: {
            "description": spec.description,
            "params_schema": spec.params_model.model_json_schema(),
            "mutating": spec.mutating,
            "min_role": spec.min_role,
        }
        for name, spec in TOOL_REGISTRY.items()
    }
