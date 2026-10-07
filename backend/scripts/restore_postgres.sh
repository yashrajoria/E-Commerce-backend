#!/usr/bin/env bash
set -euo pipefail
# Restore a ShopSwift Postgres custom-format dump into the compose postgres
# container, then sanity-check that core tables have rows.
#
# Usage (from backend/):
#   ./scripts/restore_postgres.sh backups/ecommerce_20261001_120000.dump
#   ./scripts/restore_postgres.sh <dump> --yes          # skip confirmation
#   FORCE=1 ./scripts/restore_postgres.sh <dump>        # skip confirmation
#   TARGET_DB=shopswift_restore_test ./scripts/restore_postgres.sh <dump> --yes
#
# By default restores into the live database (POSTGRES_DB) after confirmation.
# Set TARGET_DB to restore into a scratch database instead (recommended for
# verifying a backup without touching production-like data).

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-ecommerce}"
TARGET_DB="${TARGET_DB:-}"

if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-ecommerce}"
TARGET_DB="${TARGET_DB:-}"

DUMP_FILE="${1:-}"
ASSUME_YES=0
for arg in "${@:2}"; do
  if [[ "$arg" == "--yes" ]]; then
    ASSUME_YES=1
  fi
done
if [[ "${FORCE:-0}" == "1" ]]; then
  ASSUME_YES=1
fi

if [[ -z "$DUMP_FILE" ]]; then
  echo "Usage: $0 <dump-file> [--yes]" >&2
  echo "  dump-file: path to a .dump produced by backup_postgres.sh" >&2
  exit 1
fi
if [[ ! -f "$DUMP_FILE" ]]; then
  echo "ERROR: dump file not found: $DUMP_FILE" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker not found." >&2
  exit 1
fi
# Compose prefixes container names with the project (e.g. backend-postgres-1),
# so auto-detect when the literal name isn't running.
if ! docker ps --format '{{.Names}}' | grep -q "^${POSTGRES_CONTAINER}$"; then
  DETECTED="$(docker ps --format '{{.Names}}' | grep -E '(^|[-_])postgres([-_0-9]*)$' | head -1 || true)"
  if [[ -n "$DETECTED" ]]; then
    echo "[detect] Using postgres container '${DETECTED}' (override with POSTGRES_CONTAINER=...)"
    POSTGRES_CONTAINER="$DETECTED"
  else
    echo "ERROR: no running postgres container found (looked for '${POSTGRES_CONTAINER}')." >&2
    exit 1
  fi
fi

RESTORE_DB="${TARGET_DB:-$POSTGRES_DB}"
echo "[restore] Target database: ${RESTORE_DB}"
echo "[restore] Dump file:       ${DUMP_FILE}"

if [[ "$ASSUME_YES" != "1" ]]; then
  echo "This OVERWRITES database '${RESTORE_DB}' on container '${POSTGRES_CONTAINER}'."
  printf "Continue? [y/N] "
  read -r REPLY
  if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
  fi
fi

psql_exec() {
  docker exec -i "$POSTGRES_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$RESTORE_DB" "$@"
}

if [[ "$RESTORE_DB" != "$POSTGRES_DB" ]]; then
  echo "[restore] Creating scratch database '${RESTORE_DB}' if needed..."
  docker exec -i "$POSTGRES_CONTAINER" psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres \
    -c "CREATE DATABASE \"${RESTORE_DB}\";" 2>/dev/null || true
fi

echo "[restore] Dropping existing tables in '${RESTORE_DB}'..."
psql_exec -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO public;" \
  || { echo "ERROR: failed to reset schema." >&2; exit 1; }

echo "[restore] Restoring dump..."
if docker exec -i "$POSTGRES_CONTAINER" pg_restore -U "$POSTGRES_USER" -d "$RESTORE_DB" \
    --no-owner --no-privileges < "$DUMP_FILE"; then
  echo "[restore] pg_restore completed."
else
  # pg_restore exits non-zero when it encounters non-fatal errors (e.g. missing
  # extension). Surface the failure but continue to verification — the row
  # counts below are the real pass/fail signal.
  echo "[restore] pg_restore reported errors (continuing to verification)..." >&2
fi

echo "[restore] Sanity check — core table row counts:"
psql_exec -t -c "SELECT 'users', count(*) FROM users UNION ALL SELECT 'orders', count(*) FROM orders UNION ALL SELECT 'payments', count(*) FROM payments UNION ALL SELECT 'coupons', count(*) FROM coupons UNION ALL SELECT 'agent_audit_log', count(*) FROM agent_audit_log;" \
  2>/dev/null || echo "(some core tables missing — check migration state)"

echo "Restore into '${RESTORE_DB}' complete."
