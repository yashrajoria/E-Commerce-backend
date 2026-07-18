#!/usr/bin/env bash
set -euo pipefail

# Apply SQL migrations with golang-migrate.
# Usage (from backend/):
#   ./scripts/migrate.sh up
#   ./scripts/migrate.sh down 1
#
# Requires: migrate CLI (https://github.com/golang-migrate/migrate)
#   go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ACTION="${1:-up}"
shift || true

POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-postgres}"
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_DB="${POSTGRES_DB:-ecommerce}"
POSTGRES_SSLMODE="${POSTGRES_SSLMODE:-disable}"

if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi

DATABASE_URL="${DATABASE_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}?sslmode=${POSTGRES_SSLMODE}}"

if ! command -v migrate >/dev/null 2>&1; then
  echo "migrate CLI not found. Install with:"
  echo "  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"
  exit 1
fi

echo "Running migrate $ACTION against ${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}"
migrate -path "$ROOT/migrations" -database "$DATABASE_URL" "$ACTION" "$@"
