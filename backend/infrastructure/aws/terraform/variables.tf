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

  # EC2 instance variables
  variable "ec2_ami" {
    description = "AMI ID for EC2 instance"
    type        = string
    default     = "ami-0c02fb55956c7d316" # Amazon Linux 2
  }

  variable "ec2_instance_type" {
    description = "EC2 instance type"
    type        = string
    default     = "t3.micro"
  }

  # DynamoDB table name
  variable "dynamodb_table_name" {
    description = "DynamoDB table name"
    type        = string
    default     = "ECommerceDynamoDB"
  }

  # SNS topic name
  variable "sns_topic_name" {
    description = "SNS topic name"
    type        = string
    default     = "ECommerceSNSTopic"
  }

  # SQS queue name
  variable "sqs_queue_name" {
    description = "SQS queue name"
    type        = string
    default     = "ECommerceSQSQueue"
  }

  # CloudWatch log group name
  variable "cloudwatch_log_group_name" {
    description = "CloudWatch log group name"
    type        = string
    default     = "/aws/ecommerce/app"
  }
