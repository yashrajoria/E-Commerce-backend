# Load Test Toolkit — ShopSwift Backend

Load tests the **local** stack through the API gateway. Never point these at production.

## Prerequisites

1. Stack running locally:
   ```bash
   cd backend && ./scripts/dev-up.sh
   ```
2. `k6` installed:
   ```bash
   brew install k6        # macOS
   # or https://k6.io/docs/get-started/installation/
   ```
3. Optional (for the authenticated scenario): a seeded demo user. See
   `scripts/seed_postgres_data.sh` — demo password is `Demo123!` for the
   seeded `*.shopswift-demo.test` emails. Export:
   ```bash
   export LOADTEST_EMAIL=alice.johnson@shopswift-demo.test
   export LOADTEST_PASSWORD=Demo123!
   ```

## Run

```bash
cd backend/tools/loadtest
./run-loadtest.sh
```

Or directly:

```bash
k6 run --env GATEWAY_URL=http://localhost:8080 k6-loadtest.js
```

If `LOADTEST_EMAIL` / `LOADTEST_PASSWORD` are unset, the login scenario is
skipped (logged as a warning) instead of failing the run.

## What it hits (gateway :8080)

| Scenario | Route | Method | Auth |
|----------|-------|--------|------|
| health   | `/health`        | GET  | none |
| products | `/products`      | GET  | none |
| login    | `/auth/login`    | POST | demo credentials (env vars) |

Thresholds (tuned after first baseline run — see `baseline-notes.md`):

- `http_req_failed < 1%` overall
- health p95 < 500 ms
- products p95 < 1500 ms
- login p95 < 2000 ms

## Safety

- **Local stack only.** The script defaults to `http://localhost:8080`.
- Stages ramp to a modest steady VUs (see `k6-loadtest.js`) — raise them only
  after recording a baseline on your machine.
- The login scenario uses real credentials against the local seed; do not
  export production credentials into this environment.
- Stop the stack when done: `docker compose -f docker-compose.yml -f docker-compose.localstack.yml down`.

## Recording results

Fill in `baseline-notes.md` after each run. Keep one row per scenario per machine
so regressions are visible across commits.
