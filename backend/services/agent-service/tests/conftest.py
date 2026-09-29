"""
conftest.py — ensures the agent-service source root (/app on container, the
service directory on local) is on sys.path so `from app.xxx import yyy` works
regardless of how pytest is invoked.
"""
import sys
import os

# Insert the project root (parent of the `app` package) at position 0.
# On the container this is /app; locally it is wherever this conftest.py lives.
_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _root not in sys.path:
    sys.path.insert(0, _root)
