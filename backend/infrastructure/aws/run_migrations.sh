#!/usr/bin/env bash
set -euo pipefail

# Run golang-migrate against a remote/local Postgres (e.g. RDS).
# Example:
#   DB_HOST=... DB_USER=... DB_PASS=... DB_NAME=ecommerce ./run_migrations.sh up

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BACKEND_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

DB_HOST="${DB_HOST:-}"
DB_USER="${DB_USER:-postgres}"
DB_PASS="${DB_PASS:-}"
DB_NAME="${DB_NAME:-ecommerce}"
DB_PORT="${DB_PORT:-5432}"
DB_SSLMODE="${DB_SSLMODE:-require}"
ACTION="${1:-up}"

if [ -z "$DB_HOST" ]; then
  echo "DB_HOST not set. Example: DB_HOST=mydb.rds.amazonaws.com DB_USER=... DB_PASS=... $0 up"
  exit 1
fi

export POSTGRES_HOST="$DB_HOST"
export POSTGRES_USER="$DB_USER"
export POSTGRES_PASSWORD="$DB_PASS"
export POSTGRES_DB="$DB_NAME"
export POSTGRES_PORT="$DB_PORT"
export POSTGRES_SSLMODE="$DB_SSLMODE"

exec "$BACKEND_ROOT/scripts/migrate.sh" "$ACTION"
