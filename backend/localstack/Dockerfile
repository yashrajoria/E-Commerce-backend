FROM localstack/localstack:latest

# Write the bootstrap script inline to avoid CRLF issues on Windows hosts.
# No COPY needed - the content is generated directly inside the Linux image.
RUN mkdir -p /etc/localstack/init/ready.d

RUN printf '%s\n' \
  '#!/bin/bash' \
  'set -euo pipefail' \
  'set -x' \
  'echo "Initializing LocalStack resources..."' \
  '' \
  'retry() {' \
  '  n=0; max=6; delay=2' \
  '  while true; do' \
  '    if "$@"; then return 0; fi' \
  '    n=$((n+1))' \
  '    if [ "$n" -ge "$max" ]; then echo "Failed after $n attempts: $*" >&2; return 1; fi' \
  '    sleep $delay; delay=$((delay * 2))' \
  '  done' \
  '}' \
  '' \
  '# 1. S3' \
  'retry awslocal s3 mb s3://${AWS_S3_BUCKET:-shopswift} || true' \
  '' \
  '# 2. DynamoDB Tables' \
  'retry awslocal dynamodb create-table \' \
  '  --table-name ${DDB_TABLE_PRODUCTS:-Products} \' \
  '  --attribute-definitions AttributeName=id,AttributeType=S \' \
  '  --key-schema AttributeName=id,KeyType=HASH \' \
  '  --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 || true' \
  '' \
  'retry awslocal dynamodb create-table \' \
  '  --table-name ${DDB_TABLE_CATEGORIES:-Categories} \' \
  '  --attribute-definitions AttributeName=id,AttributeType=S \' \
  '  --key-schema AttributeName=id,KeyType=HASH \' \
  '  --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 || true' \
  '' \
  'retry awslocal dynamodb create-table \' \
  '  --table-name ${DDB_TABLE_INVENTORY:-Inventory} \' \
  '  --attribute-definitions AttributeName=id,AttributeType=S \' \
  '  --key-schema AttributeName=id,KeyType=HASH \' \
  '  --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 || true' \
  '' \
  '# 3. SNS Topics' \
  'ORDER_TOPIC_ARN=$(retry awslocal sns create-topic --name order-events --query TopicArn --output text || true)' \
  'PAYMENT_TOPIC_ARN=$(retry awslocal sns create-topic --name payment-events --query TopicArn --output text || true)' \
  '' \
  '# 4. SQS Queues' \
  'ORDER_QUEUE_URL=$(retry awslocal sqs create-queue --queue-name order-processing-queue --query QueueUrl --output text || true)' \
  'PAYMENT_EVENTS_QUEUE_URL=$(retry awslocal sqs create-queue --queue-name payment-events-queue --query QueueUrl --output text || true)' \
  'retry awslocal sqs create-queue --queue-name payment-request-queue || true' \
  '' \
  '# 5. SNS -> SQS Subscriptions' \
  'if [ -n "$ORDER_QUEUE_URL" ] && [ -n "$ORDER_TOPIC_ARN" ]; then' \
  '  ORDER_QUEUE_ARN=$(awslocal sqs get-queue-attributes --queue-url "$ORDER_QUEUE_URL" --attribute-names QueueArn --query Attributes.QueueArn --output text || true)' \
  '  [ -n "$ORDER_QUEUE_ARN" ] && awslocal sns subscribe --topic-arn "$ORDER_TOPIC_ARN" --protocol sqs --notification-endpoint "$ORDER_QUEUE_ARN" || true' \
  'fi' \
  '' \
  'if [ -n "$PAYMENT_EVENTS_QUEUE_URL" ] && [ -n "$PAYMENT_TOPIC_ARN" ]; then' \
  '  PAYMENT_EVENTS_QUEUE_ARN=$(awslocal sqs get-queue-attributes --queue-url "$PAYMENT_EVENTS_QUEUE_URL" --attribute-names QueueArn --query Attributes.QueueArn --output text || true)' \
  '  [ -n "$PAYMENT_EVENTS_QUEUE_ARN" ] && awslocal sns subscribe --topic-arn "$PAYMENT_TOPIC_ARN" --protocol sqs --notification-endpoint "$PAYMENT_EVENTS_QUEUE_ARN" || true' \
  'fi' \
  '' \
  '# 6. EC2 dummy instance (ignored on failure)' \
  'retry awslocal ec2 run-instances --image-id ami-ff000000 --count 1 --instance-type t2.micro --tag-specifications '\''ResourceType=instance,Tags=[{Key=Name,Value=local-dev-instance}]'\'' || true' \
  '' \
  'echo "LocalStack resources initialized successfully."' \
  > /etc/localstack/init/ready.d/10-bootstrap-resources.sh

RUN chmod +x /etc/localstack/init/ready.d/10-bootstrap-resources.sh
