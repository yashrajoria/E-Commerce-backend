import logging
from typing import List, Dict

from pydantic import ValidationError

from app.tools.registry import TOOL_REGISTRY
from app.agent.schemas import ToolCall


def map_and_validate_calls(raw_calls: List[Dict], logger: logging.LoggerAdapter) -> List[ToolCall]:
    """Validate LLM-proposed tool calls against each tool's typed params model.

    This is the boundary that enforces "no raw/untyped access from the LLM":
    a proposal is only turned into a `ToolCall` (and later executed) once
    `spec.params_model.model_validate(...)` succeeds. Malformed arguments
    (wrong type, out-of-range, unknown field per model config) are rejected
    here and never reach an HTTP call.
    """
    valid_calls = []
    seen_tools = set()

    for call in raw_calls:
        tool_name = call.get("tool", "")

        spec = TOOL_REGISTRY.get(tool_name)
        if spec is None:
            logger.warning(f"Validation: Skipped unknown tool '{tool_name}'")
            continue

        if tool_name in seen_tools:
            logger.info(f"Validation: Deduplicated duplicate tool call '{tool_name}'")
            continue

        seen_tools.add(tool_name)
        raw_params = call.get("params", {})

        try:
            validated_params = spec.params_model.model_validate(raw_params)
        except ValidationError as err:
            logger.warning(f"Validation: Malformed params for '{tool_name}': {err}")
            continue

        valid_calls.append(ToolCall(tool=tool_name, params=validated_params.model_dump()))

    MAX_TOOLS = 5
    if len(valid_calls) > MAX_TOOLS:
        logger.warning(f"Validation: Truncating tool calls from {len(valid_calls)} to {MAX_TOOLS}")
        valid_calls = valid_calls[:MAX_TOOLS]

    return valid_calls
