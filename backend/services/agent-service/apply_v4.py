import os

FILES = {}

FILES["app/core/config.py"] = '''import os

class Settings:
    APP_NAME: str = "ShopSwift Agent Service"
    SERVICE_VERSION: str = "4.0.0"
    ENVIRONMENT: str = os.getenv("ENVIRONMENT", "production")
    
    LLM_SERVICE_URL: str = os.getenv("LLM_SERVICE_URL", "http://ollama:11434/api/chat")
    LLM_MODEL: str = os.getenv("LLM_MODEL", "qwen2.5:7b")
    LLM_TIMEOUT: float = float(os.getenv("LLM_TIMEOUT", "30.0"))
    
    BFF_BASE_URL: str = os.getenv("BFF_BASE_URL", "http://bff-service:8088")
    BFF_TIMEOUT: float = float(os.getenv("BFF_TIMEOUT", "15.0"))
    TOOL_TIMEOUT: float = 15.0
    MAX_CONCURRENT_TOOLS: int = 5

    @property
    def is_production(self) -> bool:
        return self.ENVIRONMENT.lower() == "production"

settings = Settings()
'''

FILES["app/agent/schemas.py"] = '''from pydantic import BaseModel, Field, field_validator
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
        return re.sub(r"\\s{3,}", "  ", v.strip())

class AgentResponseV2(BaseModel):
    success: bool
    answer: str
    tool_results: List[ToolResult] = Field(default_factory=list)
    tools_called: List[str] = Field(default_factory=list)
    session_id: str
    correlation_id: str
    error: Optional[str] = None
'''

FILES["app/agent/intent.py"] = '''import re
from typing import List, Dict
from app.agent.schemas import ToolCall
from app.agent.planner import plan_tools
from app.core.logging import CorrelationAdapter

async def map_intent(prompt: str, history: List[Dict[str, str]], logger: CorrelationAdapter) -> List[ToolCall]:
    prompt_lower = prompt.lower()
    tool_calls = []

    if re.search(r"how many (products|items)|total products|number of products", prompt_lower):
        tool_calls.append(ToolCall(tool="get_product_count"))
    elif re.search(r"top products|best selling|top items|best sellers", prompt_lower):
        tool_calls.append(ToolCall(tool="get_top_products", params={"range": "30d", "limit": 5}))
    elif re.search(r"low stock|out of stock", prompt_lower):
        tool_calls.append(ToolCall(tool="get_low_stock", params={"threshold": 5}))
    elif re.search(r"sales|revenue|how much did we make", prompt_lower) and not re.search(r"breakdown|category", prompt_lower):
        tool_calls.append(ToolCall(tool="get_sales", params={"range": "30d"}))

    if tool_calls:
        logger.info(f"Intent Layer -> Matched fast heuristics: {[c.tool for c in tool_calls]}")
        return tool_calls

    logger.info("Intent Layer -> NLP too complex, falling back to LLM routing pass.")
    return await plan_tools(prompt, history, logger)
'''

FILES["app/agent/formatter.py"] = '''from typing import List
from app.agent.schemas import ToolResult
import json

def format_result(tool_name: str, data: dict) -> str:
    if not data:
        return "No data returned."
    
    if tool_name == "get_product_count":
        total = data.get("total_products", 0)
        res = f"You currently have {total} total products in your store."
        breakdown = data.get("category_breakdown", {})
        if breakdown:
            res += "\\nCategory breakdown:"
            for cat, count in breakdown.items():
                res += f"\\n- {cat}: {count}"
        return res

    elif tool_name == "get_top_products":
        products = data.get("products", [])
        if not products:
            return "No top selling products found."
        res = f"Top {data.get('limit', len(products))} products:"
        for p in products:
            res += f"\\n- {p.get('name', 'Unknown')} ({p.get('category', 'Generic')}): {p.get('units', 0)} units sold"
        return res

    elif tool_name == "get_low_stock":
        items = data.get("items", [])
        res = f"Found {data.get('count', len(items))} items with low stock:"
        for i in items:
            name = i.get("name") or i.get("title") or "Unknown"
            stock = i.get("current_stock") or i.get("stock") or i.get("inventory") or i.get("quantity") or 0
            res += f"\\n- {name}: {stock} units remaining"
        return res

    elif tool_name == "get_sales":
        res = f"Sales data overview ({data.get('range', '30d')}):"
        res += f"\\n- Total Revenue: ${data.get('revenue', 0)}"
        res += f"\\n- Total Orders: {data.get('orders', 0)}"
        return res
    
    return json.dumps(data, separators=(",", ":"))

def format_all_results(tool_results: List[ToolResult]) -> List[ToolResult]:
    for r in tool_results:
        if r.success and r.data is not None:
            r.summary = format_result(r.tool, r.data)
        elif not r.success:
            r.summary = f"Error occurred: {r.error}"
        else:
            r.summary = "Execution completed with no data."
    return tool_results
'''

