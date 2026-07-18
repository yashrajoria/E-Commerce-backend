# Data and messaging

Source of truth for local/prod data stores and async messaging. Aligns with Compose + LocalStack bootstrap.

## Ownership summary

| Concern | Owner service | Store |
|---------|---------------|-------|
| Credentials, refresh tokens, verification | **auth-service** | Postgres `users`, `refresh_tokens` |
| Profile, phone, addresses | **user-service** | Postgres `users` (profile cols), `addresses` |
| Catalog + categories | **product-service** | DynamoDB `Products`, `Categories`, `ProductCategories`; S3 images (Mongo retired) |
| Stock | **inventory-service** | DynamoDB `Inventory` |
| Cart | **cart-service** | Redis only |
| Orders | **order-service** | Postgres `orders`, `order_items` |
| Payments | **payment-service** | Postgres `payments`, `stripe_processed_events` |
| Coupons | **promotion-service** | Postgres `coupons` |
| Shipping rates | **shipping-service** | In-process rate provider (no DB at runtime) |
| Notification logs | **notification-service** | Postgres `notification_logs` |

**Auth vs user on `users`:** Auth owns identity columns (email, password hash, role, verification). User owns profile/address fields. AutoMigrate is gated solely by `ALLOW_AUTO_MIGRATE` (not `ENV`). Each service migrates once inside `database.Connect` — auth: `User` + `RefreshToken`; user: `Address`. Prefer SQL migrations in production (`ALLOW_AUTO_MIGRATE=false`).

**RBAC:** Gateway validates JWT and injects `X-User-*` (client-supplied identity headers are stripped). Admin routes require `role=admin`. Product/inventory writes and auth `POST /auth/admin/users` also enforce admin at the service. Token refresh reloads role from Postgres. Admin UI requires admin on login and protected routes.

**Admin bootstrap:** On auth-service startup, if `ADMIN_EMAIL` and `ADMIN_PASSWORD` are set and **no** `role=admin` user exists, auth creates a verified admin (idempotent). Public self-registration always creates `role=user`. Additional admins are created only via `POST /auth/admin/users` (JWT + admin role). Never leave a weak `ADMIN_PASSWORD` in production secrets.

## Postgres (`ecommerce`)

| Table | Created by |
|-------|------------|
| `users` | Migrations + auth AutoMigrate (gated) |
| `refresh_tokens` | Migrations + auth |
| `addresses` | Migrations + user |
| `orders`, `order_items` | Migrations + order |
| `payments` | Migrations + payment |
| `stripe_processed_events` | Migrations + payment (webhook dedup) |
| `coupons` | Migrations + promotion |
| `notification_logs` | Migrations + notification |
| `shipments` | SQL migration only — **unused** by runtime shipping-service (future) |

Run: `./scripts/migrate.sh up` from `backend/`.

## DynamoDB

| Table | Env var | Service |
|-------|---------|---------|
| Products | `DDB_TABLE_PRODUCTS` | product-service |
| Categories | `DDB_TABLE_CATEGORIES` | product-service |
| ProductCategories | `DDB_TABLE_PRODUCT_CATEGORIES` | product-service (category→product adjacency) |
| Inventory | `DDB_TABLE_INVENTORY` | inventory-service |

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
| Cart + checkout keys | cart-service |
| Checkout SetNX / result | bff-service |
| Rate limiting | api-gateway |
| Product cache | product-service (version bump on product **and** category mutations) |

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
| `payment-request-queue` | order-service → payment-service |
| `notification-queue` | SNS notification-events → notification-service |
| `promotion-order-queue` | SNS (order-related) → promotion-service usage |

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
