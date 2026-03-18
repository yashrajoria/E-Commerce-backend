"""
FastAPI agent service for ShopSwift admin dashboard.

Architecture:
  1. Validate & sanitize the incoming prompt
  2. ROUTING PASS  — LLM decides which tools to call (structured JSON)
  3. EXECUTION     — Run all tools concurrently; isolate failures per-tool
  4. SYNTHESIS PASS — LLM reads tool results and writes the final answer
  5. Return a rich AgentResponse with the natural-language answer + raw data

Design principles
-----------------
* Two-pass LLM pattern: routing → execution → synthesis
* Per-tool error isolation via return_exceptions=True
* Session-aware conversation history for multi-turn support
* All config-driven (no hard-coded URLs / model names)
* Structured logging with correlation ID on every log line
* Clean separation: routing prompt vs synthesis prompt vs tool executor
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
from contextlib import asynccontextmanager
from typing import Any, AsyncGenerator, Optional
from uuid import uuid4

import httpx
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field, field_validator

from config import settings
from models import AgentQueryRequest, AgentResponse, ToolCall, ToolResult
from tools import close_http_client, execute_tool

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(name)s %(levelname)s %(message)s",
)
logger = logging.getLogger(__name__)


class CorrelationAdapter(logging.LoggerAdapter):
    """
    Injects [correlation_id] into every log message produced via get_logger().
    Compatible with Python 3.11+ (avoids the 3.12-only `defaults` kwarg).
    """

    def process(self, msg: str, kwargs: dict) -> tuple[str, dict]:
        cid = self.extra.get("correlation_id", "N/A")
        return f"[{cid}] {msg}", kwargs


def get_logger(correlation_id: str) -> CorrelationAdapter:
    return CorrelationAdapter(logger, {"correlation_id": correlation_id})


# ---------------------------------------------------------------------------
# Session store (in-memory; swap for Redis in production)
# ---------------------------------------------------------------------------

_sessions: dict[str, list[dict]] = {}

MAX_HISTORY_TURNS = 10  # keep last N user+assistant pairs


def _get_history(session_id: str) -> list[dict]:
    return _sessions.get(session_id, [])


def _push_history(session_id: str, role: str, content: str) -> None:
    history = _sessions.setdefault(session_id, [])
    history.append({"role": role, "content": content})
    # Trim to last MAX_HISTORY_TURNS * 2 messages (user + assistant pairs)
    if len(history) > MAX_HISTORY_TURNS * 2:
        _sessions[session_id] = history[-(MAX_HISTORY_TURNS * 2):]


# ---------------------------------------------------------------------------
# Tool registry — single source of truth
# ---------------------------------------------------------------------------

TOOL_REGISTRY: dict[str, dict] = {
    "get_sales": {
        "description": "Fetch sales numeric data (revenue, order count, etc.).",
        "params": {"range": "string — e.g. '7d', '30d', '1y'"},
    },
    "get_top_products": {
        "description": "Fetch the top-selling products.",
        "params": {
            "range": "string — e.g. '7d', '30d'",
            "limit": "integer — number of products to return",
        },
    },
    "get_low_stock": {
        "description": "List products with inventory below a threshold.",
        "params": {"threshold": "integer — stock count below which a product is 'low'"},
    },
    "get_failed_payments": {
        "description": "Retrieve recent failed payment transactions.",
        "params": {},
    },
    "get_orders": {
        "description": "Fetch order list with optional status filter.",
        "params": {
            "status": "string — e.g. 'pending', 'shipped', 'cancelled', 'all'",
            "range": "string — e.g. '7d', '30d'",
            "limit": "integer",
        },
    },
    "get_customers": {
        "description": "Fetch customer stats or list.",
        "params": {
            "range": "string — e.g. '30d', '1y'",
            "metric": "string — e.g. 'new', 'returning', 'all'",
        },
    },
    "get_revenue_breakdown": {
        "description": "Break down revenue by category, channel, or region.",
        "params": {
            "group_by": "string — e.g. 'category', 'channel', 'region'",
            "range": "string — e.g. '30d', '1y'",
        },
    },
    "search_products": {
        "description": "Search products by name, SKU, or category.",
        "params": {"query": "string — search term"},
    },
    "get_product_count": {
        "description": "Get the total number of products in the store.",
        "params": {},
    },
}

_TOOL_REGISTRY_TEXT = "\n".join(
    f"- `{name}`: {meta['description']}\n"
    f"  Params: {json.dumps(meta['params']) if meta['params'] else 'none'}"
    for name, meta in TOOL_REGISTRY.items()
)

# ---------------------------------------------------------------------------
# Prompts
# ---------------------------------------------------------------------------

ROUTING_SYSTEM_PROMPT = f"""You are a routing agent for an e-commerce admin dashboard.
Your ONLY job is to decide which backend tools must be called to answer the user's question.

