#!/usr/bin/env bash
set -euo pipefail
# Pre-flight health check + k6 runner for the ShopSwift load-test toolkit.
#
# Usage (from backend/tools/loadtest/):
#   ./run-loadtest.sh
#
# Env:
#   GATEWAY_URL          default http://localhost:8080
#   LOADTEST_EMAIL       optional — enables the login scenario
#   LOADTEST_PASSWORD    optional

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "[loadtest] Gateway: ${GATEWAY_URL}"
echo "[loadtest] Checking gateway health..."

if ! command -v curl >/dev/null 2>&1; then
  echo "ERROR: curl not found" >&2
  exit 1
fi

HEALTH_CODE="$(curl -s -o /dev/null -w '%{http_code}' "${GATEWAY_URL}/health" || echo "000")"
if [[ "$HEALTH_CODE" != "200" ]]; then
  echo "ERROR: gateway health check failed (HTTP ${HEALTH_CODE})." >&2
  echo "       Start the stack first: cd backend && ./scripts/dev-up.sh" >&2
  exit 1
fi
echo "[loadtest] Gateway healthy (HTTP 200)."

if ! command -v k6 >/dev/null 2>&1; then
  echo "ERROR: k6 not found. Install with:" >&2
  echo "  brew install k6" >&2
  echo "  (or https://k6.io/docs/get-started/installation/)" >&2
  exit 1
fi

if [[ -z "${LOADTEST_EMAIL:-}" || -z "${LOADTEST_PASSWORD:-}" ]]; then
  echo "[loadtest] LOADTEST_EMAIL/LOADTEST_PASSWORD not set — login scenario will be skipped."
  echo "[loadtest] For seeded demo credentials, see backend/tools/loadtest/README.md"
fi

echo "[loadtest] Running k6..."
cd "$SCRIPT_DIR"
k6 run --env GATEWAY_URL="${GATEWAY_URL}" k6-loadtest.js
