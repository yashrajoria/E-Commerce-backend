# Agent Service

The agent service is the Python/FastAPI conversational interface for ShopSwift. It runs on port `8089`, keeps short-lived in-process session history, and calls the BFF directly to execute storefront operations on behalf of the authenticated user.

## Responsibilities

- Accept natural-language queries and maintain a session conversation.
- Select and invoke tools registered by the agent runtime.
- Forward authorization, cookies, `X-User-ID`, `X-User-Role`, and correlation IDs to the BFF.
- Expose session inspection and clearing for controlled clients.
- Provide health and tool-discovery endpoints.

## Architecture

```mermaid
flowchart LR
  Client[Web / Assistant UI] --> Gateway[API Gateway :8080]
  Gateway --> Agent[Agent Service :8089\nPython / FastAPI]
  Agent --> Session[In-process session history]
  Agent -->|direct BFF call\nforwarded auth context| BFF[BFF Service :8088]
  BFF --> Domain[Catalog, cart, order, payment, user services]
  Agent --> LLM[Configured LLM endpoint]
```

The direct Agent-to-BFF path avoids sending the agent through a gateway route that could create a routing loop. The service is stateless across process restarts: session history is local memory and is not a durable customer record.

## Query flow

```mermaid
sequenceDiagram
  actor User
  participant Gateway
  participant Agent
  participant Session
  participant LLM
  participant BFF

  User->>Gateway: POST /agent/query
  Gateway->>Agent: Prompt + verified identity headers
  Agent->>Session: Load session history
  Agent->>LLM: Plan response and tool calls
  LLM-->>Agent: Tool decision
  Agent->>BFF: Call storefront operation with auth context
  BFF-->>Agent: Domain result
  Agent->>Session: Append turn
  Agent-->>User: AgentResponseV2
```

## HTTP surface

| Route | Purpose | Access |
| --- | --- | --- |
| `POST /agent/query` | Run an agent query | Gateway-authenticated or guest context |
| `GET /agent/session/:session_id` | Inspect in-memory history | Controlled/internal use |
| `DELETE /agent/session/:session_id` | Clear session history | Controlled/internal use |
| `GET /tools` and `GET /agent/tools` | List registered tools | Public/runtime discovery |
| `GET /health` | Health status and version | Public |

## Configuration and safety

Configure the BFF base URL, LLM endpoint and credentials, request timeouts, and logging. Preserve correlation IDs across downstream calls. Do not persist tokens or sensitive prompts in session logs; session memory should be treated as ephemeral process state.
