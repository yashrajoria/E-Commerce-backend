#!/usr/bin/env bash
set -euo pipefail

# seed_data.sh
# Seeds DynamoDB tables and creates SNS/SQS subscriptions if needed.
# Requires AWS CLI configured (profile or env vars).

if [ -z "${AWS_REGION:-}" ]; then
  echo "Please set AWS_REGION env var (e.g. us-east-1)."
  exit 1
fi

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI not found in PATH; please install AWS CLI v2"
  exit 1
fi

set -x

# Read terraform outputs file if present
BASE_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_JSON="${BASE_DIR}/terraform-outputs.json"

if [ -f "${OUT_JSON}" ]; then
  S3_BUCKET=$(jq -r '.s3_bucket.value' "${OUT_JSON}" 2>/dev/null || true)
  PRODUCTS_TABLE=$(jq -r '.dynamodb_products_table.value' "${OUT_JSON}" 2>/dev/null || true)
else
  echo "No terraform-outputs.json found at ${OUT_JSON}; please run deploy_infrastructure.sh apply first or set resource names manually."
fi

PRODUCTS_TABLE=${PRODUCTS_TABLE:-Products}

echo "Seeding Categories into DynamoDB table ${PRODUCTS_TABLE}"
aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-electronics"}, "name": {"S": "Electronics"}}' || true
aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-fashion"}, "name": {"S": "Fashion"}}' || true
aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-home"}, "name": {"S": "Home"}}' || true
aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-books"}, "name": {"S": "Books"}}' || true
aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-sports"}, "name": {"S": "Sports"}}' || true

echo "Seed complete."
