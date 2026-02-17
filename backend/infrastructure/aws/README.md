# AWS infrastructure folder

This folder stores deployment-related scripts and infrastructure notes for the `aws-deployment` branch.

What goes here
- Place Terraform configurations or CloudFormation templates in this folder when you add real infra.
- Place deploy scripts such as `deploy_to_ec2.sh` which the GitHub Actions workflow can call.

How to use
- To deploy to EC2 (example): upload `deploy_to_ec2.sh` to your CI runner or have Actions SSH to an EC2 host and run it.
- To run migrations on AWS: connect to the host with access to the DB (or run via CI) and execute your SQL migration tool against the RDS endpoint.

Health checks
- After deploy, test service health endpoints (for example `/health` or `/ready`) by curling the public load-balancer or EC2 host.

Secrets and placeholders
- The CI workflows expect the following GitHub Secrets (placeholders):
  - `DOCKERHUB_USERNAME`
  - `DOCKERHUB_TOKEN`
  - `AWS_ACCESS_KEY_ID`
  - `AWS_SECRET_ACCESS_KEY`
  - `EC2_HOST`
  - `EC2_USER`
  - `EC2_SSH_KEY` (private key)

Executable scripts
- If you add scripts like `deploy_to_ec2.sh` or `run_migrations.sh`, make them executable before committing:

  ```bash
  git update-index --add --chmod=+x backend/infrastructure/aws/deploy_to_ec2.sh
  git update-index --add --chmod=+x backend/infrastructure/aws/run_migrations.sh
  git commit -m "Make infra scripts executable"
  ```

Terraform
- A minimal Terraform skeleton is in `backend/infrastructure/aws/terraform/`. Add resources (VPC, ECS/ECR, RDS) as needed.

Migration runner
- `run_migrations.sh` is a placeholder migration runner. Replace with your real migration tool and commands.

Dockerfiles
- Per-service `Dockerfile`s have been added to `backend/services/<service>/Dockerfile`.

CI / Secrets
- Add the GitHub Secrets listed above in your repository settings so `ci-aws.yml` can push images and (optionally) deploy to EC2.

