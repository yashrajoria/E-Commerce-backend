from typing import List, Dict
from app.llm.client import call_ollama
from app.utils.json_parser import parse_json_tool_calls
from app.tools.registry import get_tool_registry_text
from app.tools.validator import map_and_validate_calls
from app.agent.schemas import ToolCall
from app.core.logging import CorrelationAdapter

ROUTING_PROMPT = f"""You are a routing agent for an e-commerce admin dashboard.
Your ONLY job is to decide which backend tools must be called to answer the user's question.

Available tools:
{get_tool_registry_text()}

Rules:
1. Output ONLY a raw JSON array — no markdown, no explanation, no prose.
2. Each element must be {{"tool": "<name>", "params": {{...}}}}.
3. Include every tool needed to fully answer the question.
4. If the question cannot be answered by any tool, return [].
5. Infer sensible defaults for missing params (e.g. range="30d", limit=10).
"""

async def plan_tools(prompt: str, history: List[Dict[str, str]], logger: CorrelationAdapter) -> List[ToolCall]:
    messages = [{"role": "system", "content": ROUTING_PROMPT}] + history + [{"role": "user", "content": prompt}]
    
    try:
        raw_output = await call_ollama(messages, logger, json_mode=True)
        raw_calls = parse_json_tool_calls(raw_output, logger)
    except Exception as exc:
        logger.error(f"Routing LLM call failed: {exc} — returning empty plan")
        raw_calls = []

    valid_calls = map_and_validate_calls(raw_calls, logger)
    logger.info(f"Planner complete | tools={[c.tool for c in valid_calls]}")
    return valid_calls
