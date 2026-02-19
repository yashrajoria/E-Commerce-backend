output "s3_bucket" {
  value = aws_s3_bucket.app_bucket.id
}

output "dynamodb_products_table" {
  value = aws_dynamodb_table.products.name
}

output "sqs_order_processing_url" {
  value = aws_sqs_queue.order_processing.id
}

output "cloudwatch_log_group" {
  value = aws_cloudwatch_log_group.services.name
}
output "ci_role_arn" {
  value = aws_iam_role.github_oidc_role.arn
  description = "IAM role ARN for GitHub Actions OIDC"
}
output "region" {
  description = "AWS region used for deployment"
  value       = var.aws_region
}
