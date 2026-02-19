Development — Local vs AWS

This document explains how to run the backend locally (against real AWS resources)
and how the previous local development workflow was migrated to use AWS.

Local development (recommended)

- Run services locally but target real AWS (recommended for environment parity).
- Copy the AWS-aware env template and start compose:

```bash
cp .env.local .env
docker compose -f docker-compose.yml up --build
```

Note: The repository previously included a compose override
(for example a local override file) and optional init scripts under a
local `infrastructure/` folder. Those were removed to simplify testing
against real AWS. If you need an offline/local testing setup, reintroduce
local tooling and any init scripts with caution.

Development against real AWS (staging / integration)

- Use real AWS for integration testing and staging to validate IAM, service behavior, and production parity.
- Do not put long-lived credentials in `.env`. Use one of:
  - IAM role on the host (EC2/ECS) or instance profile.
  - CI provider OIDC (GitHub Actions) to assume a role.
  - Short-lived credentials stored in your CI secrets.

- Steps to run with real AWS resources:

1. Ensure your environment has credentials (e.g., `aws configure`, environment variables, or instance role).

2. Create AWS resources (recommended via Terraform):

```bash
cd infrastructure/aws/terraform
terraform init
terraform plan -out plan.tfplan
terraform apply plan.tfplan
```

3. Update `.env` (or CI secrets) with the real resource names/ARNS/URLs if needed (e.g., SQS URLs, S3 bucket names). Ensure `AWS_ENDPOINT` is empty so the SDK talks to AWS.

4. Start services:

```bash
# ensure .env has AWS credentials or environment provides role
docker compose -f docker-compose.yml up --build -d
```

CI integration notes

- We added `.github/workflows/terraform.yml` to plan Terraform on PRs and apply on `main` when the workflow has permissions. The workflow assumes an OIDC role via `secrets.AWS_ROLE_TO_ASSUME`.

Security notes

- `.env.local` and `terraform.tfvars` are ignored by git (`.gitignore`). Do NOT commit credentials. Rotate any secrets used for testing.
