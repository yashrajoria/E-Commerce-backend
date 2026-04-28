import os

class Settings:
    APP_NAME: str = "ShopSwift Agent Service"
    SERVICE_VERSION: str = "4.0.0"
    ENVIRONMENT: str = os.getenv("ENVIRONMENT", "production")
    
    LLM_SERVICE_URL: str = os.getenv("LLM_SERVICE_URL", "http://ollama:11434/api/chat")
    LLM_MODEL: str = os.getenv("LLM_MODEL", "qwen2.5:7b")
    LLM_TIMEOUT: float = float(os.getenv("LLM_TIMEOUT", "30.0"))
    
    TOOL_API_BASE_URL: str = os.getenv("TOOL_API_BASE_URL", "http://api-gateway:8080")
    BFF_BASE_URL: str = os.getenv("BFF_BASE_URL", "http://bff-service:8088")
    BFF_TIMEOUT: float = float(os.getenv("BFF_TIMEOUT", "15.0"))
    TOOL_TIMEOUT: float = float(os.getenv("TOOL_TIMEOUT", "20.0"))
    MAX_CONCURRENT_TOOLS: int = int(os.getenv("MAX_CONCURRENT_TOOLS", "5"))
    MAX_HISTORY_TURNS: int = int(os.getenv("MAX_HISTORY_TURNS", "10"))

    @property
    def is_production(self) -> bool:
        return self.ENVIRONMENT.lower() == "production"

settings = Settings()
