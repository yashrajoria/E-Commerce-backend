#!/usr/bin/env bash
set -euo pipefail

# Idempotent LocalStack resource provisioning script.
AWS_CLI=${AWS_CLI:-aws}
AWSL_LOCAL_CMD="awslocal"
ENDPOINT=${AWS_ENDPOINT:-http://localhost:4566}
REGION=${AWS_REGION:-us-east-1}

use_awslocal() {
  command -v ${AWSL_LOCAL_CMD} >/dev/null 2>&1
}

_aws() {
  if use_awslocal; then
    ${AWSL_LOCAL_CMD} "$@"
  else
    ${AWS_CLI} --endpoint-url=${ENDPOINT} --region ${REGION} "$@"
  fi
}

echo "Using endpoint ${ENDPOINT} region ${REGION}"

# S3 bucket
BUCKET=${S3_BUCKET_IMAGES:-ecommerce-product-images}
echo "Creating bucket ${BUCKET} if not exists"
_aws s3 mb s3://${BUCKET} || true

# SNS topic
TOPIC_NAME=order-events
echo "Waiting for SNS to be available (up to ${WAIT_RETRIES:-30} attempts)..."
for i in $(seq 1 ${WAIT_RETRIES:-30}); do
  if _aws sns list-topics >/dev/null 2>&1; then
    echo "SNS is available"
    break
  fi
  echo "SNS not ready yet (attempt ${i}/${WAIT_RETRIES:-30}), sleeping ${WAIT_SLEEP:-2}s..."
  sleep ${WAIT_SLEEP:-2}
  if [ "${i}" -eq "${WAIT_RETRIES:-30}" ]; then
    echo "SNS not available after ${WAIT_RETRIES:-30} attempts — proceeding (create may fail)"
  fi
done

echo "Creating SNS topic ${TOPIC_NAME}"
TOPIC_ARN=$(_aws sns create-topic --name ${TOPIC_NAME} --output text --query 'TopicArn' 2>/dev/null || true)
if [ -z "${TOPIC_ARN}" ]; then
  TOPIC_ARN=$(_aws sns create-topic --name ${TOPIC_NAME} --output text --query 'TopicArn')
fi
echo "SNS topic ARN: ${TOPIC_ARN}"

echo "Creating SNS topic payment-events"
PAYMENT_TOPIC_ARN=$(_aws sns create-topic --name payment-events --output text --query 'TopicArn' 2>/dev/null || true)
if [ -z "${PAYMENT_TOPIC_ARN}" ]; then
  PAYMENT_TOPIC_ARN=$(_aws sns create-topic --name payment-events --output text --query 'TopicArn')
fi
echo "Payment topic ARN: ${PAYMENT_TOPIC_ARN}"

# SQS queues
echo "Waiting for SQS to be available (up to ${WAIT_RETRIES:-30} attempts)..."
for i in $(seq 1 ${WAIT_RETRIES:-30}); do
  if _aws sqs list-queues >/dev/null 2>&1; then
    echo "SQS is available"
    break
  fi
  echo "SQS not ready yet (attempt ${i}/${WAIT_RETRIES:-30}), sleeping ${WAIT_SLEEP:-2}s..."
  sleep ${WAIT_SLEEP:-2}
  if [ "${i}" -eq "${WAIT_RETRIES:-30}" ]; then
    echo "SQS not available after ${WAIT_RETRIES:-30} attempts — proceeding (create may fail)"
  fi
done

echo "Creating SQS queues"
_aws sqs create-queue --queue-name order-processing-queue || true
_aws sqs create-queue --queue-name order-processing-dlq || true
_aws sqs create-queue --queue-name email-notify-queue || true
_aws sqs create-queue --queue-name payment-events-queue || true
_aws sqs create-queue --queue-name payment-request-queue || true

# Subscribe SQS to SNS
echo "Subscribing queues to SNS"
QUEUE_URL=$(_aws sqs get-queue-url --queue-name order-processing-queue --output text --query 'QueueUrl')
QUEUE_ARN=$(_aws sqs get-queue-attributes --queue-url ${QUEUE_URL} --attribute-names QueueArn --output text --query 'Attributes.QueueArn')
_aws sns subscribe --topic-arn ${TOPIC_ARN} --protocol sqs --notification-endpoint ${QUEUE_ARN} || true

PAY_QUEUE_URL=$(_aws sqs get-queue-url --queue-name payment-events-queue --output text --query 'QueueUrl')
PAY_QUEUE_ARN=$(_aws sqs get-queue-attributes --queue-url ${PAY_QUEUE_URL} --attribute-names QueueArn --output text --query 'Attributes.QueueArn')
_aws sns subscribe --topic-arn ${PAYMENT_TOPIC_ARN} --protocol sqs --notification-endpoint ${PAY_QUEUE_ARN} || true

# DynamoDB tables
echo "Creating DynamoDB tables"
# Wait for DynamoDB to become available (LocalStack may enable services asynchronously)
WAIT_RETRIES=${INIT_WAIT_RETRIES:-30}
WAIT_SLEEP=${INIT_WAIT_SLEEP:-2}
echo "Waiting for DynamoDB to be available (up to ${WAIT_RETRIES} attempts)..."
for i in $(seq 1 ${WAIT_RETRIES}); do
  if _aws dynamodb list-tables >/dev/null 2>&1; then
    echo "DynamoDB is available"
    break
  fi
  echo "DynamoDB not ready yet (attempt ${i}/${WAIT_RETRIES}), sleeping ${WAIT_SLEEP}s..."
  sleep ${WAIT_SLEEP}
  if [ "${i}" -eq "${WAIT_RETRIES}" ]; then
    echo "DynamoDB not available after ${WAIT_RETRIES} attempts — proceeding (create may fail)"
  fi
done

_aws dynamodb create-table --table-name Products --attribute-definitions AttributeName=product_id,AttributeType=S --key-schema AttributeName=product_id,KeyType=HASH --billing-mode PAY_PER_REQUEST || true
_aws dynamodb create-table --table-name Inventory --attribute-definitions AttributeName=product_id,AttributeType=S --key-schema AttributeName=product_id,KeyType=HASH --billing-mode PAY_PER_REQUEST || true
_aws dynamodb create-table --table-name Categories --attribute-definitions AttributeName=category_id,AttributeType=S --key-schema AttributeName=category_id,KeyType=HASH --billing-mode PAY_PER_REQUEST || true
# Seed initial categories into DynamoDB
echo "Seeding initial categories into DynamoDB"
_aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-electronics"}, "name": {"S": "Electronics"}}' || true
_aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-fashion"}, "name": {"S": "Fashion"}}' || true
_aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-home"}, "name": {"S": "Home"}}' || true
_aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-books"}, "name": {"S": "Books"}}' || true
_aws dynamodb put-item --table-name Categories --item '{"category_id": {"S": "cat-sports"}, "name": {"S": "Sports"}}' || true

echo "LocalStack resources provisioned"

# Create placeholder Secrets Manager secrets for auth-service (idempotent)
echo "Creating Secrets Manager secrets (auth)"
_aws secretsmanager create-secret --name auth/JWT_SECRET --secret-string "supersecret-jwt" || true
_aws secretsmanager create-secret --name auth/DB_CREDENTIALS --secret-string '{"POSTGRES_USER":"auth_user","POSTGRES_PASSWORD":"auth_pass","POSTGRES_DB":"auth_db","POSTGRES_HOST":"postgres","POSTGRES_PORT":"5432"}' || true

echo "Secrets created (or already exist)"

# Create service-specific secrets
echo "Creating service secrets: product, order, user, inventory"
_aws secretsmanager create-secret --name product/JWT_SECRET --secret-string "product-jwt-secret" || true
_aws secretsmanager create-secret --name product/DB_CREDENTIALS --secret-string '{"DDB_TABLE_PRODUCTS":"Products","DDB_TABLE_CATEGORIES":"Categories"}' || true

_aws secretsmanager create-secret --name order/JWT_SECRET --secret-string "order-jwt-secret" || true
_aws secretsmanager create-secret --name order/DB_CREDENTIALS --secret-string '{"POSTGRES_USER":"order_user","POSTGRES_PASSWORD":"order_pass","POSTGRES_DB":"order_db","POSTGRES_HOST":"postgres","POSTGRES_PORT":"5432"}' || true

_aws secretsmanager create-secret --name user/JWT_SECRET --secret-string "user-jwt-secret" || true
_aws secretsmanager create-secret --name user/DB_CREDENTIALS --secret-string '{"POSTGRES_USER":"user_user","POSTGRES_PASSWORD":"user_pass","POSTGRES_DB":"user_db","POSTGRES_HOST":"postgres","POSTGRES_PORT":"5432"}' || true

_aws secretsmanager create-secret --name inventory/JWT_SECRET --secret-string "inventory-jwt-secret" || true
_aws secretsmanager create-secret --name inventory/DB_CREDENTIALS --secret-string '{"DDB_TABLE":"Inventory"}' || true

echo "Service secrets created"
