"""
Configuration for the Agent Service.
Loads settings from environment variables with sensible defaults.
"""

import os
from typing import Optional


class Settings:
    """Application settings loaded from environment variables."""
    
    # Service configuration
    SERVICE_NAME: str = "agent-service"
    SERVICE_VERSION: str = "1.0.0"
    ENVIRONMENT: str = os.getenv("ENVIRONMENT", "development")
    
    # API configuration
    HOST: str = os.getenv("HOST", "0.0.0.0")
    PORT: int = int(os.getenv("PORT", "8000"))
    LOG_LEVEL: str = os.getenv("LOG_LEVEL", "info")
    
    # BFF service configuration
    BFF_BASE_URL: str = os.getenv("BFF_BASE_URL", "http://bff-service:8080")
    BFF_TIMEOUT: float = float(os.getenv("BFF_TIMEOUT", "15.0"))
    
    # LLM service configuration (for future use)
    LLM_SERVICE_URL: Optional[str] = os.getenv("LLM_SERVICE_URL")
    LLM_TIMEOUT: float = float(os.getenv("LLM_TIMEOUT", "30.0"))
    
    # Request configuration
    CONCURRENT_TOOL_LIMIT: int = int(os.getenv("CONCURRENT_TOOL_LIMIT", "10"))
    
    @property
    def is_production(self) -> bool:
        """Check if running in production environment."""
        return self.ENVIRONMENT.lower() == "production"
    
    def __repr__(self) -> str:
        return (
            f"Settings(service={self.SERVICE_NAME}, "
            f"version={self.SERVICE_VERSION}, "
            f"environment={self.ENVIRONMENT}, "
            f"bff_url={self.BFF_BASE_URL})"
        )


# Global settings instance
settings = Settings()
