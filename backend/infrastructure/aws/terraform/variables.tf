variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "s3_bucket" {
  description = "S3 bucket name for application assets"
  type        = string
  default     = "shopswift"
}

variable "ddb_tables" {
  description = "DynamoDB table names"
  type = map(string)
  default = {
    products   = "Products"
    categories = "Categories"
    inventory  = "Inventory"
  }
}

variable "sqs_queues" {
  description = "SQS queue names"
  type = map(string)
  default = {
    order_processing = "order-processing-queue"
    payment_events   = "payment-events-queue"
    payment_request  = "payment-request-queue"
  }
}

variable "project_name" {
  description = "Project name"
  type        = string
  default     = "e-commerce-backend"
}

variable "github_repo" {
  description = "GitHub repository in OWNER/REPO format used for OIDC trust condition"
  type        = string
  default     = "yashrajoria/E-Commerce-backend"
}

variable "ci_role_name" {
  description = "IAM role name for GitHub Actions OIDC"
  type        = string
  default     = "ecommerce-github-actions-oidc-role"
}
