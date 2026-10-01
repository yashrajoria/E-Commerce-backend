import json

from fastapi import APIRouter, HTTPException, Request
from pydantic import BaseModel
from uuid import UUID, uuid4

from app.agent.schemas import AgentQueryRequestV2, AgentResponseV2
from app.agent.orchestrator import run_agent
from app.audit.repository import get_pending, mark_confirmed_and_executed, mark_rejected
from app.core.session import clear_session, get_history
from app.core.logging import get_logger
from app.tools.registry import TOOL_REGISTRY, get_tool_registry_json

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
    return {"tools": get_tool_registry_json()}


class MutationConfirmRequest(BaseModel):
    approve: bool


def _require_admin(req: Request) -> None:
    """Defense-in-depth: api-gateway already gates /agent/* to admin JWTs."""
    if req.headers.get("x-user-role") != "admin":
        raise HTTPException(status_code=403, detail="Admin role required")


def _row_to_dict(row) -> dict:
    data = dict(row)
    for key in ("arguments", "result"):
        if isinstance(data.get(key), str):
            data[key] = json.loads(data[key])
    for key in ("id", "request_id", "user_id", "confirmed_by"):
        if data.get(key) is not None:
            data[key] = str(data[key])
    for key in ("created_at", "updated_at", "confirmed_at"):
        if data.get(key) is not None:
            data[key] = data[key].isoformat()
    return data


@router.get("/agent/mutations/{request_id}")
async def get_mutation(request_id: str, req: Request):
    _require_admin(req)
    try:
        request_uuid = UUID(request_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid request_id")

    row = await get_pending(request_uuid)
    if row is None:
        raise HTTPException(status_code=404, detail="Mutation not found")
    return _row_to_dict(row)


@router.post("/agent/mutations/{request_id}/confirm")
async def confirm_mutation(request_id: str, body: MutationConfirmRequest, req: Request):
    _require_admin(req)
    try:
        request_uuid = UUID(request_id)
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid request_id")

    row = await get_pending(request_uuid)
    if row is None:
        raise HTTPException(status_code=404, detail="Mutation not found")
    if row["status"] != "pending_confirmation":
        raise HTTPException(status_code=409, detail=f"Mutation already {row['status']}")

    confirmed_by = req.headers.get("x-user-id")

    if not body.approve:
        await mark_rejected(request_uuid, confirmed_by)
        return {"request_id": request_id, "status": "rejected"}

    spec = TOOL_REGISTRY.get(row["tool"])
    if spec is None:
        await mark_confirmed_and_executed(request_uuid, confirmed_by, None, "Unknown tool")
        raise HTTPException(status_code=500, detail="Unknown tool")

    arguments = row["arguments"]
    if isinstance(arguments, str):
        arguments = json.loads(arguments)

    try:
        result = await spec.handler(
            params=arguments,
            auth_header=req.headers.get("authorization"),
            cookie_header=req.headers.get("cookie"),
            correlation_id=req.state.correlation_id,
            user_id=confirmed_by,
            user_role=req.headers.get("x-user-role"),
        )
        await mark_confirmed_and_executed(request_uuid, confirmed_by, result, None)
        return {"request_id": request_id, "status": "executed", "result": result}
    except Exception as exc:
        error = str(exc)
        await mark_confirmed_and_executed(request_uuid, confirmed_by, None, error)
        raise HTTPException(status_code=502, detail=f"Mutation execution failed: {error}")
