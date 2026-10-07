## Correlation metadata

The API gateway normalizes `X-Request-ID` and `X-Correlation-ID` to the same value. Internal HTTP clients forward both headers. Existing checkout, payment, and notification event payloads optionally include `correlation_id`; consumers preserve it across outbox, SNS, SQS retry, and failure paths. Payment rows persist the value so webhook-triggered payment events retain the original checkout correlation. Consumers generate a fallback only when accepting a legacy event without correlation metadata.

# Data and messaging

Source of truth for local/prod data stores and async messaging. Aligns with Compose + LocalStack bootstrap.

## Order-service outbox delivery

Order creation writes downstream events to the Postgres `outbox_events` table transactionally. The order-service publisher claims rows with a process lease, routes each row by `destination_type` to SQS or SNS, and marks it `published` only after the AWS publish succeeds. Failed sends return to `pending` with backoff. Expired leases are reclaimed, so a crash after publishing but before the status update can produce a duplicate; downstream consumers must use their existing idempotency controls.

## Ownership summary

| Concern | Owner service | Store |
|---------|---------------|-------|
| Credentials, refresh tokens, verification, profile, phone, addresses | **identity-service** | Postgres `users`, `refresh_tokens`, `addresses` |
| Catalog + categories, stock, cart | **catalog-service** | Postgres schema `catalog` (`products`, `categories`, `product_categories`, `inventory`, `stock_reservations`); S3 images; Redis cart + cache |
| Orders, coupons, shipping rates, payments | **order-service** | Postgres `orders`, `order_items`, `coupons`, `payments`, `stripe_processed_events`, `payment_outbox_events` |
| Notification logs | **notification-service** | Postgres `notification_logs` |

**Identity on `users`:** identity-service owns the whole table (credentials, verification, lockout, profile, addresses) with one `User` model. AutoMigrate is gated solely by `ALLOW_AUTO_MIGRATE` (not `ENV`) and runs once inside `database.Connect` (`User` + `RefreshToken` + `Address`). Prefer SQL migrations in production (`ALLOW_AUTO_MIGRATE=false`).

**RBAC:** Gateway validates JWT and injects `X-User-*` (client-supplied identity headers are stripped). Admin routes require `role=admin`. Product/inventory writes and auth `POST /auth/admin/users` also enforce admin at the service. Token refresh reloads role from Postgres. Admin UI requires admin on login and protected routes.

**Admin bootstrap:** On identity-service startup, if `ADMIN_EMAIL` and `ADMIN_PASSWORD` are set and **no** `role=admin` user exists, identity creates a verified admin (idempotent). Public self-registration always creates `role=user`. Additional admins are created only via `POST /auth/admin/users` (JWT + admin role). Never leave a weak `ADMIN_PASSWORD` in production secrets.

## Postgres (`ecommerce`)

| Table | Created by |
|-------|------------|
| `users` | Migrations + identity AutoMigrate (gated) |
| `refresh_tokens` | Migrations + identity |
| `addresses` | Migrations + identity |
| `orders`, `order_items` | Migrations + order |
| `coupons` | Migrations + order |
| `payments` | Migrations + order |
| `stripe_processed_events` | Migrations + order (webhook dedup) |
| `coupons` | Migrations + promotion |
| `notification_logs` | Migrations + notification |
| `shipments` | SQL migration only — **unused** by the runtime rate provider (future) |

Run: `./scripts/migrate.sh up` from `backend/`.

## Postgres `catalog` schema

| Table | Service |
|-------|---------|
| `products` | catalog-service |
| `categories` | catalog-service |
| `product_categories` | catalog-service (many-to-many link, indexed both ways) |
| `inventory` | catalog-service |
| `stock_reservations` | catalog-service (one row per order and product; reserve/confirm/release are idempotent) |

### Access paths

| Access path | Query | Index |
|-------------|-------|-------|
| Product by `id` | PK lookup | `products_pkey` |
| Product by SKU (`FindBySKUs`) | `WHERE sku IN (…)` | unique live index `idx_products_sku_live` |
| Product list / count (any filters) | one `WHERE` built from brand, price range, stock, featured, category (`EXISTS` on `product_categories`) | `idx_products_created_live`, `idx_products_brand_created_live`, `idx_products_featured_live` |
| Products by category | `IN` on `product_categories` | PK `(category_id, product_id)`; `idx_product_categories_product` for the reverse |
| Category by id / name / `FindAll` | indexed read / full read of a small table | `categories_pkey`, `idx_categories_name` |
| Inventory get / reserve | PK lookup / conditional `UPDATE … WHERE available >= qty` inside a transaction | `inventory_pkey` |
| Inventory admin `ListAll` | `ORDER BY product_id LIMIT/OFFSET` | PK |

Schema isolation: in production the service connects as `catalog_svc` (`backend/infrastructure/postgres/catalog_role.sql`), which can only use the `catalog` schema. No cross-schema joins or foreign keys.

## Redis

| Use | Service |
|-----|---------|
| Cart + checkout keys (`cart:user:*`, `idem:cart:*`) | catalog-service |
| Rate limiting | api-gateway |
| Product cache (version bump on product **and** category mutations) | catalog-service |

## S3

- Bucket default: `shopswift` (`AWS_S3_BUCKET`)
- Prefix: `products/` (`AWS_S3_PREFIX`)

## SNS topics (LocalStack bootstrap)

- `order-events`
- `payment-events`
- `auth-events`
- `promotion-events`
- `notification-events`

(`shipping-events` removed — no publisher/subscriber.)

## SQS queues (+ DLQs)

| Queue | Typical producer / consumer |
|-------|----------------------------|
| `order-processing-queue` | SNS order-events → order-service |
| `payment-events-queue` | SNS payment-events → order-service |
| `payment-request-queue` | order-service outbox → order-service payment consumer (same binary since merge; queue retained for durability) |
| `notification-queue` | SNS notification-events → notification-service |
| `promotion-order-queue` | SNS notification-events (`order_created`) → order-service coupon-usage consumer |

Each source queue has a matching `<source>-dlq` and a default `maxReceiveCount` of `3`. LocalStack reconciles this redrive policy on every bootstrap; Terraform exposes queue names and receive counts as variables.

## Env naming

Prefer:

```bash
USE_LOCALSTACK=true
LOCALSTACK_ENDPOINT=http://localstack:4566
ALLOW_AUTO_MIGRATE=true   # local DX; false in prod
CATALOG_DB_USER=          # production: catalog_svc (blank locally = the shared postgres user)
CATALOG_DB_PASSWORD=
```
