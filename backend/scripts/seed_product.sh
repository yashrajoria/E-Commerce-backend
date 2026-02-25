#!/usr/bin/env sh
# Seed a product into LocalStack DynamoDB via awslocal inside the localstack container.
# Usage: ./scripts/seed_product.sh <product-id> <name> <price>

if [ -z "$1" ]; then
  echo "Usage: $0 <product-id> <name> <price>"
  exit 1
fi
ID="$1"
NAME="${2:-Seeded Test Product}"
PRICE="${3:-1000}"

# run awslocal inside the localstack container
# requires docker compose to be available and the service named 'localstack' in docker-compose.yml

docker compose exec localstack sh -c "awslocal dynamodb put-item --table-name Products --item '{\"id\":{\"S\":\"$ID\"},\"name\":{\"S\":\"$NAME\"},\"price\":{\"N\":\"$PRICE\"}}'"

echo "Seeded product $ID" > /dev/stderr
