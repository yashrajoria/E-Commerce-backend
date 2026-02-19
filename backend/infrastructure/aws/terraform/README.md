Terraform for AWS resources used by E-Commerce backend

Overview

This folder contains Terraform configs for provisioning AWS resources used by the backend.

Quick steps (local):

1.  Review `terraform.tfvars.example` and copy values into `terraform.tfvars` (do NOT commit this file).

2.  Run the included helper to apply infrastructure:

```bash
cd infrastructure/aws
./deploy_infrastructure.sh apply
```

The script runs `terraform init` and `terraform apply` then writes outputs to `infrastructure/aws/terraform-outputs.json`.

Seeding data:

After applying, seed initial categories and simple data with:

```bash
AWS_REGION=us-east-1 ./seed_data.sh
```

CI notes:

- The repository contains `.github/workflows/terraform.yml` which performs `terraform plan` on PRs and `terraform apply` on `main`. That workflow uses OIDC to assume an IAM role—set `secrets.AWS_ROLE_TO_ASSUME` in the repository settings.
- Ensure these GitHub secrets are present: `AWS_ROLE_TO_ASSUME`, `DOCKERHUB_TOKEN`, `DOCKERHUB_USERNAME`, and any SSH keys used for EC2 deployment.

Security:

- Do not commit long-lived credentials. Use short-lived OIDC roles in CI or instance profiles on hosts.

Quick start (local)

1. Copy example vars:
   cp terraform.tfvars.example terraform.tfvars
2. Init and plan:

```bash
cd infrastructure/aws/terraform
terraform init
terraform plan -out plan.tfplan
```

3. Apply (be careful — will create real AWS resources):

```bash
terraform apply plan.tfplan
```

CI notes

- Use `aws-actions/configure-aws-credentials` with OIDC (recommended) or store short-lived credentials in CI secrets.
- Ensure `terraform init` runs with the working directory set to `infrastructure/aws/terraform`.

Cleanup

- To destroy resources:

```bash
terraform destroy
```

Security

- Do NOT commit `terraform.tfvars` with real credentials. Use CI secrets or AWS roles.
