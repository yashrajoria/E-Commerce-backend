import logging

def can_execute_tool(tool_name: str, user_role: str, logger: logging.LoggerAdapter) -> bool:
    restricted_tools = ["get_failed_payments", "get_revenue_breakdown"]
    if tool_name in restricted_tools:
        if user_role not in ["admin", "super-admin"]:
            logger.warning(f"Security: User with role '{user_role}' denied access to '{tool_name}'")
            return False
    return True
