#!/usr/bin/env bash
set -euo pipefail

# Start the full local stack including LocalStack (required for SNS/SQS/DDB/S3).
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "Starting ShopSwift backend (compose + LocalStack)..."
docker compose -f docker-compose.yml -f docker-compose.localstack.yml up -d --build "$@"

echo ""
echo "Gateway:      http://localhost:8080"
echo "BFF:          http://localhost:8088"
echo "OpenAPI UI:   http://localhost:8099"
echo "LocalStack:   http://localhost:4566"
echo ""
echo "Tip: ./scripts/migrate.sh up  (against localhost:5432 when Postgres is up)"
