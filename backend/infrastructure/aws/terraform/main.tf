// Terraform skeleton for AWS (placeholders) - review before use
terraform {
  required_version = ">= 1.0"
}

provider "aws" {
  region = var.aws_region
  # Credentials are expected to be provided via CI/GitHub Secrets or environment
}

# Example: an ECS cluster, ECR repo, or RDS instance can be added here.
# Add real resources when ready.
