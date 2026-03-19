from fastapi import FastAPI, Request
from uuid import uuid4
from contextlib import asynccontextmanager
from app.core.config import settings
from app.core.logging import logger
from app.api.routes import router
from app.tools.executor import close_http_client
from app.tools.executor import get_http_client

@asynccontextmanager
async def lifespan(application: FastAPI):
    logger.info(f"Starting {settings.APP_NAME}")
    await get_http_client()
    yield
    await close_http_client()
    logger.info(f"Shutting down {settings.APP_NAME}")

app = FastAPI(
    title=settings.APP_NAME,
    description="Modular Two-Pass AI Agent Architecture.",
    version="3.0.0",
    lifespan=lifespan,
)

@app.middleware("http")
async def correlation_middleware(request: Request, call_next):
    """Ensure every trace boundary has a UUID."""
    correlation_id = request.headers.get("X-Correlation-ID", str(uuid4()))
    request.state.correlation_id = correlation_id
    response = await call_next(request)
    response.headers["X-Correlation-ID"] = correlation_id
    return response

app.include_router(router)
