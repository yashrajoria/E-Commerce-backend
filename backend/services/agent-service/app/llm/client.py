import httpx
from typing import List, Dict, Any
from app.core.config import settings
from app.core.logging import CorrelationAdapter

async def call_ollama(
    messages: List[Dict[str, str]], 
    logger: CorrelationAdapter, 
    json_mode: bool = False
) -> str:
    llm_url = settings.LLM_SERVICE_URL
    model = settings.LLM_MODEL

    payload: Dict[str, Any] = {
        "model": model,
        "messages": messages,
        "stream": False,
        "options": {"temperature": 0.0},
    }
    if json_mode:
        payload["format"] = "json"

    logger.info(
        f"LLM call started | model={model} json_mode={json_mode}",
        extra={"event": "llm.call"},
    )

    try:
        async with httpx.AsyncClient() as client:
            response = await client.post(
                llm_url,
                json=payload,
                timeout=settings.LLM_TIMEOUT,
            )
            response.raise_for_status()

        content = response.json().get("message", {}).get("content", "")
        logger.info(
            f"LLM call complete | content_length={len(content)}",
            extra={"event": "llm.response", "status_code": response.status_code},
        )
        return content
    except Exception as e:
        logger.error(f"Ollama call failed: {repr(e)}", extra={"event": "llm.failure"}, exc_info=True)
        raise
