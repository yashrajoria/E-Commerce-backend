# Backup / Restore Runbook — ShopSwift Backend Postgres

Related: [architecture.md](./architecture.md) · [data-and-messaging.md](./data-and-messaging.md)

## What lives in Postgres

The compose `postgres` container (service name `postgres`, db `ecommerce`, user `postgres`) holds all relational state:

| Owner | Tables |
|-------|--------|
| identity-service | `users`, `refresh_tokens`, `addresses` |
| order-service | `orders`, `order_items`, `payments`, `stripe_processed_events`, `coupons`, `shipments`, `outbox_events`, `notification_events`, `payment_outbox_events` |
| notification-service | `notification_logs` |
| agent-service | `agent_audit_log` (same database — agent uses its own pool against `ecommerce`) |

Non-Postgres state is **not** covered by these scripts:

- **DynamoDB** (catalog, categories, inventory) — LocalStack volume / AWS table backup via `aws dynamodb export-table-to-point-in-time` or on-demand snapshot.
- **Redis** (cart state, checkout idempotency, rate limits, bulk-import queue) — ephemeral by design; carts/idempotency can be rebuilt.
- **S3** (product media) — versioning enabled at bucket level; not part of pg_dump.

## When to back up

1. **Before every migration** — `ALLOW_AUTO_MIGRATE` is true locally; take a dump first.
2. **Nightly** — schedule `./scripts/backup_postgres.sh` (cron / systemd timer on the host that has Docker access).
3. **Before restores / data fixes** — take a dump of the current state even if it is "broken"; you may need to roll back.

## Backup procedure

From `backend/` on a machine that can reach the compose stack:

```bash
./scripts/backup_postgres.sh
```

- Reads `POSTGRES_*` from `backend/.env` (falls back to `postgres` / `ecommerce`).
- Writes `backups/ecommerce_YYYYmmdd_HHMMSS.dump` (pg_dump custom format, compressed).
- Prints the path and size on success; exits non-zero on failure.
- `DRY_RUN=1 ./scripts/backup_postgres.sh` prints the command without executing.

Backups land in `backend/backups/` (gitignored — copies are not committed). Copy them off-box (S3, another host) for real DR; a dump on the same disk as the database is not a backup.

## Restore procedure

### Verify a backup into a scratch database (recommended first step)

```bash
TARGET_DB=shopswift_restore_test ./scripts/restore_postgres.sh backups/ecommerce_YYYYmmdd_HHMMSS.dump --yes
```

The script creates the scratch database, drops/recreates its `public` schema, runs `pg_restore`, then prints row counts for `users`, `orders`, `payments`, `coupons`, and `agent_audit_log`.

### Restore into the live database (destructive)

```bash
./scripts/restore_postgres.sh backups/ecommerce_YYYYmmdd_HHMMSS.dump
```

Prompts for confirmation unless `--yes` or `FORCE=1` is set. **This overwrites all data** in the target database.

### Verification checklist after any restore

1. Row counts look plausible (`users`, `orders`, `payments` non-zero on a seeded stack).
2. `./scripts/migrate.sh up` reports no pending migrations (schema matches code).
3. Gateway health: `curl -s http://localhost:8080/health` → `200`.
4. Spot-check an order via admin API or psql: `SELECT id, status FROM orders ORDER BY created_at DESC LIMIT 5;`
5. If `agent_audit_log` was non-zero before the backup, confirm it is non-zero after.

## DR notes

- The pg_dump custom format is version-agnostic within major Postgres releases; the stack pins `postgres:16.4-alpine` in `docker-compose.yml` — do not restore a 16.x dump into a future 17+ major without testing in the scratch database first.
- Compose volume `postgres_data` is the live data directory; restoring via `pg_restore` into a running container is the supported path (no need to stop the stack).
- If the container itself is lost, start a fresh stack (`./scripts/dev-up.sh`), run migrations, then restore into the new database with the live-database procedure above.
