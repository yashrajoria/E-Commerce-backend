import time
from functools import wraps
from typing import Callable, Any

def async_log_timing(logger, operation_name: str = None):
    """Decorator to measure and log the execution time of async functions."""
    def decorator(func: Callable) -> Callable:
        @wraps(func)
        async def wrapper(*args, **kwargs) -> Any:
            start_time = time.perf_counter()
            try:
                return await func(*args, **kwargs)
            finally:
                duration = time.perf_counter() - start_time
                name = operation_name or func.__name__
                if logger:
                    logger.info(f"{name} completed in {duration:.3f}s")
                else:
                    print(f"{name} completed in {duration:.3f}s")
        return wrapper
    return decorator
