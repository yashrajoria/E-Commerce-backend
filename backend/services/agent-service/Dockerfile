FROM python:3.11-slim AS builder

WORKDIR /app

# Install compiler tooling only while building dependencies.
RUN apt-get update && apt-get install -y \
    gcc \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .

# Install dependencies into a self-contained prefix for the runtime image.
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

COPY . .

FROM python:3.11-slim AS runtime

WORKDIR /app

COPY --from=builder /install /usr/local
COPY . .

RUN groupadd --system app && \
    useradd --system --uid 1001 --gid app --home-dir /app --shell /usr/sbin/nologin app && \
    chown -R app:app /app
USER app

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python -c "import httpx; httpx.get('http://localhost:8000/health', timeout=5)" || exit 1

# Run the application
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
