from app.tools.registry import MUTATING_TOOLS, READ_TOOLS, TOOL_REGISTRY, get_tool_registry_text


def test_every_tool_has_a_handler_and_params_model():
    for name, spec in TOOL_REGISTRY.items():
        assert callable(spec.handler), f"{name} missing a callable handler"
        assert spec.params_model is not None, f"{name} missing a params_model"


def test_mutating_and_read_partitions_cover_the_whole_registry():
    assert set(READ_TOOLS) | set(MUTATING_TOOLS) == set(TOOL_REGISTRY)
    assert set(READ_TOOLS) & set(MUTATING_TOOLS) == set()


def test_mutation_tools_require_admin():
    for name, spec in MUTATING_TOOLS.items():
        assert spec.min_role == "admin", f"{name} is mutating but not admin-gated"


def test_registry_text_round_trips_from_each_params_schema():
    text = get_tool_registry_text()
    for name, spec in TOOL_REGISTRY.items():
        assert f"`{name}`" in text
        schema = spec.params_model.model_json_schema()
        for prop_name in schema.get("properties", {}):
            assert prop_name in text
