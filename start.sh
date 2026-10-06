#!/bin/bash
set -e

echo "=========================================="
echo " Starting ShopSwift Multi-Service Backend "
echo "=========================================="

# Render automatically provides $PORT (e.g. 10000 or custom)
# API Gateway listens on this port to receive incoming web traffic.
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

# Disable AWS IMDS metadata lookups and AWS Secrets Manager on non-AWS hosts
export AWS_REGION="${AWS_REGION:-us-east-1}"
export AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-test}"
export AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-test}"
export AWS_EC2_METADATA_DISABLED="${AWS_EC2_METADATA_DISABLED:-true}"
export AWS_USE_SECRETS="${AWS_USE_SECRETS:-false}"
export CLOUDWATCH_ENABLED="${CLOUDWATCH_ENABLED:-false}"

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
