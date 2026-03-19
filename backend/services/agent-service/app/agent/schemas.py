from pydantic import BaseModel, Field, field_validator
from typing import Any, Dict, List, Optional
import re

class ToolCall(BaseModel):
    tool: str
    params: Dict[str, Any] = Field(default_factory=dict)

class ToolResult(BaseModel):
    tool: str
    success: bool
    data: Optional[Any] = None
    error: Optional[str] = None
    summary: Optional[str] = None

class AgentQueryRequestV2(BaseModel):
    prompt: str = Field(..., min_length=1, max_length=2000)
    session_id: Optional[str] = Field(default=None)

    @field_validator("prompt")
    @classmethod
    def sanitize_prompt(cls, v: str) -> str:
        return re.sub(r"\s{3,}", "  ", v.strip())

class AgentResponseV2(BaseModel):
    success: bool
    answer: str
    tool_results: List[ToolResult] = Field(default_factory=list)
    tools_called: List[str] = Field(default_factory=list)
    session_id: str
    correlation_id: str
    error: Optional[str] = None
