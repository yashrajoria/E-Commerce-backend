// Terraform skeleton for AWS (placeholders) - review before use
terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 4.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
  # Credentials are expected to be provided via CI/GitHub Secrets or environment
}

# Resources are defined in resources.tf
