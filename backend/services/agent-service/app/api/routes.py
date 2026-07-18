from fastapi import APIRouter, HTTPException, Request
from uuid import uuid4
from app.agent.schemas import AgentQueryRequestV2, AgentResponseV2
from app.agent.orchestrator import run_agent
from app.core.session import clear_session, get_history
from app.core.logging import get_logger
from app.tools.registry import TOOL_REGISTRY

router = APIRouter()

@router.post("/agent/query", response_model=AgentResponseV2)
async def query_agent(request: AgentQueryRequestV2, req: Request) -> AgentResponseV2:
    correlation_id: str = req.state.correlation_id
    logger = get_logger(correlation_id)
    session_id = request.session_id or str(uuid4())

    auth = req.headers.get("authorization")
    cookie = req.headers.get("cookie")
    user_id = req.headers.get("x-user-id")
    user_role = req.headers.get("x-user-role", "guest")

    logger.info(
        f"Query received | Session={session_id} Role={user_role}",
        extra={"event": "request.start"},
    )

    try:
        response = await run_agent(
            prompt=request.prompt,
            session_id=session_id,
            auth_header=auth,
            cookie_header=cookie,
            user_id=user_id,
            user_role=user_role,
            logger=logger,
            correlation_id=correlation_id,
        )
        return AgentResponseV2(**response)

    except HTTPException:
        raise
    except Exception as exc:
        logger.error(
            f"Agent pipeline crash: {exc}",
            extra={"event": "request.failure"},
            exc_info=True,
        )
        raise HTTPException(status_code=500, detail="Internal server error")

@router.delete("/agent/session/{session_id}")
async def flush_session(session_id: str, req: Request):
    cleared = clear_session(session_id)
    return {"cleared": cleared, "session_id": session_id}

@router.get("/agent/session/{session_id}")
async def inspect_session(session_id: str):
    history = get_history(session_id)
    return {"session_id": session_id, "turns": len(history) // 2, "history": history}

@router.get("/health")
async def health_check():
    return {"status": "healthy", "version": "3.0.0"}

@router.get("/tools")
@router.get("/agent/tools")
async def list_tools():
    return {"tools": TOOL_REGISTRY}
