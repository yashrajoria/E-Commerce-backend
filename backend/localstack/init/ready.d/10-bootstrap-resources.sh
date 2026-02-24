#!/bin/bash
set -euo pipefail

set -x

echo "Initializing LocalStack resources..."

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

ORDER_TOPIC_ARN=$(retry create_topic_if_missing "order-events")
PAYMENT_TOPIC_ARN=$(retry create_topic_if_missing "payment-events")


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

ORDER_QUEUE_URL=$(retry create_queue_if_missing "order-processing-queue")
PAYMENT_EVENTS_QUEUE_URL=$(retry create_queue_if_missing "payment-events-queue")
retry create_queue_if_missing "payment-request-queue" >/dev/null


# --------------------------------------------------
# SNS → SQS Subscription (idempotent)
# --------------------------------------------------
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

subscribe_if_missing "$ORDER_TOPIC_ARN" "$ORDER_QUEUE_URL"
subscribe_if_missing "$PAYMENT_TOPIC_ARN" "$PAYMENT_EVENTS_QUEUE_URL"


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
