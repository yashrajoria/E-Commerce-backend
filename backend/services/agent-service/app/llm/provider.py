import json
from abc import ABC, abstractmethod
from typing import Any, AsyncIterator, Dict, List, Optional

import httpx

from app.core.config import settings
from app.core.logging import CorrelationAdapter


class BaseLLMProvider(ABC):
    @abstractmethod
    async def call(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
        json_mode: bool = False,
    ) -> str:
        """Execute a non-streaming chat completion."""
        pass

    @abstractmethod
    async def stream(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
    ) -> AsyncIterator[str]:
        """Stream chat completion token chunks."""
        pass


class OllamaProvider(BaseLLMProvider):
    def __init__(
        self,
        service_url: Optional[str] = None,
        model: Optional[str] = None,
        timeout: Optional[float] = None,
    ):
        self.service_url = service_url or settings.LLM_SERVICE_URL
        self.model = model or settings.LLM_MODEL
        self.timeout = timeout or settings.LLM_TIMEOUT

    async def call(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
        json_mode: bool = False,
    ) -> str:
        payload: Dict[str, Any] = {
            "model": self.model,
            "messages": messages,
            "stream": False,
            "options": {"temperature": 0.0},
        }
        if json_mode:
            payload["format"] = "json"

        logger.info(
            f"Ollama call started | model={self.model} json_mode={json_mode}",
            extra={"event": "llm.call", "provider": "ollama"},
        )

        async with httpx.AsyncClient() as client:
            response = await client.post(
                self.service_url,
                json=payload,
                timeout=self.timeout,
            )
            response.raise_for_status()

        content = response.json().get("message", {}).get("content", "")
        logger.info(
            f"Ollama call complete | content_length={len(content)}",
            extra={"event": "llm.response", "provider": "ollama"},
        )
        return content

    async def stream(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
    ) -> AsyncIterator[str]:
        payload: Dict[str, Any] = {
            "model": self.model,
            "messages": messages,
            "stream": True,
            "options": {"temperature": 0.0},
        }

        logger.info(
            f"Ollama stream started | model={self.model}",
            extra={"event": "llm.stream_start", "provider": "ollama"},
        )

        async with httpx.AsyncClient() as client:
            async with client.stream(
                "POST",
                self.service_url,
                json=payload,
                timeout=self.timeout,
            ) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if not line:
                        continue
                    try:
                        chunk_json = json.loads(line)
                        chunk = chunk_json.get("message", {}).get("content", "")
                        if chunk:
                            yield chunk
                    except json.JSONDecodeError:
                        continue


class GeminiProvider(BaseLLMProvider):
    def __init__(
        self,
        api_key: Optional[str] = None,
        model: Optional[str] = None,
        timeout: Optional[float] = None,
    ):
        self.api_key = api_key or settings.GEMINI_API_KEY or ""
        self.model = model or settings.GEMINI_MODEL
        self.timeout = timeout or settings.LLM_TIMEOUT

    def _convert_messages(self, messages: List[Dict[str, str]]) -> Dict[str, Any]:
        """Convert standard role/content messages to Gemini contents + systemInstruction."""
        contents: List[Dict[str, Any]] = []
        system_text: Optional[str] = None

        for msg in messages:
            role = msg.get("role", "user")
            content = msg.get("content", "")
            if role == "system":
                system_text = content
            elif role in ("assistant", "model"):
                contents.append({"role": "model", "parts": [{"text": content}]})
            else:
                contents.append({"role": "user", "parts": [{"text": content}]})

        body: Dict[str, Any] = {"contents": contents}
        if system_text:
            body["systemInstruction"] = {"parts": [{"text": system_text}]}
        return body

    async def call(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
        json_mode: bool = False,
    ) -> str:
        body = self._convert_messages(messages)
        if json_mode:
            body["generationConfig"] = {"responseMimeType": "application/json"}

        url = f"https://generativelanguage.googleapis.com/v1beta/models/{self.model}:generateContent?key={self.api_key}"

        logger.info(
            f"Gemini call started | model={self.model} json_mode={json_mode}",
            extra={"event": "llm.call", "provider": "gemini"},
        )

        async with httpx.AsyncClient() as client:
            response = await client.post(url, json=body, timeout=self.timeout)
            response.raise_for_status()

        data = response.json()
        candidates = data.get("candidates", [])
        if not candidates:
            return ""
        parts = candidates[0].get("content", {}).get("parts", [])
        content = "".join(p.get("text", "") for p in parts)
        return content

    async def stream(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
    ) -> AsyncIterator[str]:
        body = self._convert_messages(messages)
        url = f"https://generativelanguage.googleapis.com/v1beta/models/{self.model}:streamGenerateContent?alt=sse&key={self.api_key}"

        logger.info(
            f"Gemini stream started | model={self.model}",
            extra={"event": "llm.stream_start", "provider": "gemini"},
        )

        async with httpx.AsyncClient() as client:
            async with client.stream("POST", url, json=body, timeout=self.timeout) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if not line.startswith("data: "):
                        continue
                    payload = line[6:].strip()
                    if not payload:
                        continue
                    try:
                        data = json.loads(payload)
                        candidates = data.get("candidates", [])
                        if candidates:
                            parts = candidates[0].get("content", {}).get("parts", [])
                            for p in parts:
                                txt = p.get("text", "")
                                if txt:
                                    yield txt
                    except json.JSONDecodeError:
                        continue


