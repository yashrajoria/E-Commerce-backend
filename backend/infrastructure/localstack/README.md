LocalStack init helper

This folder contains a script to provision AWS resources in LocalStack for local development.

Usage:

Run LocalStack (e.g. docker-compose) and then:

```bash
./infrastructure/localstack/init-aws.sh
```

Or let Docker mount this script into LocalStack's `/etc/localstack/init/ready.d/` so it's executed automatically.

Resources created:

- S3 bucket: `ecommerce-product-images`
- SNS topic: `order-events`
- SQS queues: `order-processing-queue`, `email-notify-queue`, `order-processing-dlq`
- DynamoDB tables: `Products`, `Inventory`
