"""Optional OpenTelemetry instrumentation for agent-service.

Disabled (no-op) when OTEL_EXPORTER_OTLP_ENDPOINT is unset — the default path
for local dev and CI. When enabled, exports OTLP/HTTP to the collector and
instruments FastAPI, httpx, and asyncpg. Correlation-ID middleware in main.py
is untouched and keeps working alongside traceparent propagation.

Local Jaeger (see services/common/telemetry doc comment in the Go side):
    docker run -d --name jaeger -p 16686:16686 -p 4318:4318 \
      jaegertracing/all-in-one:1.57
    export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
"""

from __future__ import annotations

import os
from typing import Optional

from app.core.logging import logger

_enabled: bool = False
_instrumented_apps: set[int] = set()


def init_telemetry(service_name: str) -> bool:
    """Initialize OpenTelemetry if an endpoint is configured.

    Returns True when tracing is active, False when disabled (no-op).
    Never raises — startup must not fail because of telemetry.
    """
    global _enabled
    endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "").strip()
    if not endpoint:
        logger.info("[telemetry] OTEL_EXPORTER_OTLP_ENDPOINT not set — telemetry disabled for %s", service_name)
        _enabled = False
        return False

    try:
        from opentelemetry import trace
        from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
        from opentelemetry.sdk.resources import Resource
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor
        from opentelemetry.propagation import set_global_textmap
        from opentelemetry.propagation.tracecontext import TraceContextTextMapPropagator

        env = os.getenv("DEPLOY_ENV") or os.getenv("APP_ENV") or "local"
        resource = Resource.create({
            "service.name": service_name,
            "deployment.environment.name": env,
        })
        exporter = OTLPSpanExporter(endpoint=endpoint)
        provider = TracerProvider(resource=resource)
        provider.add_span_processor(BatchSpanProcessor(exporter))
        trace.set_tracer_provider(provider)
        set_global_textmap(TraceContextTextMapPropagator())
        _enabled = True
        logger.info("[telemetry] enabled for %s (endpoint=%s, env=%s)", service_name, endpoint, env)
        return True
    except Exception as exc:  # noqa: BLE001 — telemetry must never break startup
        logger.warning("[telemetry] init failed for %s: %s (telemetry disabled)", service_name, exc)
        _enabled = False
        return False


def is_enabled() -> bool:
    return _enabled


def instrument_app(app) -> None:
    """Instrument a FastAPI app with OTel middleware. No-op when disabled."""
    if not _enabled:
        return
    app_id = id(app)
    if app_id in _instrumented_apps:
        return
    try:
        from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

        FastAPIInstrumentor.instrument_app(app)
        _instrumented_apps.add(app_id)
    except Exception as exc:  # noqa: BLE001
        logger.warning("[telemetry] FastAPI instrumentation failed: %s", exc)


def instrument_httpx_client(client) -> None:
    """Instrument an httpx.AsyncClient. No-op when disabled."""
    if not _enabled or client is None:
        return
    try:
        from opentelemetry.instrumentation.httpx import HTTPXClientInstrumentor

        HTTPXClientInstrumentor().instrument_client(client)
    except Exception as exc:  # noqa: BLE001
        logger.warning("[telemetry] httpx instrumentation failed: %s", exc)


def instrument_asyncpg_pool(pool) -> None:
    """Instrument an asyncpg connection pool. No-op when disabled."""
    if not _enabled or pool is None:
        return
    try:
        from opentelemetry.instrumentation.asyncpg import AsyncPGInstrumentor

        AsyncPGInstrumentor().instrument()
    except Exception as exc:  # noqa: BLE001
        logger.warning("[telemetry] asyncpg instrumentation failed: %s", exc)
