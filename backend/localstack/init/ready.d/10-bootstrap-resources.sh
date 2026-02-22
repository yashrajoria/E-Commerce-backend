#!/bin/bash
set -euo pipefail

# Enable debug mode to see commands in logs
set -x

echo "Initializing LocalStack resources..."

# 1. S3
awslocal s3 mb s3://${AWS_S3_BUCKET:-shopswift} || true

# 2. DynamoDB Tables
awslocal dynamodb create-table \
    --table-name ${DDB_TABLE_PRODUCTS:-Products} \
    --attribute-definitions AttributeName=id,AttributeType=S \
    --key-schema AttributeName=id,KeyType=HASH \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 || true

awslocal dynamodb create-table \
    --table-name ${DDB_TABLE_CATEGORIES:-Categories} \
    --attribute-definitions AttributeName=category_id,AttributeType=S \
    --key-schema AttributeName=category_id,KeyType=HASH \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 || true

awslocal dynamodb create-table \
    --table-name ${DDB_TABLE_INVENTORY:-Inventory} \
    --attribute-definitions AttributeName=product_id,AttributeType=S \
    --key-schema AttributeName=product_id,KeyType=HASH \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 || true

# 3. SNS Topics
ORDER_TOPIC_ARN=$(awslocal sns create-topic --name order-events --query "TopicArn" --output text)
PAYMENT_TOPIC_ARN=$(awslocal sns create-topic --name payment-events --query "TopicArn" --output text)

# 4. SQS Queues
ORDER_QUEUE_URL=$(awslocal sqs create-queue --queue-name order-processing-queue --query "QueueUrl" --output text)
PAYMENT_EVENTS_QUEUE_URL=$(awslocal sqs create-queue --queue-name payment-events-queue --query "QueueUrl" --output text)
awslocal sqs create-queue --queue-name payment-request-queue

# 5. SNS -> SQS Subscriptions
ORDER_QUEUE_ARN=$(awslocal sqs get-queue-attributes --queue-url $ORDER_QUEUE_URL --attribute-names QueueArn --query "Attributes.QueueArn" --output text)
awslocal sns subscribe --topic-arn $ORDER_TOPIC_ARN --protocol sqs --notification-endpoint $ORDER_QUEUE_ARN

PAYMENT_EVENTS_QUEUE_ARN=$(awslocal sqs get-queue-attributes --queue-url $PAYMENT_EVENTS_QUEUE_URL --attribute-names QueueArn --query "Attributes.QueueArn" --output text)
awslocal sns subscribe --topic-arn $PAYMENT_TOPIC_ARN --protocol sqs --notification-endpoint $PAYMENT_EVENTS_QUEUE_ARN

# 6. EC2 Instance
awslocal ec2 run-instances \
    --image-id ami-ff000000 \
    --count 1 \
    --instance-type t2.micro \
    --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=local-dev-instance}]'

echo "LocalStack resources initialized successfully."