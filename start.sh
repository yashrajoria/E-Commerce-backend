#!/bin/bash
set -e

echo "=========================================="
echo " Starting ShopSwift Multi-Service Backend "
echo "=========================================="

# Render automatically provides $PORT (e.g. 10000 or custom)
PUBLIC_PORT="${PORT:-8080}"

# Direct all service-to-service internal traffic to localhost ports
export PRODUCT_SERVICE_URL="http://localhost:8082"
export USER_SERVICE_URL="http://localhost:8081"
export AUTH_SERVICE_URL="http://localhost:8081"
export CART_SERVICE_URL="http://localhost:8082"
export ORDER_SERVICE_URL="http://localhost:8083"
export INVENTORY_SERVICE_URL="http://localhost:8082"
export NOTIFICATION_SERVICE_URL="http://localhost:8092"

# Ensure GORM creates tables on Supabase if they don't exist yet
export ALLOW_AUTO_MIGRATE="${ALLOW_AUTO_MIGRATE:-true}"
export GIN_MODE="${GIN_MODE:-release}"

# Route AWS SDK to embedded local DynamoDB
export USE_LOCALSTACK=true
export LOCALSTACK_ENDPOINT="http://localhost:8000"
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-test}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-test}"
export AWS_EC2_METADATA_DISABLED="true"
export AWS_USE_SECRETS="false"
export CLOUDWATCH_ENABLED="false"

# Disable SQS background polling on single container (no AWS SQS in free tier)
export ENABLE_SQS_CONSUMER="false"
export SQS_QUEUE_URL=""
export NOTIFICATION_SQS_QUEUE_URL=""

# If OTEL_EXPORTER_OTLP_ENDPOINT points to local docker compose jaeger, disable it
case "${OTEL_EXPORTER_OTLP_ENDPOINT:-}" in
    *jaeger*)
        echo "-> Disabling unreachable jaeger tracing endpoint in cloud container"
        unset OTEL_EXPORTER_OTLP_ENDPOINT
        ;;
esac

# Graceful shutdown handler
cleanup() {
    echo "Shutting down services..."
    kill $(jobs -p) 2>/dev/null || true
    exit 0
}
trap cleanup SIGINT SIGTERM

echo "-> Starting embedded DynamoDB on :8000..."
if [ -d "/home/dynamodblocal" ] && [ -f "/home/dynamodblocal/DynamoDBLocal.jar" ]; then
    (cd /home/dynamodblocal && java -Xmx64m -jar DynamoDBLocal.jar -inMemory -sharedDb -port 8000) &
elif [ -f "/opt/dynamodb/DynamoDBLocal.jar" ]; then
    (cd /opt/dynamodb && java -Xmx64m -jar DynamoDBLocal.jar -inMemory -sharedDb -port 8000) &
else
    echo "Warning: DynamoDBLocal.jar not found, attempting generic java start..."
    java -Xmx64m -jar DynamoDBLocal.jar -inMemory -sharedDb -port 8000 &
fi

# Wait for DynamoDB to accept connections
sleep 2

echo "-> Initializing DynamoDB tables & seed catalog..."
/app/init-dynamo || echo "init-dynamo completed with warnings"

echo "-> Launching Identity Service on :8081..."
PORT=8081 /app/identity-service &

echo "-> Launching Catalog Service on :8082..."
PORT=8082 /app/catalog-service &

echo "-> Launching Order Service on :8083..."
PORT=8083 /app/order-service &

echo "-> Launching Notification Service on :8092..."
PORT=8092 /app/notification-service &

# Allow internal microservices to initialize DB & Redis connections
sleep 3

echo "-> Launching API Gateway on public port :${PUBLIC_PORT}..."
PORT=${PUBLIC_PORT} exec /app/api-gateway
