# AWS infrastructure folder

Deployment scripts and Terraform for AWS. Local development should use **LocalStack** via:

```bash
cd backend && ./scripts/dev-up.sh
# docker compose -f docker-compose.yml -f docker-compose.localstack.yml up -d
```

Do not point `USE_LOCALSTACK=true` services at `http://localstack:4566` without the LocalStack override file — DNS will fail.

## Contents

- Terraform skeleton: `terraform/`
- Deploy helpers / OIDC role notes: see `ROLE_SETUP.md`, `terraform/README.md`
- Migration runner: `run_migrations.sh` (golang-migrate; requires `DB_HOST`, `DB_USER`, `DB_PASS`)

## Migrations on AWS

```bash
DB_HOST=<rds-endpoint> DB_USER=... DB_PASS=... DB_NAME=ecommerce \
  ./run_migrations.sh up
```

Set `ALLOW_AUTO_MIGRATE=false` on deployed services so schema comes only from SQL.

## Health checks

After deploy, curl:

- `GET /health` or `/health/live` — process up
- `GET /health/ready` — dependencies (where implemented)

Gateway exposes `/health` for load balancer / Compose checks.

## Secrets (GitHub)

- `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` or OIDC (`AWS_ROLE_TO_ASSUME`)
- `EC2_HOST`, `EC2_USER`, `EC2_SSH_KEY` (if using EC2 deploy)

## Local vs AWS

| Concern | Local | AWS |
|---------|-------|-----|
| S3/DDB/SNS/SQS | LocalStack `:4566` | Real AWS |
| Postgres | Compose `postgres` | RDS |
| Stripe webhooks | `stripe-cli` container | Stripe → public URL / ALB |
