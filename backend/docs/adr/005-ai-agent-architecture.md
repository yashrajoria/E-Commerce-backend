# ADR 005: AI Agent Architecture, Safety Boundaries & Human-in-the-Loop Mutations

## Status
Accepted

## Context
ShopSwift integrates an intelligent assistant (`agent-service`) to assist platform administrators with sales analysis, stock alerts, and store operations. Giving an LLM direct shell, arbitrary HTTP, or unrestricted raw database execution creates catastrophic security risks (prompt injection, unauthorized data exfiltration, accidental mass deletions).

## Decision
Implement a **Constrained Tool-Execution Architecture with Mandatory Mutation Safeguards**:
1. **Tool Allowlist (`TOOL_REGISTRY`)**: The LLM can only select from a strictly curated list of pre-registered tools. Unknown tools and unmapped capabilities are rejected before invocation.
2. **Schema & Argument Validation**: Every tool's arguments are typed and validated with Pydantic (`app/tools/schemas.py`). Malformed, out-of-bounds, or injection-like parameters are rejected at the boundary (`map_and_validate_calls`).
3. **Role-Based Access Control**: Sensitive tools (e.g. `get_failed_payments`, mutations) require explicit role verification (`can_execute_tool`).
4. **Two-Phase Human-in-the-Loop Confirmation**: Mutating tools (such as `cancel_order`, `create_restock_request`) are marked `mutating=True`. When proposed by the LLM, the handler is **never executed directly**. Instead:
   - A pending record is persisted to the `agent_audit_log` with status `pending_confirmation`.
   - The user/admin is prompted with the exact proposed payload.
   - Execution is only triggered when an explicit approval request (`POST /agent/confirm`) is received from an authenticated admin.
5. **Full Audit Logging**: Every tool proposal, confirmation, execution, and execution result is logged with correlation IDs.

## Consequences
- **Positive**: Immune to arbitrary code/SQL injection from prompt tampering; guarantees administrative oversight on critical state mutations; comprehensive audit trail.
- **Negative**: Multi-step user interaction required for operations that alter store state.
