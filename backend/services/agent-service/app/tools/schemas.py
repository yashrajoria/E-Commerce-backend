"""Typed parameter models for every tool the LLM may call.

This is the actual type boundary between the LLM's free-text tool proposals
and any HTTP/DB call: a proposal is only ever executed after
`params_model.model_validate(...)` succeeds (see app/tools/validator.py).
"""

from typing import Optional

from pydantic import BaseModel, Field


class EmptyParams(BaseModel):
    """Params model for tools that take no arguments."""


class GetSalesParams(BaseModel):
    range: str = Field(default="30d", pattern=r"^\d{1,3}[dy]$")


class GetTopProductsParams(BaseModel):
    range: str = Field(default="30d", pattern=r"^\d{1,3}[dy]$")
    limit: int = Field(default=5, ge=1, le=100)


class GetLowStockParams(BaseModel):
    threshold: int = Field(default=10, ge=0, le=1_000_000)


class GetOrdersParams(BaseModel):
    status: str = Field(default="all", pattern=r"^(pending|shipped|cancelled|paid|completed|all)$")
    range: Optional[str] = Field(default=None, pattern=r"^\d{1,3}d$")
    days: Optional[int] = Field(default=None, ge=1, le=3650)
    page: int = Field(default=1, ge=1)
    limit: int = Field(default=20, ge=1, le=100)


class GetCustomersParams(BaseModel):
    range: str = Field(default="30d", pattern=r"^\d{1,3}[dy]$")
    metric: str = Field(default="all", pattern=r"^(new|returning|all)$")


class GetRevenueBreakdownParams(BaseModel):
    group_by: str = Field(default="category", pattern=r"^(category|channel|region)$")
    range: str = Field(default="30d", pattern=r"^\d{1,3}[dy]$")


class SearchProductsParams(BaseModel):
    query: str = Field(..., min_length=1, max_length=200)


class CancelOrderParams(BaseModel):
    order_id: str = Field(..., min_length=1, max_length=64)
    reason: Optional[str] = Field(default=None, max_length=500)


class CreateRestockRequestParams(BaseModel):
    product_id: str = Field(..., min_length=1, max_length=64)
    quantity: int = Field(..., gt=0, le=100_000)
