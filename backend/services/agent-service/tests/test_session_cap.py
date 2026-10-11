from app.core import session


def test_sessions_are_capped_and_lru(monkeypatch):
    monkeypatch.setattr(session, "MAX_SESSIONS", 3)
    session._sessions.clear()
    for i in range(3):
        session.push_history(f"s{i}", "user", "hi")
    session.push_history("s0", "user", "again")  # touch s0 -> s1 is now oldest
    session.push_history("s3", "user", "new")
    assert session.get_history("s1") == []
    assert session.get_history("s0") != [] and session.get_history("s3") != []
    assert len(session._sessions) == 3