FILES["app/agent/synthesizer.py"] = '''from typing import List, Dict
from app.llm.client import call_ollama
from app.agent.schemas import ToolResult
from app.core.logging import CorrelationAdapter

SYNTHESIS_PROMPT = """You are a knowledgeable e-commerce business analyst assistant.
Write a clear, concise, professional answer to the admin's question using ONLY the provided data limits.
Do NOT make up numbers or inject external logic. Address the admin directly.
"""

def _build_injection_view(results: List[ToolResult]) -> str:
    blocks = []
    for r in results:
        indicator = "✅ SUCCESS" if r.success else "❌ FAILED"
        blocks.append(f"### Output From {r.tool} - {indicator}\\n{r.summary}")
    return "\\n\\n".join(blocks) if blocks else "No tools executed."

async def synthesize_answer(
    prompt: str, 
    history: List[Dict[str, str]], 
    tool_results: List[ToolResult], 
    logger: CorrelationAdapter
) -> str:
    injected_data = _build_injection_view(tool_results)
    
    messages = [{"role": "system", "content": SYNTHESIS_PROMPT}] + history + [
        {"role": "user", "content": prompt},
        {
            "role": "assistant",
            "content": f"Here is the database information I queried via internal tools:\\n\\n{injected_data}\\n\\nI will answer right now."
        },
        {"role": "user", "content": "Please synthesize and format a final response to my question."}
    ]

    try:
        answer = await call_ollama(messages, logger, json_mode=False)
        return answer.strip()
    except Exception as exc:
        logger.error(f"Synthesis Pass Failed -> Network Error: {exc}")
        return "Here is the raw processed data:\\n\\n" + injected_data
'''

FILES["app/agent/orchestrator.py"] = '''from typing import Optional, Tuple, List
from app.agent.schemas import ToolResult
from app.agent.intent import map_intent
from app.agent.executor import execute_concurrent
from app.agent.formatter import format_all_results
from app.agent.synthesizer import synthesize_answer
from app.core.session import get_history, push_history
from app.core.logging import CorrelationAdapter
from app.utils.timing import async_log_timing

@async_log_timing(logger=None, operation_name="Full Hybrid Workflow")
async def run_agent_workflow(
    prompt: str,
    session_id: str,
    auth_header: Optional[str],
    cookie_header: Optional[str],
    user_id: Optional[str],
    user_role: Optional[str],
    logger: CorrelationAdapter
) -> Tuple[str, List[ToolResult]]:
    history = get_history(session_id)

    logger.info("--- PASS 1: INTENT & ROUTING ---")
    tool_calls = await map_intent(prompt, history, logger)

    logger.info("--- PASS 2: SECURE CONCURRENT EXECUTION ---")
    raw_results = await execute_concurrent(
        tool_calls, auth_header, cookie_header, user_id, user_role, logger
    )

    logger.info("--- PASS 3: RESULT FORMATTER ---")
    formatted_results = format_all_results(raw_results)

    logger.info("--- PASS 4: SYNTHESIS PASS ---")
    answer = await synthesize_answer(prompt, history, formatted_results, logger)

    push_history(session_id, "user", prompt)
    push_history(session_id, "assistant", answer)

    logger.info("--- PIPELINE COMPLETE ---")
    return answer, formatted_results
'''

for path, content in FILES.items():
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

print("V4 Hybrid Layer implemented!")