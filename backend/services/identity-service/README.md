# identity-service (port `8081`)

Merged from `auth-service` + `user-service`. Owns everything about "who is this
user": credentials, JWT issuance/refresh/revocation, email verification, admin
bootstrap, profiles, and addresses. Single Postgres connection, single `User`
model, single migration over the shared `users` table.

## Routes (unchanged from the pre-merge services)

- `POST /auth/register|login|verify-email|resend-verification|logout|refresh`
- `GET /auth/status` (gateway mesh token required)
- `POST /auth/admin/users` (admin)
- `GET|PUT /users/profile`, `POST /users/change-password`
- `GET /users` (admin, paginated)

## What the merge deleted

- `user-service → auth-service` HTTP hop (`AUTH_SERVICE_URL`,
  `POST /auth/internal/revoke-tokens`). Password changes now revoke refresh
  tokens via a direct in-process call.
- Dual AutoMigrate of the `users` table; `database.Connect` migrates
  `User` + `RefreshToken` + `Address` once.
- Dead config: `SMTP_*` (never read), `types` package (unused).

## Env

`PORT` (default `8081`), `POSTGRES_*`, `JWT_SECRET` (required),
`ADMIN_EMAIL`/`ADMIN_PASSWORD`/`ADMIN_NAME` (first-admin bootstrap),
`INTERNAL_SERVICE_TOKEN`, `ALLOW_AUTO_MIGRATE`, SNS/LocalStack vars.
