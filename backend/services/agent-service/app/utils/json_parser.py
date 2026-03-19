import json
import re
import logging
from typing import List, Dict

def parse_json_tool_calls(raw: str, logger: logging.LoggerAdapter) -> List[Dict]:
    """
    Robustly extract a JSON array from raw LLM output.
    Handles markdown fences, extra whitespace, and trailing text.
    """
    raw = re.sub(r"```(?:json)?", "", raw).strip()

    try:
        parsed = json.loads(raw)
        if isinstance(parsed, list):
            return parsed
        logger.warning("LLM returned JSON but not a list — wrapping")
        return [parsed] if isinstance(parsed, dict) else []
    except json.JSONDecodeError:
        pass

    match = re.search(r"\[.*?\]", raw, re.DOTALL)
    if match:
        try:
            return json.loads(match.group())
        except json.JSONDecodeError:
            pass

    logger.warning(f"Could not parse tool calls from LLM output: {raw[:200]}")
    return []
