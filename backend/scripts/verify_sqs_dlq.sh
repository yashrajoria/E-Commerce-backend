#!/usr/bin/env bash
set -euo pipefail

# Verifies the five source queues and their explicit DLQs. Pass --exercise to
# send one probe message through each source queue until SQS moves it to its DLQ.

AWS_ENDPOINT="${LOCALSTACK_ENDPOINT:-}"
AWS_ARGS=()
if [ -n "$AWS_ENDPOINT" ]; then
    AWS_ARGS+=(--endpoint-url "$AWS_ENDPOINT")
fi

REGION="${AWS_REGION:-us-east-1}"
MAX_RECEIVE_COUNT="${SQS_MAX_RECEIVE_COUNT:-3}"
EXERCISE=false
if [ "${1:-}" = "--exercise" ]; then
    EXERCISE=true
fi

queues=(
    "${ORDER_PROCESSING_QUEUE_NAME:-order-processing-queue}"
    "${PAYMENT_EVENTS_QUEUE_NAME:-payment-events-queue}"
    "${PAYMENT_REQUEST_QUEUE_NAME:-payment-request-queue}"
    "${NOTIFICATION_SQS_QUEUE_NAME:-notification-queue}"
    "${PROMOTION_ORDER_QUEUE_NAME:-promotion-order-queue}"
)

queue_url() {
    aws --region "$REGION" "${AWS_ARGS[@]}" sqs get-queue-url --queue-name "$1" --query QueueUrl --output text
}

queue_attribute() {
    aws --region "$REGION" "${AWS_ARGS[@]}" sqs get-queue-attributes \
        --queue-url "$1" --attribute-names "$2" --query "Attributes.$2" --output text
}

for source_name in "${queues[@]}"; do
    source_url=$(queue_url "$source_name")
    dlq_name="${source_name}-dlq"
    dlq_url=$(queue_url "$dlq_name")
    source_arn=$(queue_attribute "$source_url" QueueArn)
    dlq_arn=$(queue_attribute "$dlq_url" QueueArn)
    redrive_policy=$(queue_attribute "$source_url" RedrivePolicy)

    python3 - "$redrive_policy" "$dlq_arn" "$MAX_RECEIVE_COUNT" <<'PY'
import json
import sys

policy = json.loads(sys.argv[1])
expected = {
    "deadLetterTargetArn": sys.argv[2],
    "maxReceiveCount": str(sys.argv[3]),
}
if policy != expected:
    raise SystemExit(f"redrive mismatch: actual={policy!r} expected={expected!r}")
PY
    echo "verified $source_name -> $dlq_name (maxReceiveCount=$MAX_RECEIVE_COUNT)"

    if "$EXERCISE" = true; then
        probe="sqs-dlq-verification-$(date +%s)-$$"
        aws --region "$REGION" "${AWS_ARGS[@]}" sqs send-message \
            --queue-url "$source_url" --message-body "$probe" >/dev/null

        moved=false
        for attempt in $(seq 1 $((MAX_RECEIVE_COUNT + 1))); do
            aws --region "$REGION" "${AWS_ARGS[@]}" sqs receive-message \
                --queue-url "$source_url" --visibility-timeout 0 --wait-time-seconds 1 \
                --query 'Messages[0].ReceiptHandle' --output text >/dev/null || true
            dlq_result=$(aws --region "$REGION" "${AWS_ARGS[@]}" sqs receive-message \
                --queue-url "$dlq_url" --visibility-timeout 0 --wait-time-seconds 1 \
                --query 'Messages[0].[ReceiptHandle,Body]' --output text 2>/dev/null || true)
            dlq_receipt=""
            dlq_probe=""
            if [ "$dlq_result" != "None" ]; then
                read -r dlq_receipt dlq_probe <<< "$dlq_result"
            fi
            if [ "$dlq_probe" = "$probe" ]; then
                moved=true
                break
            fi
        done
        if [ "$moved" != true ]; then
            echo "message did not reach $dlq_name" >&2
            exit 1
        fi
        echo "verified redrive exercise for $source_name"

        aws --region "$REGION" "${AWS_ARGS[@]}" sqs start-message-move-task \
            --source-arn "$dlq_arn" --destination-arn "$source_arn" >/dev/null

        redriven=false
        for attempt in $(seq 1 10); do
            source_result=$(aws --region "$REGION" "${AWS_ARGS[@]}" sqs receive-message \
                --queue-url "$source_url" --visibility-timeout 0 --wait-time-seconds 1 \
                --query 'Messages[0].[ReceiptHandle,Body]' --output text 2>/dev/null || true)
            source_receipt=""
            source_probe=""
            if [ "$source_result" != "None" ]; then
                read -r source_receipt source_probe <<< "$source_result"
            fi
            if [ "$source_probe" = "$probe" ]; then
                redriven=true
                aws --region "$REGION" "${AWS_ARGS[@]}" sqs delete-message \
                    --queue-url "$source_url" --receipt-handle "$source_receipt" >/dev/null
                break
            fi
        done
        if [ "$redriven" != true ]; then
            echo "message did not redrive from $dlq_name to $source_name" >&2
            exit 1
        fi
        echo "verified redrive-to-source for $source_name"
    fi
done

echo "SQS DLQ verification completed successfully."