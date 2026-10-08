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

# Supabase & PostgreSQL Messaging defaults (zero external AWS required)
export ORDER_SNS_TOPIC_ARN="${ORDER_SNS_TOPIC_ARN:-order-events}"
export PAYMENT_SNS_TOPIC_ARN="${PAYMENT_SNS_TOPIC_ARN:-payment-events}"
export AUTH_SNS_TOPIC_ARN="${AUTH_SNS_TOPIC_ARN:-auth-events}"
export NOTIFICATION_SNS_TOPIC_ARN="${NOTIFICATION_SNS_TOPIC_ARN:-notification-queue}"
export CHECKOUT_QUEUE_URL="${CHECKOUT_QUEUE_URL:-order-processing-queue}"
export PAYMENT_EVENTS_QUEUE_URL="${PAYMENT_EVENTS_QUEUE_URL:-payment-events-queue}"
export PAYMENT_REQUEST_QUEUE_URL="${PAYMENT_REQUEST_QUEUE_URL:-payment-request-queue}"
export NOTIFICATION_SQS_QUEUE_URL="${NOTIFICATION_SQS_QUEUE_URL:-notification-queue}"

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