Available tools:
{_TOOL_REGISTRY_TEXT}

Rules:
1. Output ONLY a raw JSON array — no markdown, no explanation, no prose.
2. Each element must be {{"tool": "<name>", "params": {{...}}}}.
3. Include every tool needed to fully answer the question.
4. If the question cannot be answered by any tool, return [].
5. Infer sensible defaults for missing params (e.g. range="30d", limit=10).

Examples:
User: "Show me sales for the last week"
Output: [{{"tool": "get_sales", "params": {{"range": "7d"}}}}]

User: "What are my top 5 products this month and do I have any low stock?"
Output: [
  {{"tool": "get_top_products", "params": {{"range": "30d", "limit": 5}}}},
  {{"tool": "get_low_stock", "params": {{"threshold": 10}}}}
]
"""

SYNTHESIS_SYSTEM_PROMPT = """You are a knowledgeable e-commerce business analyst assistant
embedded in an admin dashboard. You have just run a set of backend data tools on behalf
of the admin and received their results.

Your job: write a clear, concise, professional answer to the admin's question using
ONLY the data provided. Do NOT make up numbers. If a tool failed, say so briefly and
work with what succeeded.

Formatting rules:
- Use plain prose; bullet points only for lists of items.
- Highlight the most important insight first.
- Keep it under ~200 words unless the data demands more.
- Never expose raw JSON to the user.
- Address the admin directly (use "you" / "your").
"""


# ---------------------------------------------------------------------------
# LLM client
# ---------------------------------------------------------------------------

async def _call_ollama(
    messages: list[dict],
    log: CorrelationAdapter,
    *,
    json_mode: bool = False,
) -> str:
    """
    Send a chat request to the configured Ollama endpoint.
    Returns the assistant's text content.

    Raises httpx.HTTPError on network/HTTP failures (caller decides how to handle).
    """
    llm_url = settings.LLM_SERVICE_URL
    model = getattr(settings, "LLM_MODEL", "qwen2.5:7b")

    payload: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "stream": False,
        "options": {"temperature": 0.0},
    }
    if json_mode:
        payload["format"] = "json"

    log.info(f"Calling Ollama | model={model} | url={llm_url} | msgs={len(messages)}")
    print(f"[OLLAMA_CALL] URL: {llm_url}")
    print(f"[OLLAMA_CALL] Payload (JSON Mode: {json_mode}):\n{json.dumps(payload, indent=2)}")
    
    try:
        async with httpx.AsyncClient() as client:
            response = await client.post(
                llm_url,
                json=payload,
                timeout=settings.LLM_TIMEOUT,
            )
            print(f"[OLLAMA_CALL] Response status code: {response.status_code}")
            response.raise_for_status()

        raw_resp = response.json()
        content: str = raw_resp.get("message", {}).get("content", "")
        print(f"[OLLAMA_CALL] Response content length: {len(content)}")
        
        log.info(f"Ollama responded | content_length={len(content)}")
        return content
    except Exception as e:
        print(f"[OLLAMA_CALL] EXCEPTION in _call_ollama: {repr(e)}")
        raise


def _parse_json_tool_calls(raw: str, log: CorrelationAdapter) -> list[dict]:
    """
    Robustly extract a JSON array from raw LLM output.
    Handles markdown fences, extra whitespace, and trailing text.
    """
    # Strip markdown fences
    raw = re.sub(r"```(?:json)?", "", raw).strip()

    # Try direct parse
    try:
        parsed = json.loads(raw)
        if isinstance(parsed, list):
            return parsed
        log.warning("LLM returned JSON but not a list — wrapping")
        return [parsed] if isinstance(parsed, dict) else []
    except json.JSONDecodeError:
        pass

    # Try to find the first JSON array in the text
    match = re.search(r"\[.*?\]", raw, re.DOTALL)
    if match:
        try:
            return json.loads(match.group())
        except json.JSONDecodeError:
            pass

    log.warning(f"Could not parse tool calls from LLM output: {raw[:200]}")
    return []


# ---------------------------------------------------------------------------
# Two-pass agent core
# ---------------------------------------------------------------------------

async def run_agent(
    prompt: str,
    session_id: str,
    auth_header: Optional[str],
    cookie_header: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    log: CorrelationAdapter,
) -> tuple[str, list[ToolResult]]:
    """
    Full two-pass agent pipeline.

    Returns:
        (natural_language_answer, list_of_tool_results)
    """

    # ── Pass 1: Routing ────────────────────────────────────────────────────
    print(f"[{log.extra.get('correlation_id', 'N/A')}] --- PASS 1: ROUTING PASS ---")
    history = _get_history(session_id)
    routing_messages = (
        [{"role": "system", "content": ROUTING_SYSTEM_PROMPT}]
        + history
        + [{"role": "user", "content": prompt}]
    )

    raw_tool_calls: list[dict] = []
    try:
        raw = await _call_ollama(routing_messages, log, json_mode=True)
        raw_tool_calls = _parse_json_tool_calls(raw, log)
    except Exception as exc:
        log.error(f"Routing LLM call failed: {exc} — using fallback tool set")
        # Graceful degradation: default overview tools
        raw_tool_calls = [
            {"tool": "get_sales", "params": {"range": "30d"}},
            {"tool": "get_top_products", "params": {"range": "30d", "limit": 5}},
        ]

    # Validate tool names against registry; drop unknown tools
    valid_calls: list[ToolCall] = []
    for call in raw_tool_calls:
        tool_name = call.get("tool", "")
        if tool_name not in TOOL_REGISTRY:
            log.warning(f"LLM requested unknown tool '{tool_name}' — skipping")
            continue
        try:
            valid_calls.append(ToolCall(tool=tool_name, params=call.get("params", {})))
        except Exception as parse_err:
            log.warning(f"Malformed tool call {call}: {parse_err} — skipping")

    log.info(f"Routing complete | tools={[c.tool for c in valid_calls]}")
    print(f"[{log.extra.get('correlation_id', 'N/A')}] Routing generated tool calls: {[c.tool for c in valid_calls]}")

    # ── Pass 2: Concurrent tool execution ─────────────────────────────────
    print(f"[{log.extra.get('correlation_id', 'N/A')}] --- PASS 2: CONCURRENT TOOL EXECUTION ---")
    tool_results: list[ToolResult] = []

    if valid_calls:
        tasks = [
            execute_tool(
                tool_name=call.tool,
                params=call.params,
                auth_header=auth_header,
                cookie_header=cookie_header,
                user_id=user_id,
                user_role=user_role,
            )
            for call in valid_calls
        ]

        # return_exceptions=True: one failing tool never kills others
        raw_results = await asyncio.gather(*tasks, return_exceptions=True)

        for call, result in zip(valid_calls, raw_results):
            if isinstance(result, Exception):
                log.error(f"Tool '{call.tool}' raised: {result}")
                tool_results.append(
                    ToolResult(
                        tool=call.tool,
                        success=False,
                        data=None,
                        error=f"Tool execution error: {type(result).__name__}: {result}",
                    )
                )
            else:
                tool_results.append(ToolResult(**result))
    
    success_count = sum(1 for r in tool_results if r.success)
    print(f"[{log.extra.get('correlation_id', 'N/A')}] Tool execution complete. Successful: {success_count}/{len(tool_results)}")

    # ── Pass 3: Synthesis ─────────────────────────────────────────────────
    print(f"[{log.extra.get('correlation_id', 'N/A')}] --- PASS 3: SYNTHESIS PASS ---")
    # Build a compact, readable summary of tool results for the LLM
    results_summary = _format_results_for_synthesis(tool_results)

    synthesis_messages = (
        [{"role": "system", "content": SYNTHESIS_SYSTEM_PROMPT}]
        + history
        + [
            {"role": "user", "content": prompt},
            {
                "role": "assistant",
                "content": (
                    f"I ran the following data tools:\n\n{results_summary}\n\n"
                    "Now I will answer based on this data."
                ),
            },
            {
                "role": "user",
                "content": "Please provide your final answer to my question.",
            },
        ]
    )

    answer = "I was unable to generate a summary at this time."
    try:
        answer = await _call_ollama(synthesis_messages, log, json_mode=False)
        answer = answer.strip()
    except Exception as exc:
        log.error(f"Synthesis LLM call failed: {exc}")
        # Fall back to a plain data dump if synthesis fails
        answer = _plain_fallback_answer(tool_results)

    # Persist turn to session history
    _push_history(session_id, "user", prompt)
    _push_history(session_id, "assistant", answer)

    print(f"[{log.extra.get('correlation_id', 'N/A')}] --- PIPELINE COMPLETE ---")
    return answer, tool_results


def _format_results_for_synthesis(results: list[ToolResult]) -> str:
    """Convert ToolResult list into a compact text block for the synthesis prompt."""
    lines: list[str] = []
    for r in results:
        if r.success:
            data_str = json.dumps(r.data, indent=2) if r.data else "No data returned."
            lines.append(f"### {r.tool} — SUCCESS\n{data_str}")
        else:
            lines.append(f"### {r.tool} — FAILED\nError: {r.error}")
    return "\n\n".join(lines) if lines else "No tools were executed."


def _plain_fallback_answer(results: list[ToolResult]) -> str:
    """Last-resort answer when the synthesis LLM call fails."""
    parts: list[str] = ["Here is the raw data retrieved:\n"]
    for r in results:
        if r.success:
            parts.append(f"**{r.tool}**: {json.dumps(r.data)}")
        else:
            parts.append(f"**{r.tool}**: failed — {r.error}")
    return "\n".join(parts)


# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------

@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator:
    logger.info("Agent Service starting up")
    yield
    logger.info("Agent Service shutting down")
    await close_http_client()


app = FastAPI(
    title="ShopSwift Agent Service",
    description="Two-pass AI agent (routing → execution → synthesis) for the admin dashboard.",
    version="2.0.0",
    lifespan=lifespan,
)


@app.middleware("http")
async def add_correlation_id(request: Request, call_next):
    """Stamp every request/response with a traceable correlation ID."""
    correlation_id = request.headers.get("X-Correlation-ID", str(uuid4()))
    request.state.correlation_id = correlation_id
    response = await call_next(request)
    response.headers["X-Correlation-ID"] = correlation_id
    return response


# ---------------------------------------------------------------------------
# Request / Response models (augmented)
# ---------------------------------------------------------------------------

class AgentQueryRequestV2(BaseModel):
    """Extended request model with optional session support."""

    prompt: str = Field(..., min_length=1, max_length=2000)
    session_id: Optional[str] = Field(
        default=None,
        description="Pass a session_id to enable multi-turn conversation history.",
    )

    @field_validator("prompt")
    @classmethod
    def sanitize_prompt(cls, v: str) -> str:
        # Strip leading/trailing whitespace; collapse excessive internal whitespace
        v = v.strip()
        v = re.sub(r"\s{3,}", "  ", v)
        return v


class AgentResponseV2(BaseModel):
    """Rich response that includes both the natural-language answer and raw tool data."""

    success: bool
    answer: str = Field(description="Natural-language answer synthesised from tool data.")
    tool_results: list[ToolResult] = Field(default_factory=list)
    tools_called: list[str] = Field(default_factory=list)
    session_id: str
    correlation_id: str
    error: Optional[str] = None


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

@app.post("/agent/query", response_model=AgentResponseV2)
async def agent_query(request: AgentQueryRequestV2, req: Request) -> AgentResponseV2:
    """
    Main agent endpoint.

    Flow:
      1. Validate & sanitize prompt (Pydantic)
      2. ROUTING PASS  — LLM selects tools + params
      3. EXECUTION     — All tools run concurrently; failures are isolated
      4. SYNTHESIS PASS — LLM writes a natural-language answer from tool data
      5. Session history is updated for multi-turn support
    """
    correlation_id: str = req.state.correlation_id
    log = get_logger(correlation_id)

    # Resolve or create session
    session_id = request.session_id or str(uuid4())

    auth_header   = req.headers.get("authorization")
    cookie_header  = req.headers.get("cookie")
    user_id        = req.headers.get("x-user-id")
    user_role      = req.headers.get("x-user-role")

    log.info(
        f"agent_query | session={session_id} | prompt_len={len(request.prompt)} | "
        f"role={user_role}"
    )
    
    print(f"\n[{correlation_id}] === NEW REQUEST: agent_query ===")
    print(f"[{correlation_id}] Prompt: {request.prompt}")

    try:
        print(f"[{correlation_id}] Executing run_agent()...")
        answer, tool_results = await run_agent(
            prompt=request.prompt,
            session_id=session_id,
            auth_header=auth_header,
            cookie_header=cookie_header,
            user_id=user_id,
            user_role=user_role,
            log=log,
        )

        overall_success = all(r.success for r in tool_results)
        errors = [r.error for r in tool_results if r.error]

        return AgentResponseV2(
            success=overall_success,
            answer=answer,
            tool_results=tool_results,
            tools_called=[r.tool for r in tool_results],
            session_id=session_id,
            correlation_id=correlation_id,
            error="; ".join(errors) if errors else None,
        )

    except HTTPException:
        raise
    except Exception as exc:
        log.error(f"Unhandled exception in agent_query: {exc}", exc_info=True)
        raise HTTPException(
            status_code=500,
            detail="Internal server error processing agent query.",
        )


@app.delete("/agent/session/{session_id}")
async def clear_session(session_id: str, req: Request) -> dict:
    """Clear conversation history for a given session."""
    _sessions.pop(session_id, None)
    log = get_logger(req.state.correlation_id)
    log.info(f"Session cleared | session={session_id}")
    return {"cleared": True, "session_id": session_id}


@app.get("/agent/session/{session_id}")
async def get_session(session_id: str) -> dict:
    """Inspect current conversation history for a session (debug/admin use)."""
    history = _get_history(session_id)
    return {"session_id": session_id, "turns": len(history) // 2, "history": history}


@app.get("/health")
async def health_check() -> dict:
    return {"status": "healthy", "version": "2.0.0"}


@app.get("/tools")
async def list_tools() -> dict:
    """Return the full tool registry so front-ends can introspect capabilities."""
    return {"tools": TOOL_REGISTRY}


@app.get("/")
async def root() -> dict:
    return {
        "service": "shopswift-agent-service",
        "version": "2.0.0",
        "description": "Two-pass AI agent for ShopSwift admin dashboard.",
    }