# SQL migrations

Active migrations use [golang-migrate](https://github.com/golang-migrate/migrate) naming:

- `000001_baseline.up.sql` / `000001_baseline.down.sql`
- `000002_soft_delete_indexes.up.sql` / `000002_soft_delete_indexes.down.sql`

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

Unique constraints are enforced via **named unique indexes**, not inline `UNIQUE` column attrs, so a re-run still creates them if a prior AutoMigrate left the table without them.

Historical one-off scripts (`20260125_*.sql`, etc.) are kept for reference; they are **not** executed by golang-migrate. Prefer the baseline + new versioned files going forward.

Shipments table is included for future use; shipping-service does not use Postgres at runtime today.
