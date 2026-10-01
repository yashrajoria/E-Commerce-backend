import pytest
from pydantic import ValidationError

from app.tools.schemas import (
    CancelOrderParams,
    CreateRestockRequestParams,
    GetOrdersParams,
    GetSalesParams,
)


def test_cancel_order_requires_order_id():
    with pytest.raises(ValidationError):
        CancelOrderParams()


def test_cancel_order_accepts_optional_reason():
    params = CancelOrderParams(order_id="abc-123", reason="customer request")
    assert params.order_id == "abc-123"
    assert params.reason == "customer request"


def test_create_restock_request_rejects_negative_quantity():
    with pytest.raises(ValidationError):
        CreateRestockRequestParams(product_id="p1", quantity=-5)


def test_create_restock_request_rejects_zero_quantity():
    with pytest.raises(ValidationError):
        CreateRestockRequestParams(product_id="p1", quantity=0)


def test_create_restock_request_accepts_valid_quantity():
    params = CreateRestockRequestParams(product_id="p1", quantity=50)
    assert params.quantity == 50


def test_get_orders_rejects_unknown_status_literal():
    with pytest.raises(ValidationError):
        GetOrdersParams(status="not-a-real-status")


def test_get_orders_defaults():
    params = GetOrdersParams()
    assert params.status == "all"
    assert params.limit == 20


def test_get_sales_rejects_malformed_range():
    with pytest.raises(ValidationError):
        GetSalesParams(range="not-a-range")


def test_get_sales_accepts_valid_range():
    assert GetSalesParams(range="7d").range == "7d"
