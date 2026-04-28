import re
from typing import Dict, List

from app.agent.planner import plan_tools
from app.agent.schemas import ToolCall
from app.core.logging import CorrelationAdapter
from app.tools.registry import TOOL_REGISTRY


def _extract_day_window(prompt_lower: str) -> int | None:
    match = re.search(r"\b(\d{1,3})\s*(?:day|days|d)\b", prompt_lower)
    if not match:
        return None
    days = int(match.group(1))
    if days <= 0:
        return None
    return days


def _validate_and_dedupe(calls: List[ToolCall], logger: CorrelationAdapter) -> List[ToolCall]:
    seen = set()
    valid: List[ToolCall] = []
    for call in calls:
        if call.tool not in TOOL_REGISTRY:
            logger.warning(
                f"Intent rejected unknown tool: {call.tool}",
                extra={"event": "intent.validation"},
            )
            continue
        if call.tool in seen:
            continue
        seen.add(call.tool)
        valid.append(call)
    return valid


def _heuristic_map(prompt_lower: str) -> List[ToolCall]:
    calls: List[ToolCall] = []
    days = _extract_day_window(prompt_lower)

    if re.search(r"total products|how many items|how many products|number of sku|number of skus", prompt_lower):
        calls.append(ToolCall(tool="get_product_count", params={}))
    if re.search(r"top products|best selling|best sellers|top items", prompt_lower):
        calls.append(ToolCall(tool="get_top_products", params={"limit": 5}))
    if re.search(r"low stock|low inventory|out of stock", prompt_lower):
        calls.append(ToolCall(tool="get_low_stock", params={"threshold": 10}))
    if re.search(r"sales|revenue", prompt_lower):
        calls.append(ToolCall(tool="get_sales", params={"range": "30d"}))
    if re.search(r"\border\b|\borders\b", prompt_lower):
        order_params: Dict[str, int | str] = {"page": 1, "limit": 100}
        if days is not None:
            order_params["days"] = days
            order_params["range"] = f"{days}d"
        calls.append(ToolCall(tool="get_orders", params=order_params))

    return calls


async def map_intent(prompt: str, history: List[Dict[str, str]], logger: CorrelationAdapter) -> List[ToolCall]:
    prompt_lower = prompt.lower().strip()
    logger.info(
        "Intent mapping started",
        extra={"event": "intent.start"},
    )

    heuristic_calls = _validate_and_dedupe(_heuristic_map(prompt_lower), logger)
    if heuristic_calls:
        selected = [c.tool for c in heuristic_calls]
        logger.info(
            f"Intent mapped via heuristics: {selected}",
            extra={"event": "intent.heuristic"},
        )
        return heuristic_calls

    logger.info(
        "Intent falling back to LLM planner",
        extra={"event": "intent.llm_fallback"},
    )
    llm_calls = await plan_tools(prompt, history, logger)
    llm_calls = _validate_and_dedupe(llm_calls, logger)
    logger.info(
        f"Intent mapped via LLM: {[c.tool for c in llm_calls]}",
        extra={"event": "intent.complete"},
    )
    return llm_calls
