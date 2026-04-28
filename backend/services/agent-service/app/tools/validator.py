import logging
from typing import List, Dict
from app.tools.registry import TOOL_REGISTRY
from app.agent.schemas import ToolCall

def map_and_validate_calls(raw_calls: List[Dict], logger: logging.LoggerAdapter) -> List[ToolCall]:
    valid_calls = []
    seen_tools = set()

    for call in raw_calls:
        tool_name = call.get("tool", "")
        
        if tool_name not in TOOL_REGISTRY:
            logger.warning(f"Validation: Skipped unknown tool '{tool_name}'")
            continue
            
        if tool_name in seen_tools:
            logger.info(f"Validation: Deduplicated duplicate tool call '{tool_name}'")
            continue
            
        seen_tools.add(tool_name)
        params = call.get("params", {})
        
        try:
            valid_calls.append(ToolCall(tool=tool_name, params=params))
        except Exception as err:
            logger.warning(f"Validation: Malformed tool call {call}: {err}")

    MAX_TOOLS = 5
    if len(valid_calls) > MAX_TOOLS:
        logger.warning(f"Validation: Truncating tool calls from {len(valid_calls)} to {MAX_TOOLS}")
        valid_calls = valid_calls[:MAX_TOOLS]
        
    return valid_calls
