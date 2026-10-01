#!/usr/bin/env bash
set -euo pipefail
# Back up the ShopSwift Postgres database to a timestamped pg_dump custom-format
# file under backend/backups/.
#
# Usage (from backend/):
#   ./scripts/backup_postgres.sh
#   DRY_RUN=1 ./scripts/backup_postgres.sh    # print the command, don't execute
#
# Reads POSTGRES_* from backend/.env when present (same convention as migrate.sh).
# Uses docker exec against the compose postgres container so the host does not
# need a local psql client.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-ecommerce}"
BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"

if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-ecommerce}"

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker not found. Install Docker or run pg_dump directly." >&2
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
    echo "       Start the stack first: ./scripts/dev-up.sh" >&2
    exit 1
  fi
fi

mkdir -p "$BACKUP_DIR"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
DUMP_FILE="${BACKUP_DIR}/${POSTGRES_DB}_${TIMESTAMP}.dump"

DUMP_CMD=(docker exec -i "$POSTGRES_CONTAINER" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc)

if [[ "${DRY_RUN:-0}" == "1" ]]; then
  echo "[dry-run] Would run: ${DUMP_CMD[*]}"
  echo "[dry-run] Would write: $DUMP_FILE"
  exit 0
fi

echo "[backup] Dumping ${POSTGRES_DB} from container ${POSTGRES_CONTAINER}..."
if "${DUMP_CMD[@]}" > "$DUMP_FILE"; then
  SIZE="$(du -h "$DUMP_FILE" | awk '{print $1}')"
  echo "[backup] OK → $DUMP_FILE ($SIZE)"
else
  rm -f "$DUMP_FILE"
  echo "ERROR: pg_dump failed." >&2
  exit 1
fi
