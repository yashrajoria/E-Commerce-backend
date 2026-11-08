# 🗄️ SQL Migrations Guide

The ShopSwift backend manages relational schemas with [golang-migrate](https://github.com/golang-migrate/migrate). Each migration is a pair: `NNNNNN_name.up.sql` and `NNNNNN_name.down.sql`.

There are 24 migrations, numbered `000001` to `000025`. **`000008` does not exist** (skipped, not deprecated). The `legacy/` folder holds three older dated scripts that are not run by golang-migrate.

---

## 📋 Migration Catalog

| Version | Name | Area | Key Schema Changes (verified in the `.up.sql`) |
|:---|:---|:---|:---|
| `000001` | `baseline` | Core | Initial tables, including `users`, `products`, `categories`, `orders`, `order_items`, `coupons`, `payments`, `shipments` |
| `000002` | `soft_delete_indexes` | Performance | Indexes for soft-deleted rows |
| `000003` | `verification_rate_limit` | Auth | Columns to rate-limit verification requests |
| `000004` | `refresh_token_family` | Auth | Refresh token family tracking for rotation and reuse detection |
| `000005` | `login_lockout` | Auth | Failed-attempt counters and lockout timestamps |
| `000006` | `hash_verification_code` | Auth | Verification codes stored as SHA-256 hashes |
| `000007` | `order_outbox_events` | Messaging | `outbox_events` table |
| `000009` | `agent_audit_log` | Agent | `agent_audit_log` table |
| `000010` | `payment_correlation_id` | Tracing | Indexed `payments.correlation_id` |
| `000011` | `payment_outbox_events` | Messaging | `payment_outbox_events` table |
| `000012` | `schema_integrity` | Data quality | Foreign keys, cascade rules, check constraints |
| `000013` | `indexes_retention_constraints` | Operations | Composite indexes and retention constraints |
| `000014` | `triggers_history_currency` | Audit | Triggers and `order_status_history` table |
| `000015` | `catalog_schema` | Schema | Creates the `catalog` schema |
| `000016` | `catalog_inventory` | Inventory | `catalog.inventory` and `catalog.stock_reservations` |
| `000017` | `message_queues` | Messaging | `message_queues` table (relational queue adapter) |
| `000018` | `supabase_storage` | Storage | Storage configuration for Supabase / S3-compatible setups |
| `000019` | `verification_code_expiry` | Auth | Expiry timestamps on verification codes |
| `000020` | `multi_tenant_stores` | Multi-tenancy | `stores`, `store_memberships`, `store_settings` |
| `000021` | `store_scoping` | Multi-tenancy | Adds store scoping to catalog, orders and coupons |
| `000022` | `subscription_plans` | SaaS | `plans`, `plan_entitlements`, `subscriptions`, `usage_records` |
| `000023` | `align_integer_columns` | Types | Aligns integer column types with Go models |
| `000024` | `admin_portal_tables` | Admin | `support_tickets`, `returns`, `reviews`, `activity_logs`, `campaigns`, related tables |
| `000025` | `store_credit` | Store credit | `store_credit_accounts`, `store_credit_ledger`, `order_payment_allocations`, `refunded_amount` columns |

Tables created by `CREATE TABLE` across all migrations also include `addresses`, `coupon_usages`, `notification_events`, `notification_logs`, `stripe_processed_events`, and `refresh_tokens`.

---

## 🔒 Idempotency

Migrations use re-runnable SQL:
- Tables: `CREATE TABLE IF NOT EXISTS`
- Indexes: `CREATE [UNIQUE] INDEX IF NOT EXISTS`
- Columns: `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`
- Functions: `CREATE OR REPLACE FUNCTION`
- Constraints: wrapped in `DO $$ ... EXCEPTION WHEN duplicate_object THEN NULL; END $$`

---

## 🚀 Running Migrations

Run from `backend/`:

```bash
# Apply all pending migrations
./scripts/migrate.sh up

# Roll back the most recent migration
./scripts/migrate.sh down 1

# Show current version
./scripts/migrate.sh version

# Force a version (recovery only)
./scripts/migrate.sh force 25
```

`scripts/migrate.sh` builds its connection string from `POSTGRES_*` (or `DATABASE_URL`). Defaults are `localhost:5432`, database `ecommerce`, `sslmode=disable`.

### ⚠️ Transaction Poolers (PgBouncer / Supabase)
Transaction poolers on port `6543` do not support advisory locks across pooled connections. `start.sh` detects `:6543` in the migrate URL and adds `x-no-lock=true` to it. Use a direct connection (port `5432`, session mode) if you run migrations by hand through a pooler.
