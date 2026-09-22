import logging

from app.tools.registry import TOOL_REGISTRY


def can_execute_tool(tool_name: str, user_role: str, logger: logging.LoggerAdapter) -> bool:
    """Defense-in-depth role check.

    The edge (api-gateway) already gates all /agent/* traffic to admin-role
    JWT holders, but this repeats the check per-tool from each ToolSpec's
    `min_role` so a future non-admin-gated entrypoint can't skip it.
    """
    spec = TOOL_REGISTRY.get(tool_name)
    min_role = spec.min_role if spec else None
    if min_role and user_role not in (min_role, "super-admin"):
        logger.warning(f"Security: User with role '{user_role}' denied access to '{tool_name}'")
        return False
    return True
