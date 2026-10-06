#!/usr/bin/env bash
set -euo pipefail
# Seed catalog tables in Postgres from dynamodb_data.json (DynamoDB-typed JSON; the
# filename is historical). Needs the catalog migrations applied.
#   DATABASE_URL=postgres://postgres:...@localhost:5432/ecommerce?sslmode=disable ./scripts/seed_catalog.sh
# No psql on the host? Run the converter and pipe it into the container instead:
#   python3 scripts/seed_catalog_postgres.py scripts/dynamodb_data.json | \
#     docker exec -i backend-postgres-1 psql -U postgres -d ecommerce -v ON_ERROR_STOP=1 -1
# Demo images (LocalStack S3 only): python3 scripts/seed_catalog_images.py
cd "$(dirname "$0")"
: "${DATABASE_URL:?set DATABASE_URL}"
python3 seed_catalog_postgres.py dynamodb_data.json | psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1
psql "$DATABASE_URL" -Atc "select 'categories', count(*) from catalog.categories union all select 'products', count(*) from catalog.products union all select 'links', count(*) from catalog.product_categories union all select 'inventory', count(*) from catalog.inventory"
