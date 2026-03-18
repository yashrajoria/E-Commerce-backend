"""
Pydantic models for the Agent Service.
Defines schemas for requests, LLM responses, and standardized API responses.
"""

from typing import Any, Optional, Dict, List
from pydantic import BaseModel, Field


class ToolCall(BaseModel):
    """Represents a single tool call returned by the LLM."""
    tool: str = Field(..., description="The tool name to execute (e.g., get_sales, get_top_products)")
    params: Dict[str, Any] = Field(default_factory=dict, description="Parameters for the tool")


class AgentQueryRequest(BaseModel):
    """Incoming request to the agent service."""
    prompt: str = Field(..., description="User prompt or query for the agent")


class ToolResult(BaseModel):
    """Result from a single tool execution."""
    tool: str = Field(..., description="The tool name that was executed")
    success: bool = Field(..., description="Whether the tool execution succeeded")
    data: Optional[Any] = Field(default=None, description="The result data from the tool")
    error: Optional[str] = Field(default=None, description="Error message if execution failed")


class AgentResponse(BaseModel):
    """Standardized response from the agent service."""
    success: bool = Field(..., description="Overall success status of the agent query")
    data: List[ToolResult] = Field(default_factory=list, description="Aggregated tool results")
    error: Optional[str] = Field(default=None, description="Overall error message if present")
    correlation_id: Optional[str] = Field(default=None, description="Trace ID for request tracking")


class SalesData(BaseModel):
    """Structured response for sales data from BFF."""
    range: str
    total_sales: float
    transaction_count: int
    average_transaction_value: float


class TopProduct(BaseModel):
    """Represents a top-selling product."""
    product_id: str
    name: str
    sales: int
    revenue: float


class TopProductsData(BaseModel):
    """Structured response for top products from BFF."""
    range: str
    limit: int
    products: List[TopProduct]


class LowStockItem(BaseModel):
    """Represents a product with low stock."""
    product_id: str
    name: str
    current_stock: int
    threshold: int


class LowStockData(BaseModel):
    """Structured response for low stock items from BFF."""
    threshold: int
    items: List[LowStockItem]


class FailedPayment(BaseModel):
    """Represents a failed payment transaction."""
    transaction_id: str
    order_id: str
    amount: float
    reason: str
    timestamp: str


class FailedPaymentsData(BaseModel):
    """Structured response for failed payments from BFF."""
    count: int
    payments: List[FailedPayment]
