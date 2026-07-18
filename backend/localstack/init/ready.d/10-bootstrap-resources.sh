#!/bin/bash
set -euo pipefail

set -x

echo "Initializing LocalStack resources..."

AWS_ENDPOINT="${LOCALSTACK_ENDPOINT:-http://localhost:4566}"
awslocal_cmd() {
    awslocal --endpoint-url "$AWS_ENDPOINT" "$@"
}

# --------------------------------------------------
# Config (names driven by env; safe defaults)
# --------------------------------------------------
ORDER_TOPIC_NAME="${ORDER_SNS_TOPIC_NAME:-order-events}"
PAYMENT_TOPIC_NAME="${PAYMENT_SNS_TOPIC_NAME:-payment-events}"
AUTH_TOPIC_NAME="${AUTH_SNS_TOPIC_NAME:-auth-events}"
PROMOTION_TOPIC_NAME="${PROMOTION_SNS_TOPIC_NAME:-promotion-events}"

ORDER_QUEUE_NAME="${ORDER_PROCESSING_QUEUE_NAME:-order-processing-queue}"
PAYMENT_EVENTS_QUEUE_NAME="${PAYMENT_EVENTS_QUEUE_NAME:-payment-events-queue}"
PAYMENT_REQUEST_QUEUE_NAME="${PAYMENT_REQUEST_QUEUE_NAME:-payment-request-queue}"
NOTIFICATION_QUEUE_NAME="${NOTIFICATION_SQS_QUEUE_NAME:-notification-queue}"
PROMOTION_ORDER_QUEUE_NAME="${PROMOTION_ORDER_QUEUE_NAME:-promotion-order-queue}"

# --------------------------------------------------
# Retry helper (ONLY for transient failures)
# --------------------------------------------------
retry() {
    local n=0
    local max=6
    local delay=2

    while true; do
        if "$@"; then
            return 0
        fi

        n=$((n+1))
        if [ "$n" -ge "$max" ]; then
            echo "Command failed after $n attempts: $*" >&2
            return 1
        fi

        sleep "$delay"
        delay=$((delay * 2))
    done
}

# --------------------------------------------------
# S3 (idempotent)
# --------------------------------------------------
BUCKET_NAME="${AWS_S3_BUCKET:-shopswift}"

if ! awslocal_cmd s3api head-bucket --bucket "$BUCKET_NAME" 2>/dev/null; then
    retry awslocal_cmd s3 mb "s3://$BUCKET_NAME"
else
    echo "S3 bucket '$BUCKET_NAME' already exists"
fi


# --------------------------------------------------
# DynamoDB (idempotent)
# NOTE: Existing LocalStack volumes with old table schemas will NOT pick up
# new GSIs. Recreate the LocalStack volume (or delete/recreate these tables)
# once after pulling GSI changes.
# --------------------------------------------------
create_products_table_if_missing() {
    local tbl_name="${DDB_TABLE_PRODUCTS:-Products}"

    if awslocal_cmd dynamodb describe-table --table-name "$tbl_name" >/dev/null 2>&1; then
        echo "DynamoDB table '$tbl_name' already exists"
        return 0
    fi

    echo "Creating DynamoDB table '$tbl_name' with sku-index and featured-index..."

    retry awslocal_cmd dynamodb create-table \
        --table-name "$tbl_name" \
        --attribute-definitions \
            AttributeName=id,AttributeType=S \
            AttributeName=sku,AttributeType=S \
            AttributeName=is_featured,AttributeType=S \
            AttributeName=created_at,AttributeType=S \
        --key-schema AttributeName=id,KeyType=HASH \
        --global-secondary-indexes \
            '[
              {
                "IndexName": "sku-index",
                "KeySchema": [{"AttributeName":"sku","KeyType":"HASH"}],
                "Projection": {"ProjectionType":"ALL"}
              },
              {
                "IndexName": "featured-index",
                "KeySchema": [
                  {"AttributeName":"is_featured","KeyType":"HASH"},
                  {"AttributeName":"created_at","KeyType":"RANGE"}
                ],
                "Projection": {"ProjectionType":"ALL"}
              }
            ]' \
        --billing-mode PAY_PER_REQUEST

    retry awslocal_cmd dynamodb wait table-exists --table-name "$tbl_name"
}

