from collections import defaultdict, deque
from typing import Deque, Dict, List

from app.core.config import settings


_sessions: Dict[str, Deque[Dict[str, str]]] = defaultdict(deque)


def _max_items() -> int:
    return settings.MAX_HISTORY_TURNS * 2


def get_history(session_id: str) -> List[Dict[str, str]]:
    return list(_sessions.get(session_id, deque()))


def push_history(session_id: str, role: str, content: str) -> None:
    history = _sessions[session_id]
    history.append({"role": role, "content": content})
    while len(history) > _max_items():
        history.popleft()


def clear_session(session_id: str) -> bool:
    return _sessions.pop(session_id, None) is not None
