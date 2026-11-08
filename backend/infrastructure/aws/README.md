# AWS infrastructure folder

Deployment scripts and Terraform for AWS. Local development uses the backend Compose stack (Postgres + Redis; no LocalStack):

```bash
cd backend && docker compose up -d --build
```

Application queues are Postgres-backed (`message_queues` table, migration `000017`; see `pkg/common/messaging`), and no Go service imports the AWS SDK. The SQS queues, S3 bucket and other resources provisioned by `terraform/` are therefore not required by the running services. `backend/scripts/dev-up.sh` still points at a `docker-compose.localstack.yml` that is not in the repo.

## Contents

| File | Purpose |
|------|---------|
| `terraform/` | Terraform: S3 bucket, 5 SQS queues + DLQs, CloudWatch log group, Secrets Manager secret, GitHub OIDC role. See `terraform/README.md` |
| `deploy_infrastructure.sh` | `./deploy_infrastructure.sh [plan\|apply]` (default `apply`): `terraform init`, then `plan -out=plan.tfplan` or `apply -auto-approve`; apply writes `terraform output -json` to `infrastructure/aws/terraform-outputs.json` |
| `deploy_to_ec2.sh` | Runs on the EC2 host. Requires `DOCKERHUB_USERNAME`, `GIT_SHA`, `DEPLOY_DIR`; installs Docker/Compose if missing (`dnf`), logs into Docker Hub (`DOCKERHUB_TOKEN`), then `docker compose -f docker-compose.yml -f docker-compose.deploy.yml pull && up -d --remove-orphans`. Refuses to run without the deploy override (SHA-pinned images) |
| `run_migrations.sh` | golang-migrate runner for RDS/remote Postgres; wraps `backend/scripts/migrate.sh` |
| `populate_env_from_terraform.sh` | Copies Terraform outputs (`AWS_S3_BUCKET`, `S3_BUCKET_IMAGES`, `AWS_REGION`, `CLOUDWATCH_LOG_GROUP`, `ORDER_SQS_URL`) into `.env`, with a timestamped backup; needs `jq`. Caveat: it reads `infrastructure/terraform/terraform-outputs.json` but `deploy_infrastructure.sh` writes `infrastructure/aws/terraform-outputs.json`, and it targets `.env` two levels above `infrastructure/` |
| `install-docker.sh` | Installs Docker + Compose on Ubuntu (`apt-get`) |
| `install-docker-amazon-linux.sh` | Installs Docker + Compose on Amazon Linux (`yum`) |
| `ROLE_SETUP.md`, `trust.json`, `tf-policy.json` | GitHub Actions OIDC role creation (replace `ACCOUNT_ID`, `OWNER/REPO` placeholders) |
| `terraform-outputs.json` | Output of the last apply (currently `{}`) |

`../postgres/catalog_role.sql` creates the least-privilege `catalog_svc` role for catalog-service (run once per environment as the admin role, after migrations `000015+`).

## Migrations on AWS

```bash
DB_HOST=<rds-endpoint> DB_USER=... DB_PASS=... DB_NAME=ecommerce \
  ./run_migrations.sh up
```

`DB_HOST` is required; defaults: `DB_USER=postgres`, `DB_NAME=ecommerce`, `DB_PORT=5432`, `DB_SSLMODE=require`. The first argument is the migrate action (default `up`).

Set `ALLOW_AUTO_MIGRATE=false` on deployed services so schema comes only from SQL.

## Health checks

After deploy, curl:

- `GET /health` or `/health/live` - process up
- `GET /health/ready` - dependencies (where implemented; the gateway checks Redis)

Gateway exposes `/health` for load balancer / Compose checks.

## CI/CD

`E-Commerce-backend/.github/workflows/ci-main.yml` runs Go vet/build/test per module, migration apply/rollback/drift checks, agent-service pytest, Spectral OpenAPI lint, and Docker build checks; on main it publishes an image to GitHub Container Registry and triggers a Render deploy hook (`RENDER_DEPLOY_HOOK_URL`). `update-commits-badge.yml` sits next to it. There is no Terraform workflow in the repo.

## Secrets (GitHub)

- `RENDER_DEPLOY_HOOK_URL` (used by `ci-main.yml`)
- For the EC2 path (`deploy_to_ec2.sh`): `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`, and your SSH host/key secrets
- For Terraform/OIDC: `AWS_ROLE_TO_ASSUME` (role ARN from `terraform output ci_role_arn`)

## Local vs AWS

| Concern | Local | AWS |
|---------|-------|-----|
| Queues | Postgres `message_queues` | Same (Terraform SQS not wired to services) |
| Postgres | Compose `postgres` | RDS / Supabase |
| Redis | Compose `redis` | Your Redis |
| Stripe webhooks | `stripe-cli` container | Stripe -> public URL / ALB |
