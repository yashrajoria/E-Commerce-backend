import os

def _safe_float(val: str | None, default: float) -> float:
    if val is None:
        return default
    try:
        return float(val)
    except (ValueError, TypeError):
        return default

def _safe_int(val: str | None, default: int) -> int:
    if val is None:
        return default
    try:
        return int(val)
    except (ValueError, TypeError):
        return default

class Settings:
    APP_NAME: str = "ShopSwift Agent Service"
    SERVICE_VERSION: str = "4.0.0"
    ENVIRONMENT: str = os.getenv("ENVIRONMENT", "production")
    
    LLM_PROVIDER: str = os.getenv("LLM_PROVIDER", "ollama").lower()  # ollama | gemini | openai | auto
    LLM_SERVICE_URL: str = os.getenv("LLM_SERVICE_URL", "http://ollama:11434/api/chat")
    LLM_MODEL: str = os.getenv("LLM_MODEL", "qwen2.5:7b")
    LLM_TIMEOUT: float = _safe_float(os.getenv("LLM_TIMEOUT"), 30.0)

    # Cloud LLM Provider configurations (Gemini / OpenAI compatible)
    GEMINI_API_KEY: str | None = os.getenv("GEMINI_API_KEY")
    GEMINI_MODEL: str = os.getenv("GEMINI_MODEL", "gemini-2.0-flash")
    OPENAI_API_KEY: str | None = os.getenv("OPENAI_API_KEY")
    OPENAI_BASE_URL: str = os.getenv("OPENAI_BASE_URL", "https://api.openai.com/v1")
    OPENAI_MODEL: str = os.getenv("OPENAI_MODEL", "gpt-4o-mini")
    ENABLE_STREAMING: bool = os.getenv("ENABLE_STREAMING", "true").lower() in ("true", "1", "yes")
    
    TOOL_API_BASE_URL: str = os.getenv("TOOL_API_BASE_URL", "http://api-gateway:8080")
    BFF_TIMEOUT: float = _safe_float(os.getenv("BFF_TIMEOUT"), 15.0)
    TOOL_TIMEOUT: float = _safe_float(os.getenv("TOOL_TIMEOUT"), 20.0)
    MAX_CONCURRENT_TOOLS: int = _safe_int(os.getenv("MAX_CONCURRENT_TOOLS"), 5)
    MAX_HISTORY_TURNS: int = _safe_int(os.getenv("MAX_HISTORY_TURNS"), 10)

    # Postgres — reuses the same root .env vars every other Go service connects
    # with (see backend/.env), for the agent_audit_log table.
    POSTGRES_HOST: str = os.getenv("POSTGRES_HOST", "postgres")
    POSTGRES_PORT: int = _safe_int(os.getenv("POSTGRES_PORT"), 5432)
    POSTGRES_USER: str = os.getenv("POSTGRES_USER", "postgres")
    POSTGRES_PASSWORD: str = os.getenv("POSTGRES_PASSWORD", "postgres")
    POSTGRES_DB: str = os.getenv("POSTGRES_DB", "ecommerce")

    @property
    def postgres_dsn(self) -> str:
        return (
            f"postgresql://{self.POSTGRES_USER}:{self.POSTGRES_PASSWORD}"
            f"@{self.POSTGRES_HOST}:{self.POSTGRES_PORT}/{self.POSTGRES_DB}"
        )

    @property
    def is_production(self) -> bool:
        return self.ENVIRONMENT.lower() == "production"

settings = Settings()