class OpenAIProvider(BaseLLMProvider):
    def __init__(
        self,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None,
        model: Optional[str] = None,
        timeout: Optional[float] = None,
    ):
        self.api_key = api_key or settings.OPENAI_API_KEY or ""
        self.base_url = (base_url or settings.OPENAI_BASE_URL).rstrip("/")
        self.model = model or settings.OPENAI_MODEL
        self.timeout = timeout or settings.LLM_TIMEOUT

    async def call(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
        json_mode: bool = False,
    ) -> str:
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }
        payload: Dict[str, Any] = {
            "model": self.model,
            "messages": messages,
            "temperature": 0.0,
            "stream": False,
        }
        if json_mode:
            payload["response_format"] = {"type": "json_object"}

        logger.info(
            f"OpenAI call started | model={self.model} json_mode={json_mode}",
            extra={"event": "llm.call", "provider": "openai"},
        )

        url = f"{self.base_url}/chat/completions"
        async with httpx.AsyncClient() as client:
            response = await client.post(url, headers=headers, json=payload, timeout=self.timeout)
            response.raise_for_status()

        data = response.json()
        choices = data.get("choices", [])
        if not choices:
            return ""
        return choices[0].get("message", {}).get("content", "")

    async def stream(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
    ) -> AsyncIterator[str]:
        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }
        payload = {
            "model": self.model,
            "messages": messages,
            "temperature": 0.0,
            "stream": True,
        }

        url = f"{self.base_url}/chat/completions"
        async with httpx.AsyncClient() as client:
            async with client.stream("POST", url, headers=headers, json=payload, timeout=self.timeout) as response:
                response.raise_for_status()
                async for line in response.aiter_lines():
                    if not line.startswith("data: "):
                        continue
                    chunk_str = line[6:].strip()
                    if chunk_str == "[DONE]":
                        break
                    try:
                        chunk_json = json.loads(chunk_str)
                        choices = chunk_json.get("choices", [])
                        if choices:
                            delta = choices[0].get("delta", {}).get("content", "")
                            if delta:
                                yield delta
                    except json.JSONDecodeError:
                        continue


class FallbackProvider(BaseLLMProvider):
    def __init__(self, primary: BaseLLMProvider, fallback: BaseLLMProvider):
        self.primary = primary
        self.fallback = fallback

    async def call(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
        json_mode: bool = False,
    ) -> str:
        try:
            return await self.primary.call(messages, logger, json_mode=json_mode)
        except Exception as exc:
            logger.warning(
                f"Primary LLM provider failed, falling back to secondary: {exc}",
                extra={"event": "llm.fallback_triggered"},
            )
            return await self.fallback.call(messages, logger, json_mode=json_mode)

    async def stream(
        self,
        messages: List[Dict[str, str]],
        logger: CorrelationAdapter,
    ) -> AsyncIterator[str]:
        try:
            async for chunk in self.primary.stream(messages, logger):
                yield chunk
        except Exception as exc:
            logger.warning(
                f"Primary stream failed, falling back to secondary: {exc}",
                extra={"event": "llm.fallback_stream_triggered"},
            )
            async for chunk in self.fallback.stream(messages, logger):
                yield chunk


def get_llm_provider() -> BaseLLMProvider:
    provider_name = settings.LLM_PROVIDER

    # Explicit provider selection
    if provider_name == "gemini" and settings.GEMINI_API_KEY:
        return FallbackProvider(GeminiProvider(), OllamaProvider())
    elif provider_name == "openai" and settings.OPENAI_API_KEY:
        return FallbackProvider(OpenAIProvider(), OllamaProvider())
    elif provider_name == "auto":
        if settings.GEMINI_API_KEY:
            return FallbackProvider(GeminiProvider(), OllamaProvider())
        elif settings.OPENAI_API_KEY:
            return FallbackProvider(OpenAIProvider(), OllamaProvider())
        return OllamaProvider()

    # Default to OllamaProvider
    return OllamaProvider()
