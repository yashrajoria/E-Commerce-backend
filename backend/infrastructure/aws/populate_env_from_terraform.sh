#!/usr/bin/env bash
set -euo pipefail

# Populate .env with values from infrastructure/aws/terraform-outputs.json
# Usage: ./populate_env_from_terraform.sh

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_JSON="$ROOT_DIR/terraform/terraform-outputs.json"
ENV_FILE="$ROOT_DIR/../../.env"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required. Install jq and retry."
  exit 1
fi

if [ ! -f "$OUT_JSON" ]; then
  echo "Terraform outputs file not found: $OUT_JSON"
  echo "Run 'infrastructure/aws/deploy_infrastructure.sh apply' first to generate outputs."
  exit 1
fi

echo "Reading Terraform outputs from $OUT_JSON"

get_val(){ jq -r "\.${1}.value // empty" "$OUT_JSON"; }

s3_bucket=$(get_val s3_bucket)
products_table=$(get_val dynamodb_products_table)
order_queue_url=$(get_val sqs_order_processing_url)
cloudwatch_group=$(get_val cloudwatch_log_group)
region=$(get_val region)

echo "Will write the following (if present):"
echo "  AWS_S3_BUCKET: $s3_bucket"
echo "  DYNAMODB_PRODUCTS_TABLE: $products_table"
echo "  ORDER_SQS_URL: $order_queue_url"
echo "  CLOUDWATCH_LOG_GROUP: $cloudwatch_group"
echo "  AWS_REGION: $region"

cp "$ENV_FILE" "${ENV_FILE}.backup.$(date +%s)"

set_or_append(){
  key="$1"
  val="$2"
  if [ -z "$val" ]; then
    return
  fi
  if grep -q "^${key}=\"\?" "$ENV_FILE"; then
    sed -i.bak "s#^${key}=.*#${key}=${val}#" "$ENV_FILE"
  else
    echo "${key}=${val}" >> "$ENV_FILE"
  fi
}

set_or_append AWS_S3_BUCKET "$s3_bucket"
set_or_append S3_BUCKET_IMAGES "$s3_bucket"
set_or_append DYNAMODB_PRODUCTS_TABLE "$products_table"
set_or_append AWS_REGION "$region"
set_or_append CLOUDWATCH_LOG_GROUP "$cloudwatch_group"
set_or_append ORDER_SQS_URL "$order_queue_url"

echo "Updated $ENV_FILE (backup at ${ENV_FILE}.backup.*)."

echo "Note: some outputs (e.g., payment queue URLs or SNS ARNs) may not be exported by Terraform.
If missing, either update Terraform to output them or set the corresponding env vars manually in $ENV_FILE."

echo "Done."