create_categories_table_if_missing() {
    local tbl_name="${DDB_TABLE_CATEGORIES:-Categories}"

    if awslocal_cmd dynamodb describe-table --table-name "$tbl_name" >/dev/null 2>&1; then
        echo "DynamoDB table '$tbl_name' already exists"
        return 0
    fi

    echo "Creating DynamoDB table '$tbl_name' with name-index..."

    retry awslocal_cmd dynamodb create-table \
        --table-name "$tbl_name" \
        --attribute-definitions \
            AttributeName=id,AttributeType=S \
            AttributeName=name,AttributeType=S \
        --key-schema AttributeName=id,KeyType=HASH \
        --global-secondary-indexes \
            '[
              {
                "IndexName": "name-index",
                "KeySchema": [{"AttributeName":"name","KeyType":"HASH"}],
                "Projection": {"ProjectionType":"ALL"}
              }
            ]' \
        --billing-mode PAY_PER_REQUEST

    retry awslocal_cmd dynamodb wait table-exists --table-name "$tbl_name"
}

create_inventory_table_if_missing() {
    local tbl_name="${DDB_TABLE_INVENTORY:-Inventory}"

    if awslocal_cmd dynamodb describe-table --table-name "$tbl_name" >/dev/null 2>&1; then
        echo "DynamoDB table '$tbl_name' already exists"
        return 0
    fi

    echo "Creating DynamoDB table '$tbl_name'..."

    retry awslocal_cmd dynamodb create-table \
        --table-name "$tbl_name" \
        --attribute-definitions AttributeName=id,AttributeType=S \
        --key-schema AttributeName=id,KeyType=HASH \
        --billing-mode PAY_PER_REQUEST

    retry awslocal_cmd dynamodb wait table-exists --table-name "$tbl_name"
}

create_product_categories_table_if_missing() {
    local tbl_name="${DDB_TABLE_PRODUCT_CATEGORIES:-ProductCategories}"

    if awslocal_cmd dynamodb describe-table --table-name "$tbl_name" >/dev/null 2>&1; then
        echo "DynamoDB table '$tbl_name' already exists"
        return 0
    fi

    echo "Creating DynamoDB table '$tbl_name' (category_id HASH, product_id RANGE, product-index)..."

    retry awslocal_cmd dynamodb create-table \
        --table-name "$tbl_name" \
        --attribute-definitions \
            AttributeName=category_id,AttributeType=S \
            AttributeName=product_id,AttributeType=S \
        --key-schema \
            AttributeName=category_id,KeyType=HASH \
            AttributeName=product_id,KeyType=RANGE \
        --global-secondary-indexes \
            '[
              {
                "IndexName": "product-index",
                "KeySchema": [
                  {"AttributeName":"product_id","KeyType":"HASH"},
                  {"AttributeName":"category_id","KeyType":"RANGE"}
                ],
                "Projection": {"ProjectionType":"ALL"}
              }
            ]' \
        --billing-mode PAY_PER_REQUEST

    retry awslocal_cmd dynamodb wait table-exists --table-name "$tbl_name"
}

create_products_table_if_missing
create_categories_table_if_missing
create_inventory_table_if_missing
create_product_categories_table_if_missing


# --------------------------------------------------
# SNS (idempotent)
# --------------------------------------------------
create_topic_if_missing() {
    local topic_name="$1"

    awslocal_cmd sns create-topic \
        --name "$topic_name" \
        --query "TopicArn" \
        --output text
}

