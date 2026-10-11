from collections import OrderedDict, deque
from typing import Deque, Dict, List

from app.core.config import settings

# /shopper/query is public and session ids are client-chosen, so cap the number of
# sessions kept (least recently used evicted) or a client can grow memory without bound.
MAX_SESSIONS = 5000

_sessions: "OrderedDict[str, Deque[Dict[str, str]]]" = OrderedDict()


def _max_items() -> int:
    return settings.MAX_HISTORY_TURNS * 2


def get_history(session_id: str) -> List[Dict[str, str]]:
    return list(_sessions.get(session_id, deque()))


def push_history(session_id: str, role: str, content: str) -> None:
    history = _sessions.get(session_id)
    if history is None:
        history = _sessions[session_id] = deque()
        while len(_sessions) > MAX_SESSIONS:
            _sessions.popitem(last=False)
    else:
        _sessions.move_to_end(session_id)
    history.append({"role": role, "content": content})
    while len(history) > _max_items():
        history.popleft()


def clear_session(session_id: str) -> bool:
    return _sessions.pop(session_id, None) is not None
