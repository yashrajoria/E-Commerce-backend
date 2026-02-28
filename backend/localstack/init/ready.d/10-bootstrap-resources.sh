#!/bin/bash
set -euo pipefail

set -x

echo "Initializing LocalStack resources..."

# --------------------------------------------------
# Config (names driven by env; safe defaults)
# --------------------------------------------------
ORDER_TOPIC_NAME="${ORDER_SNS_TOPIC_NAME:-order-events}"
PAYMENT_TOPIC_NAME="${PAYMENT_SNS_TOPIC_NAME:-payment-events}"

ORDER_QUEUE_NAME="${ORDER_PROCESSING_QUEUE_NAME:-order-processing-queue}"
PAYMENT_EVENTS_QUEUE_NAME="${PAYMENT_EVENTS_QUEUE_NAME:-payment-events-queue}"
PAYMENT_REQUEST_QUEUE_NAME="${PAYMENT_REQUEST_QUEUE_NAME:-payment-request-queue}"
NOTIFICATION_QUEUE_NAME="${NOTIFICATION_SQS_QUEUE_NAME:-notification-queue}"

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

if ! awslocal s3api head-bucket --bucket "$BUCKET_NAME" 2>/dev/null; then
    retry awslocal s3 mb "s3://$BUCKET_NAME"
else
    echo "S3 bucket '$BUCKET_NAME' already exists"
fi


# --------------------------------------------------
# DynamoDB (idempotent)
# --------------------------------------------------
create_table_if_missing() {
    local tbl_name="$1"

    if awslocal dynamodb describe-table --table-name "$tbl_name" >/dev/null 2>&1; then
        echo "DynamoDB table '$tbl_name' already exists"
        return 0
    fi

    echo "Creating DynamoDB table '$tbl_name'..."

    retry awslocal dynamodb create-table \
        --table-name "$tbl_name" \
        --attribute-definitions AttributeName=id,AttributeType=S \
        --key-schema AttributeName=id,KeyType=HASH \
        --billing-mode PAY_PER_REQUEST

    # wait until ACTIVE
    retry awslocal dynamodb wait table-exists --table-name "$tbl_name"
}

create_table_if_missing "${DDB_TABLE_PRODUCTS:-Products}"
create_table_if_missing "${DDB_TABLE_CATEGORIES:-Categories}"
create_table_if_missing "${DDB_TABLE_INVENTORY:-Inventory}"


# --------------------------------------------------
# SNS (idempotent)
# --------------------------------------------------
create_topic_if_missing() {
    local topic_name="$1"

    awslocal sns create-topic \
        --name "$topic_name" \
        --query "TopicArn" \
        --output text
}

ORDER_TOPIC_ARN=$(retry create_topic_if_missing "$ORDER_TOPIC_NAME")
PAYMENT_TOPIC_ARN=$(retry create_topic_if_missing "$PAYMENT_TOPIC_NAME")


# --------------------------------------------------
# SQS (idempotent)
# --------------------------------------------------
create_queue_if_missing() {
    local queue_name="$1"

    awslocal sqs create-queue \
        --queue-name "$queue_name" \
        --query "QueueUrl" \
        --output text
}

ORDER_QUEUE_URL=$(retry create_queue_if_missing "$ORDER_QUEUE_NAME")
PAYMENT_EVENTS_QUEUE_URL=$(retry create_queue_if_missing "$PAYMENT_EVENTS_QUEUE_NAME")
PAYMENT_REQUEST_QUEUE_URL=$(retry create_queue_if_missing "$PAYMENT_REQUEST_QUEUE_NAME")
NOTIFICATION_QUEUE_URL=$(retry create_queue_if_missing "$NOTIFICATION_QUEUE_NAME")


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
    queue_arn=$(awslocal sqs get-queue-attributes \
        --queue-url "$queue_url" \
        --attribute-names QueueArn \
        --query "Attributes.QueueArn" \
        --output text)

    local existing_policy
    existing_policy=$(awslocal sqs get-queue-attributes \
        --queue-url "$queue_url" \
        --attribute-names Policy \
        --query "Attributes.Policy" \
        --output text 2>/dev/null || true)

    # If the policy already references this topic ARN, assume it's fine.
    if [ -n "$existing_policy" ] && echo "$existing_policy" | grep -q "$topic_arn"; then
        echo "SQS policy already allows SNS topic $topic_arn"
        return 0
    fi

    local policy
    policy=$(cat <<EOF
{"Version":"2012-10-17","Statement":[{"Sid":"Allow-SNS-SendMessage","Effect":"Allow","Principal":"*","Action":"sqs:SendMessage","Resource":"$queue_arn","Condition":{"ArnEquals":{"aws:SourceArn":"$topic_arn"}}}]}
EOF
)

    retry awslocal sqs set-queue-attributes \
        --queue-url "$queue_url" \
        --attributes Policy="$policy" >/dev/null
}

subscribe_if_missing() {
    local topic_arn="$1"
    local queue_url="$2"

    local queue_arn
    queue_arn=$(awslocal sqs get-queue-attributes \
        --queue-url "$queue_url" \
        --attribute-names QueueArn \
        --query "Attributes.QueueArn" \
        --output text)

    # check existing subscriptions
    if awslocal sns list-subscriptions-by-topic \
        --topic-arn "$topic_arn" \
        --query "Subscriptions[?Endpoint=='$queue_arn']" \
        --output text | grep -q "$queue_arn"; then
        echo "Subscription already exists for $queue_arn"
        return 0
    fi

    retry awslocal sns subscribe \
        --topic-arn "$topic_arn" \
        --protocol sqs \
        --notification-endpoint "$queue_arn"
}

ensure_sqs_policy_allows_sns "$ORDER_TOPIC_ARN" "$ORDER_QUEUE_URL"
subscribe_if_missing "$ORDER_TOPIC_ARN" "$ORDER_QUEUE_URL"
ensure_sqs_policy_allows_sns "$PAYMENT_TOPIC_ARN" "$PAYMENT_EVENTS_QUEUE_URL"
subscribe_if_missing "$PAYMENT_TOPIC_ARN" "$PAYMENT_EVENTS_QUEUE_URL"

# Fan-out order/payment events into the shared notification queue so that
# notification-service can react to any event type without coupling tightly
# to individual service queues.
ensure_sqs_policy_allows_sns "$ORDER_TOPIC_ARN" "$NOTIFICATION_QUEUE_URL"
subscribe_if_missing "$ORDER_TOPIC_ARN" "$NOTIFICATION_QUEUE_URL"
ensure_sqs_policy_allows_sns "$PAYMENT_TOPIC_ARN" "$NOTIFICATION_QUEUE_URL"
subscribe_if_missing "$PAYMENT_TOPIC_ARN" "$NOTIFICATION_QUEUE_URL"


# --------------------------------------------------
# EC2 (dev-only, safe ignore if already exists)
# --------------------------------------------------
retry awslocal ec2 run-instances \
    --image-id ami-ff000000 \
    --count 1 \
    --instance-type t2.micro \
    --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=local-dev-instance}]' \
    || echo "EC2 instance may already exist (ignored for local dev)"


echo "LocalStack resources initialized successfully."