ORDER_TOPIC_ARN=$(retry create_topic_if_missing "$ORDER_TOPIC_NAME")
PAYMENT_TOPIC_ARN=$(retry create_topic_if_missing "$PAYMENT_TOPIC_NAME")
AUTH_TOPIC_ARN=$(retry create_topic_if_missing "$AUTH_TOPIC_NAME")
PROMOTION_TOPIC_ARN=$(retry create_topic_if_missing "$PROMOTION_TOPIC_NAME")
NOTIFICATION_TOPIC_ARN=$(retry create_topic_if_missing "notification-events")


# --------------------------------------------------
# SQS (idempotent with DLQs)
# --------------------------------------------------
create_queue_with_dlq() {
    local queue_name="$1"
    local max_receive_count="${2:-3}"
    local dlq_name="${queue_name}-dlq"

    # --------------------------------------------------
    # 1️⃣ Create DLQ if missing
    # --------------------------------------------------
    if ! awslocal_cmd sqs get-queue-url --queue-name "$dlq_name" >/dev/null 2>&1; then
        echo "Creating DLQ: $dlq_name" >&2
        retry awslocal_cmd sqs create-queue --queue-name "$dlq_name" >/dev/null
    else
        echo "DLQ '$dlq_name' already exists" >&2
    fi

    # Get DLQ ARN
    local dlq_url
    dlq_url=$(awslocal_cmd sqs get-queue-url --queue-name "$dlq_name" --query "QueueUrl" --output text)
    local dlq_arn
    dlq_arn=$(awslocal_cmd sqs get-queue-attributes \
        --queue-url "$dlq_url" \
        --attribute-names QueueArn \
        --query "Attributes.QueueArn" \
        --output text)

    # --------------------------------------------------
    # 2️⃣ Create Main Queue if missing
    # --------------------------------------------------
    if ! awslocal_cmd sqs get-queue-url --queue-name "$queue_name" >/dev/null 2>&1; then
        echo "Creating Main Queue: $queue_name with DLQ: $dlq_name" >&2

        # Use temp file for RedrivePolicy JSON
        local tmpfile
        tmpfile=$(mktemp)
        cat > "$tmpfile" <<EOF
{
  "RedrivePolicy": "{\"deadLetterTargetArn\":\"$dlq_arn\",\"maxReceiveCount\":$max_receive_count}"
}
EOF

        retry awslocal_cmd sqs create-queue \
            --queue-name "$queue_name" \
            --attributes "file://$tmpfile" >/dev/null

        rm -f "$tmpfile"
    else
        echo "Main queue '$queue_name' already exists" >&2
    fi

    # Return Queue URL
    awslocal_cmd sqs get-queue-url --queue-name "$queue_name" --query "QueueUrl" --output text
}


ORDER_QUEUE_URL=$(retry create_queue_with_dlq "$ORDER_QUEUE_NAME")
PAYMENT_EVENTS_QUEUE_URL=$(retry create_queue_with_dlq "$PAYMENT_EVENTS_QUEUE_NAME")
PAYMENT_REQUEST_QUEUE_URL=$(retry create_queue_with_dlq "$PAYMENT_REQUEST_QUEUE_NAME")
NOTIFICATION_QUEUE_URL=$(retry create_queue_with_dlq "$NOTIFICATION_QUEUE_NAME")
PROMOTION_ORDER_QUEUE_URL=$(retry create_queue_with_dlq "$PROMOTION_ORDER_QUEUE_NAME")


