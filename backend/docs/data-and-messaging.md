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
| Catalog + categories, stock, cart | **catalog-service** | DynamoDB `Products`, `Categories`, `ProductCategories`, `Inventory`; S3 images (Mongo retired); Redis cart + cache |
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

## DynamoDB

| Table | Env var | Service |
|-------|---------|---------|
| Products | `DDB_TABLE_PRODUCTS` | catalog-service |
| Categories | `DDB_TABLE_CATEGORIES` | catalog-service |
| ProductCategories | `DDB_TABLE_PRODUCT_CATEGORIES` | catalog-service (category→product adjacency) |
| Inventory | `DDB_TABLE_INVENTORY` | catalog-service |

### GSIs and Query vs Scan

| Access path | Access method | Index / notes |
|-------------|---------------|---------------|
| Product by `id` | `GetItem` | Table PK |
| Product by SKU (`FindBySKUs`) | `Query` | `sku-index` (HASH `sku`) |
| Featured-only list (`is_featured` only) | `Query` | `featured-index` (HASH `is_featured` as `"true"`/`"false"`, RANGE `created_at`) |
| Products by category | `Query` + `BatchGetItem` | `ProductCategories` (HASH `category_id`, RANGE `product_id`); GSI `product-index` for product→categories |
| Product multi-filter list / count (no category) | `Scan` + `FilterExpression` | brand, price, stock, etc. |
| Category by `id` | `GetItem` | Table PK |
| Category by name | `Query` | `name-index` (HASH `name`) |
| Category `FindAll` | `Scan` | Soft-delete filter |
| Category `HasProducts` | `Query` | `ProductCategories` Limit 1 |
| Inventory get / reserve | `GetItem` / conditional update | Table PK |
| Inventory admin `ListAll` | `Scan` | Acceptable for admin |

`is_featured` is stored as a DynamoDB string (`"true"` / `"false"`) so it can be a GSI HASH key; the HTTP/API layer still exposes a bool.

**LocalStack note:** Existing volumes with old table schemas will not gain GSIs/tables automatically. After pulling schema changes, recreate the LocalStack volume or create missing tables (e.g. `ProductCategories`) once.

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
DDB_TABLE_PRODUCTS=Products
DDB_TABLE_CATEGORIES=Categories
DDB_TABLE_INVENTORY=Inventory
USE_LOCALSTACK=true
LOCALSTACK_ENDPOINT=http://localstack:4566
ALLOW_AUTO_MIGRATE=true   # local DX; false in prod
```

Legacy `DYNAMODB_*` names in older env files are deprecated — use `DDB_TABLE_*`.
