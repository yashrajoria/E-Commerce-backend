# SQL migrations

Active migrations use [golang-migrate](https://github.com/golang-migrate/migrate) naming:

- `NNNNNN_name.up.sql` / `NNNNNN_name.down.sql`, currently `000001` to `000025` (`000008` is unused).

| Range | Area |
|-------|------|
| 000001-000006 | Baseline, soft-delete indexes, auth hardening (verification rate limit, refresh-token family, login lockout, hashed codes) |
| 000007, 000009-000011 | Order outbox, agent audit log, payment correlation ID, payment outbox |
| 000012-000014 | Schema integrity, indexes/retention, triggers/history/currency |
| 000015-000016 | `catalog` schema and inventory/reservations |
| 000017-000019 | Postgres message queues, Supabase storage, verification code expiry |
| 000020-000024 | Multi-tenant stores, store scoping, subscription plans, integer-column alignment, admin portal tables |
| 000025 | Store credit accounts, immutable ledger, and order payment allocations |

Apply:

```bash
./scripts/migrate.sh up
```

## Idempotency

All active and historical SQL in this folder is written to be **re-runnable**:

| Pattern | Use |
|---------|-----|
| `CREATE TABLE IF NOT EXISTS` | Tables |
| `CREATE [UNIQUE] INDEX IF NOT EXISTS` | Indexes (including uniqueness) |
| `CREATE EXTENSION IF NOT EXISTS` | Extensions |
| `CREATE OR REPLACE FUNCTION` | Functions |
| `DROP … IF EXISTS` (+ `CASCADE` on downs) | Teardown |
| `DO $$ … EXCEPTION WHEN duplicate_object` | FKs / CHECKs |

Unique constraints are enforced via **named unique indexes**, not inline `UNIQUE` column attrs, so a re-run still creates them if a prior AutoMigrate left the table without them. Phase 1 outbox and consumer-deduplication changes are intentionally combined in one migration so their schema is applied and rolled back as one unit.

Historical one-off scripts (`legacy/20260125_*.sql`, etc.) are archived in `legacy/` for reference; they are **not** executed by golang-migrate. Prefer the baseline + new versioned files going forward.

Shipments table is included for future use; shipping-service does not use Postgres at runtime today.

Migration `000010_payment_correlation_id` adds the nullable, indexed `payments.correlation_id` column used to preserve checkout correlation IDs through Stripe webhook publication.
