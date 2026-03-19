from typing import Any, List

from app.agent.schemas import ToolResult
from app.core.logging import logger


def _safe_int(value: Any, default: int = 0) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _fmt_money(value: Any) -> str:
    try:
        return f"${float(value):,.2f}"
    except (TypeError, ValueError):
        return "$0.00"


def _humanize_status(status: str) -> str:
    cleaned = status.replace("_", " ").strip()
    return " ".join(part.capitalize() for part in cleaned.split()) or "Unknown"


def format_tool_result(result: ToolResult) -> str:
    if not result.success:
        return f"{result.tool.replace('_', ' ').title()} failed: {result.error or 'Unknown error.'}"

    data = result.data or {}

    if result.tool == "get_product_count":
        total = _safe_int(data.get("total_products"))
        breakdown = data.get("category_breakdown", {}) if isinstance(data, dict) else {}
        lines = [f"You currently have {total} products in your store."]
        if isinstance(breakdown, dict) and breakdown:
            lines.append("Category breakdown:")
            for category, count in sorted(breakdown.items(), key=lambda item: item[0].lower()):
                lines.append(f"- {category}: {_safe_int(count)}")
        return "\n".join(lines)

    if result.tool == "get_top_products":
        products = data.get("products", []) if isinstance(data, dict) else []
        if not products:
            return "No top products were returned for the selected period."
        lines = ["Top products:"]
        for product in products:
            name = product.get("name") or product.get("title") or "Unknown product"
            sold = _safe_int(product.get("units") or product.get("sold") or product.get("quantity"))
            lines.append(f"- {name}: {sold} units sold")
        return "\n".join(lines)

    if result.tool == "get_low_stock":
        items = data.get("items", []) if isinstance(data, dict) else []
        threshold = _safe_int(data.get("threshold"), 10) if isinstance(data, dict) else 10
        if not items:
            return f"No products are currently below the low-stock threshold ({threshold})."
        lines = [f"Low stock alerts (threshold: {threshold}):"]
        for item in items:
            name = (
                item.get("name")
                or item.get("title")
                or item.get("product_name")
                or item.get("product_id")
                or "Unknown product"
            )
            stock = _safe_int(
                item.get("stock")
                or item.get("quantity")
                or item.get("current_stock")
                or item.get("available")
            )
            lines.append(f"- {name}: {stock} remaining")
        return "\n".join(lines)

    if result.tool == "get_sales":
        if not isinstance(data, dict):
            return "Sales summary is unavailable right now."
        revenue = data.get("revenue") or data.get("total_revenue") or 0
        orders = _safe_int(data.get("orders") or data.get("order_count"))
        period = data.get("range", "30d")
        return (
            f"Sales summary for {period}:\n"
            f"- Revenue: {_fmt_money(revenue)}\n"
            f"- Orders: {orders}"
        )

    if result.tool == "get_orders":
        if not isinstance(data, dict):
            return "Orders summary is unavailable right now."

        meta = data.get("meta") if isinstance(data.get("meta"), dict) else {}
        orders = data.get("orders") if isinstance(data.get("orders"), list) else []

        total_orders = _safe_int(meta.get("total_orders"), len(orders))
        period = meta.get("range")
        period_text = f" in the last {str(period).rstrip('d')} days" if isinstance(period, str) and period.endswith("d") else ""

        status_counts: dict[str, int] = {}
        for order in orders:
            if not isinstance(order, dict):
                continue
            status = str(order.get("Status") or order.get("status") or "unknown")
            status_counts[status] = status_counts.get(status, 0) + 1

        lines = [f"Total orders{period_text}: {total_orders}."]
        if status_counts:
            lines.append("Status breakdown:")
            for status, count in sorted(status_counts.items(), key=lambda item: item[0].lower()):
                lines.append(f"- {_humanize_status(status)}: {_safe_int(count)}")
        return "\n".join(lines)

    if isinstance(data, dict):
        keys = ", ".join(sorted(data.keys()))
        return f"{result.tool.replace('_', ' ').title()} completed successfully. Available fields: {keys or 'none'}."

    return f"{result.tool.replace('_', ' ').title()} completed successfully."


def format_all_results(results: List[ToolResult]) -> List[ToolResult]:
    for result in results:
        result.summary = format_tool_result(result)
        logger.info(
            f"Formatter summary generated for {result.tool}",
            extra={"event": "formatter.summary", "tool": result.tool},
        )
    return results
