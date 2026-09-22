import logging

from app.tools.validator import map_and_validate_calls


def _logger():
    return logging.getLogger("test")


def test_unknown_tool_is_dropped():
    calls = [{"tool": "delete_everything", "params": {}}]
    assert map_and_validate_calls(calls, _logger()) == []


def test_malformed_params_are_dropped_not_passed_through():
    calls = [{"tool": "create_restock_request", "params": {"product_id": "p1", "quantity": -1}}]
    result = map_and_validate_calls(calls, _logger())
    assert result == []


def test_valid_call_is_kept_and_typed():
    calls = [{"tool": "cancel_order", "params": {"order_id": "o1"}}]
    result = map_and_validate_calls(calls, _logger())
    assert len(result) == 1
    assert result[0].tool == "cancel_order"
    assert result[0].params["order_id"] == "o1"
    assert result[0].params["reason"] is None


def test_duplicate_tool_calls_are_deduplicated():
    calls = [
        {"tool": "get_sales", "params": {"range": "7d"}},
        {"tool": "get_sales", "params": {"range": "30d"}},
    ]
    result = map_and_validate_calls(calls, _logger())
    assert len(result) == 1


def test_truncates_to_max_five_tools():
    calls = [{"tool": "get_product_count", "params": {}}] + [
        {"tool": name, "params": {}}
        for name in ["get_failed_payments", "get_sales", "get_low_stock", "get_top_products", "search_products"]
    ]
    # search_products has a required field so it'll drop out; pad with valid ones instead
    calls[-1] = {"tool": "get_customers", "params": {}}
    result = map_and_validate_calls(calls, _logger())
    assert len(result) <= 5
