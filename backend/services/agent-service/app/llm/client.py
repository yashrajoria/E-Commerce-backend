from typing import AsyncIterator, Dict, List

from app.core.logging import CorrelationAdapter
from app.llm.provider import get_llm_provider


async def call_llm(
    messages: List[Dict[str, str]],
    logger: CorrelationAdapter,
    json_mode: bool = False,
) -> str:
    """Execute LLM chat query using the configured provider (Ollama, Gemini, OpenAI, or Fallback)."""
    provider = get_llm_provider()
    return await provider.call(messages, logger, json_mode=json_mode)


async def stream_llm(
    messages: List[Dict[str, str]],
    logger: CorrelationAdapter,
) -> AsyncIterator[str]:
    """Stream LLM response token-by-token."""
    provider = get_llm_provider()
    async for chunk in provider.stream(messages, logger):
        yield chunk


async def call_ollama(
    messages: List[Dict[str, str]],
    logger: CorrelationAdapter,
    json_mode: bool = False,
) -> str:
    """Backward-compatible wrapper for legacy call_ollama callers."""
    return await call_llm(messages, logger, json_mode=json_mode)
