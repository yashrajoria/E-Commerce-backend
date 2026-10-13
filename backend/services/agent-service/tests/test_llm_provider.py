import pytest
from app.llm.provider import (
    BaseLLMProvider,
    GeminiProvider,
    GroqProvider,
    OllamaProvider,
    OpenAIProvider,
    FallbackProvider,
    get_llm_provider,
)
from app.core.config import settings
from app.core.logging import get_logger


class MockProvider(BaseLLMProvider):
    def __init__(self, answer: str = "mock answer", should_fail: bool = False):
        self.answer = answer
        self.should_fail = should_fail

    async def call(self, messages, logger, json_mode: bool = False):
        if self.should_fail:
            raise RuntimeError("Mock failure")
        return self.answer

    async def stream(self, messages, logger):
        if self.should_fail:
            raise RuntimeError("Mock failure")
        for word in self.answer.split(" "):
            yield word + " "


def test_gemini_message_conversion():
    provider = GeminiProvider(api_key="test-key", model="gemini-2.0-flash")
    messages = [
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello"},
        {"role": "assistant", "content": "Hi there!"},
    ]
    converted = provider._convert_messages(messages)
    assert "systemInstruction" in converted
    assert converted["systemInstruction"]["parts"][0]["text"] == "You are a helpful assistant."
    assert len(converted["contents"]) == 2
    assert converted["contents"][0]["role"] == "user"
    assert converted["contents"][1]["role"] == "model"


def test_groq_provider_configuration():
    provider = GroqProvider(api_key="test-key", model="llama-3.3-70b-versatile")

    assert provider.api_key == "test-key"
    assert provider.model == "llama-3.3-70b-versatile"
    assert provider.base_url == "https://api.groq.com/openai/v1"


def test_groq_provider_selection(monkeypatch):
    monkeypatch.setattr(settings, "LLM_PROVIDER", "groq")
    monkeypatch.setattr(settings, "GROQ_API_KEY", "test-key")

    provider = get_llm_provider()

    assert isinstance(provider, FallbackProvider)
    assert isinstance(provider.primary, GroqProvider)
    assert isinstance(provider.fallback, OllamaProvider)


@pytest.mark.asyncio
async def test_fallback_provider_on_primary_failure():
    logger = get_logger("test-corr-id")
    primary = MockProvider(should_fail=True)
    secondary = MockProvider(answer="secondary success")
    fallback = FallbackProvider(primary, secondary)

    answer = await fallback.call([], logger)
    assert answer == "secondary success"

    streamed = []
    async for chunk in fallback.stream([], logger):
        streamed.append(chunk)
    assert "".join(streamed).strip() == "secondary success"


def test_get_llm_provider_default():
    provider = get_llm_provider()
    # Default without api key should be OllamaProvider
    assert isinstance(provider, (OllamaProvider, FallbackProvider))