# --------------------------------------------------
# SNS → SQS Subscription (idempotent)
# NOTE: In real AWS, SQS also needs a queue policy allowing the SNS topic to
# send messages. LocalStack can be permissive, but we set it anyway to avoid
# silent non-delivery and to keep parity with AWS.
# --------------------------------------------------
ensure_sqs_policy_allows_sns() {
    local topic_arn="$1"
    local queue_url="$2"

    local queue_arn
    queue_arn=$(awslocal_cmd sqs get-queue-attributes \
        --queue-url "$queue_url" \
        --attribute-names QueueArn \
        --query "Attributes.QueueArn" \
        --output text)

    local existing_policy
    existing_policy=$(awslocal_cmd sqs get-queue-attributes \
        --queue-url "$queue_url" \
        --attribute-names Policy \
        --query "Attributes.Policy" \
        --output text 2>/dev/null || true)

    # If the policy already references this topic ARN, assume it's fine.
    if [ -n "$existing_policy" ] && echo "$existing_policy" | grep -q "$topic_arn"; then
        echo "SQS policy already allows SNS topic $topic_arn"
        return 0
    fi

    local attrs_file
    attrs_file=$(mktemp)

    # The --attributes value must be a JSON object where the Policy value is
    # itself a JSON-encoded string (double-escaped). Using file:// avoids all
    # shell quoting issues with embedded double quotes.
    cat > "$attrs_file" <<EOF
{"Policy":"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Sid\":\"Allow-SNS-SendMessage\",\"Effect\":\"Allow\",\"Principal\":\"*\",\"Action\":\"sqs:SendMessage\",\"Resource\":\"${queue_arn}\",\"Condition\":{\"ArnEquals\":{\"aws:SourceArn\":\"${topic_arn}\"}}}]}"}
EOF

    retry awslocal_cmd sqs set-queue-attributes \
        --queue-url "$queue_url" \
        --attributes "file://$attrs_file" >/dev/null

    rm -f "$attrs_file"
}

subscribe_if_missing() {
    local topic_arn="$1"
    local queue_url="$2"

    local queue_arn
    queue_arn=$(awslocal_cmd sqs get-queue-attributes \
        --queue-url "$queue_url" \
        --attribute-names QueueArn \
        --query "Attributes.QueueArn" \
        --output text)

    # check existing subscriptions
    if awslocal_cmd sns list-subscriptions-by-topic \
        --topic-arn "$topic_arn" \
        --query "Subscriptions[?Endpoint=='$queue_arn']" \
        --output text | grep -q "$queue_arn"; then
        echo "Subscription already exists for $queue_arn"
        return 0
    fi

    retry awslocal_cmd sns subscribe \
        --topic-arn "$topic_arn" \
        --protocol sqs \
        --notification-endpoint "$queue_arn"
}

ensure_sqs_policy_allows_sns "$ORDER_TOPIC_ARN" "$ORDER_QUEUE_URL"
subscribe_if_missing "$ORDER_TOPIC_ARN" "$ORDER_QUEUE_URL"
ensure_sqs_policy_allows_sns "$PAYMENT_TOPIC_ARN" "$PAYMENT_EVENTS_QUEUE_URL"
subscribe_if_missing "$PAYMENT_TOPIC_ARN" "$PAYMENT_EVENTS_QUEUE_URL"

# Dedicated notification-events topic → notification queue.
# All publisher services send NotificationEvent messages to this single topic;
# business events stay on their own topics and never reach the notification queue.
ensure_sqs_policy_allows_sns "$NOTIFICATION_TOPIC_ARN" "$NOTIFICATION_QUEUE_URL"
subscribe_if_missing "$NOTIFICATION_TOPIC_ARN" "$NOTIFICATION_QUEUE_URL"

# Promotion order queue for coupon usage increments
ensure_sqs_policy_allows_sns "$NOTIFICATION_TOPIC_ARN" "$PROMOTION_ORDER_QUEUE_URL"
subscribe_if_missing "$NOTIFICATION_TOPIC_ARN" "$PROMOTION_ORDER_QUEUE_URL"


# --------------------------------------------------
# EC2 (dev-only, safe ignore if already exists)
# --------------------------------------------------
retry awslocal_cmd ec2 run-instances \
    --image-id ami-ff000000 \
    --count 1 \
    --instance-type t2.micro \
    --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=local-dev-instance}]' \
    || echo "EC2 instance may already exist (ignored for local dev)"


echo "LocalStack resources initialized successfully."
