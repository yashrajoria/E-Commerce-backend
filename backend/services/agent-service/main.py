"""
FastAPI agent service for ShopSwift admin dashboard.
Implements single-shot routing with concurrent tool execution.
"""

import asyncio
import json
import logging
from typing import Optional, List, AsyncGenerator
from uuid import uuid4
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request, HTTPException
from fastapi.responses import JSONResponse
import uvicorn

from models import AgentQueryRequest, AgentResponse, ToolCall, ToolResult
from tools import execute_tool, close_http_client

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator:
    """
    Manage application lifecycle.
    Handles startup and shutdown of the HTTP client singleton.
    """
    # Startup: nothing to do, client is created on first request
    logger.info("Agent Service starting up")
    yield
    # Shutdown: close the HTTP client gracefully
    logger.info("Agent Service shutting down")
    await close_http_client()


app = FastAPI(
    title="Agent Service",
    description="Intelligence layer for ShopSwift admin dashboard",
    version="1.0.0",
    lifespan=lifespan,
)


@app.middleware("http")
async def add_correlation_id(request: Request, call_next):
    """
    Middleware to ensure all requests have a correlation ID for tracing.
    """
    correlation_id = request.headers.get("X-Correlation-ID", str(uuid4()))
    request.state.correlation_id = correlation_id
    response = await call_next(request)
    response.headers["X-Correlation-ID"] = correlation_id
    return response


@app.post("/agent/query")
async def agent_query(request: AgentQueryRequest, req: Request) -> AgentResponse:
    """
    Main agent endpoint. Accepts a user prompt, calls the LLM to get tool calls,
    executes them concurrently, and returns aggregated results.
    
    Args:
        request: AgentQueryRequest containing the user prompt
        req: FastAPI request object for extracting headers
        
    Returns:
        AgentResponse with aggregated tool results
    """
    correlation_id = req.state.correlation_id
    auth_header = req.headers.get("authorization")
    
    logger.info(
        f"Received agent query | correlation_id={correlation_id} | "
        f"prompt_length={len(request.prompt)}"
    )
    
    try:
        # Step 1: Call the LLM to get tool calls (placeholder - returns mock data)
        tool_calls = await call_llm(request.prompt)
        logger.info(
            f"LLM returned {len(tool_calls)} tool calls | "
            f"correlation_id={correlation_id}"
        )
        
        # Step 2: Parse LLM response into ToolCall objects
        try:
            parsed_calls = [ToolCall(**call) for call in tool_calls]
        except Exception as e:
            logger.error(
                f"Failed to parse LLM tool calls | correlation_id={correlation_id} | "
                f"error={str(e)}"
            )
            raise HTTPException(
                status_code=400,
                detail="Invalid tool call format from LLM",
            )
        
        # Step 3: Execute all tools concurrently using asyncio.gather()
        logger.info(f"Starting concurrent tool execution | correlation_id={correlation_id}")
        execution_tasks = [
            execute_tool(
                tool_name=call.tool,
                params=call.params,
                auth_header=auth_header,
                correlation_id=correlation_id,
            )
            for call in parsed_calls
        ]
        
        results = await asyncio.gather(*execution_tasks, return_exceptions=False)
        
        # Step 4: Convert results to ToolResult objects
        tool_results = [ToolResult(**result) for result in results]
        
        # Step 5: Determine overall success and aggregate any errors
        overall_success = all(result.success for result in tool_results)
        errors = [result.error for result in tool_results if result.error]
        error_message = "; ".join(errors) if errors else None
        
        if not overall_success:
            logger.warning(
                f"Some tools failed during execution | "
                f"correlation_id={correlation_id} | errors={error_message}"
            )
        
        response = AgentResponse(
            success=overall_success,
            data=tool_results,
            error=error_message,
            correlation_id=correlation_id,
        )
        
        logger.info(
            f"Agent query completed successfully | "
            f"correlation_id={correlation_id} | success={overall_success}"
        )
        
        return response
        
    except HTTPException:
        raise
    except Exception as e:
        logger.error(
            f"Unexpected error in agent query | correlation_id={correlation_id} | "
            f"error={str(e)}",
            exc_info=True,
        )
        raise HTTPException(
            status_code=500,
            detail="Internal server error processing agent query",
        )


@app.get("/health")
async def health_check() -> dict:
    """
    Health check endpoint for deployment/monitoring.
    """
    return {"status": "healthy"}


@app.get("/")
async def root() -> dict:
    """
    Root endpoint providing service information.
    """
    return {
        "service": "agent-service",
        "version": "1.0.0",
        "description": "Intelligence layer for ShopSwift admin dashboard",
    }


async def call_llm(prompt: str) -> List[dict]:
    """
    Placeholder for calling the remote LLM service.
    In production, this would call an external LLM via HTTP or streaming service.
    
    Currently returns mock tool calls for demonstration.
    
    Args:
        prompt: The user query/prompt
        
    Returns:
        List of tool calls in the format:
        [
            {"tool": "tool_name", "params": {...}},
            ...
        ]
    """
    logger.info(f"Calling LLM with prompt: {prompt[:100]}...")
    
    # TODO: Implement actual LLM call
    # Example:
    # async with httpx.AsyncClient() as client:
    #     response = await client.post(
    #         "https://llm-service/v1/query",
    #         json={"prompt": prompt},
    #         timeout=30.0,
    #     )
    #     response.raise_for_status()
    #     return response.json()["tool_calls"]
    
    # Mock response based on prompt keywords
    mock_tool_calls = [
        {
            "tool": "get_sales",
            "params": {"range": "7d"}
        },
        {
            "tool": "get_top_products",
            "params": {"range": "7d", "limit": 5}
        },
    ]
    
    # Add additional tools based on prompt content
    if "failed" in prompt.lower() or "payment" in prompt.lower():
        mock_tool_calls.append({
            "tool": "get_failed_payments",
            "params": {}
        })
    
    if "low" in prompt.lower() or "inventory" in prompt.lower() or "stock" in prompt.lower():
        mock_tool_calls.append({
            "tool": "get_low_stock",
            "params": {"threshold": 10}
        })
    
    logger.info(f"LLM returning {len(mock_tool_calls)} mock tool calls")
    
    return mock_tool_calls


if __name__ == "__main__":
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=8000,
        log_level="info",
    )
