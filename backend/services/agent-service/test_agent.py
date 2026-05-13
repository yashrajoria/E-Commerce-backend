import json
import os
import urllib.error
import urllib.request

import pytest


@pytest.mark.skipif(
    os.getenv("RUN_AGENT_LIVE_TEST") != "true",
    reason="requires a running agent-service; set RUN_AGENT_LIVE_TEST=true to enable",
)
def test_agent_query_live():
    payload = json.dumps({"prompt": "how many products do we have?"}).encode()
    req = urllib.request.Request(
        "http://127.0.0.1:8000/agent/query",
        data=payload,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            assert resp.status == 200
            assert resp.read()
    except urllib.error.URLError as exc:
        pytest.fail(f"agent-service request failed: {exc}")
