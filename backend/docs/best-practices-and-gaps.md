# Best practices and gaps

This document tracks engineering practices for the ShopSwift backend: what this repo ships today, and what remains as follow-up work.

Related: [architecture.md](./architecture.md) · [data-and-messaging.md](./data-and-messaging.md)

## Already in good shape

- API Gateway + BFF split; Redis checkout SetNX + `Idempotency-Key`
- LocalStack bootstrap for S3 / DynamoDB / SNS / SQS (`docker-compose.localstack.yml`, `./scripts/dev-up.sh`)
- Stripe CLI webhook forwarding in Compose
- OpenAPI + Spectral lint in CI
- Terraform / OIDC deploy scaffolding under `infrastructure/aws`
- Order/payment SQS consumers with `idempotency_key` columns

## Shipped in the P0/P1 pass

| Item | Status |
|------|--------|
| SQL migrations via golang-migrate (`migrations/000001_baseline.*`, `./scripts/migrate.sh`) | Done |
| `ALLOW_AUTO_MIGRATE` gate (default true locally; false in prod) | Done |
| LocalStack required local path (`scripts/dev-up.sh`) | Done |
| Gateway `GET /health` (+ live/ready) so Compose healthcheck works | Done |
| `/health/live` on core services + Compose healthchecks | Done |
| Stripe webhook **event ID** dedup (`stripe_processed_events`) | Done |
| Auth owns identity AutoMigrate; user owns **addresses** only | Done |
| Gateway `X-Request-ID` / correlation + structured request logs | Done |
| `GIN_MODE=release` in Compose | Done |
| Shared `common/errors` middleware on gateway, BFF, order, payment | Done |

## Shipped in the DB hardening pass

| Item | Status |
|------|--------|
| Single AutoMigrate gate (`ALLOW_AUTO_MIGRATE`); no double migrate in auth/user `main` | Done |
| DynamoDB GSIs: `sku-index`, `featured-index`, `name-index` (LocalStack + Terraform) | Done |
| Hot paths use `Query`; multi-filter product list stays `Scan` | Done |
| Category mutations invalidate Redis product cache version | Done |
| Mongo helpers + `migrate-mongo-to-ddb` removed | Done |
| Shipping Postgres/shipment + unused StaticRateProvider purged; rates path kept | Done |
| `shipping-events` SNS topic dropped from LocalStack bootstrap | Done |
| Admin bootstrap via `ADMIN_EMAIL` / `ADMIN_PASSWORD` (idempotent; verified admin) | Done |
| Admin UI: login-only; invite via protected `/bff/admin/users` | Done |

## Shipped in the gaps-closure pass

| Item | Status |
|------|--------|
| Compose healthchecks use `wget` (Alpine images lack `curl`) | Done |
| Cart service `RequireUser` middleware (`X-User-ID` gate) | Done |
| OpenAPI: all `operationId`s; Inventory/Notification servers; Notification tag; examples; `x-retryable`; Spectral crash fixes (null examples / `$ref` siblings) | Done |
| `ProductCategories` adjacency table (LocalStack + Terraform + seed); category list/`HasProducts` via Query | Done |
| Stripe success/cancel via `FRONTEND_URL` → storefront `:3001` | Done |
| Targeted unit tests: cart auth, gateway admin/forwarder identity, payment frontend URL, auth refresh role, product category filter helpers | Done |
| PR/unit CI workflow [`.github/workflows/unit-tests.yml`](../.github/workflows/unit-tests.yml) | Done |

## Context7-aligned practice check (2026-07-18 refresh)

Re-checked via Context7 MCP against:

- AWS DynamoDB Developer Guide (`/websites/aws_amazon_amazondynamodb_developerguide`)
- Stripe Checkout fulfillment docs (`/websites/stripe`)
- PostgreSQL current docs (`/websites/postgresql_current`)

| Area | Guidance | ShopSwift status |
|------|----------|------------------|
| DynamoDB Query over Scan | Prefer Query/GSI; Scan only when no key | Hot paths Query; multi-filter product Scan remains (acceptable) |
| Many-to-many / adjacency | Adjacency list (PK + SK edges) | `ProductCategories` table |
| Conditional batch updates | `TransactWriteItems` + conditions | Inventory reserve/release/confirm |
| Transaction idempotency | `ClientRequestToken` on TransactWrite | Reserve **and** release/confirm (`v`/`r`/`c` + compact order id) |
| Nested map updates | Parent map must exist / `if_not_exists` | Reserve initializes `order_reservations` |
| Stripe webhook verify | `ConstructEvent` + signature secret | Done |
| Stripe fulfillment | Webhook primary; handle async success; safely re-runnable | `completed` + `async_payment_succeeded`; fulfill then record event id; terminal payment status guards concurrency |
| Postgres FKs / indexes | FK + indexes on join/lookup columns | Baseline FK + indexes; `000002` soft-delete + filter indexes |
| Isolation | Read Committed OK for row updates | Default PG / GORM |
| Coupon concurrency | Atomic conditional increment | `used_count` update with `usage_limit` guard |

### Applied in this refresh

- Inventory `ReleaseAll` / `ConfirmAll`: `ClientRequestToken` (distinct from reserve; ≤36 chars)
- Stripe webhook: fulfill first, mark `event_id` after success (so Stripe retries are not permanently swallowed)
- Migration `000002_soft_delete_indexes`: `deleted_at` + common filter indexes
- All SQL migrations made **idempotent** (`IF NOT EXISTS` / `duplicate_object` guards / `CASCADE` downs)
- Removed leftover Mongo `bson` tags from product/category domain models (DynamoDB-only persistence)

### Intentional deviations (personal project — keep; don’t “fix”)

This is a **personal** project. Prefer simplicity over enterprise isolation unless something actually hurts (latency, cost, or ops pain).

- **Shared Postgres `ecommerce` DB** — keep. DB-per-service adds ops cost with no payoff for a single operator. Table ownership in [data-and-messaging.md](./data-and-messaging.md) is enough.
- **No cross-aggregate FKs** for `orders.user_id` / `payments.order_id` — app-enforced integrity + indexes; fine with a shared DB.
- **Product multi-attribute browse still Scans** — keep until list latency or DynamoDB RU cost is noticeable; then add GSIs only for proven filters.
- **Multi-table DynamoDB** (Products / Categories / Inventory) — keep; access patterns are independent.
- **No OpenTelemetry yet** — structured logs (+ optional CloudWatch) are enough for personal/local; add OTel only if you want distributed-trace practice.

## Ops notes

- Prefer `./scripts/dev-up.sh` so LocalStack is always present.
- Production: `ALLOW_AUTO_MIGRATE=false` and run `./scripts/migrate.sh up` (or `infrastructure/aws/run_migrations.sh` with `DB_HOST=...`).
- Propagate `X-Request-ID` from clients; gateway generates one when missing and forwards it upstream.
- After LocalStack volume wipe: reseed with `./scripts/seed_demo_data.sh` (products + inventory + ProductCategories). Sync helpers: `--sync-inventory-only`, `--sync-category-links-only`.
