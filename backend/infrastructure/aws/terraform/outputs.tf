output "s3_bucket" {
  value = aws_s3_bucket.app_bucket.id
}

output "dynamodb_products_table" {
  value = aws_dynamodb_table.products.name
}

output "sqs_order_processing_url" {
  value = aws_sqs_queue.order_processing.id
}

output "sqs_queue_urls" {
  description = "Source SQS queue URLs keyed by workflow"
  value = {
    order_processing = aws_sqs_queue.order_processing.id
    payment_events   = aws_sqs_queue.payment_events.id
    payment_request  = aws_sqs_queue.payment_request.id
    notification     = aws_sqs_queue.notification.id
    promotion_order  = aws_sqs_queue.promotion_order.id
  }
}

output "sqs_dlq_urls" {
  description = "SQS dead-letter queue URLs keyed by workflow"
  value = {
    order_processing = aws_sqs_queue.order_processing_dlq.id
    payment_events   = aws_sqs_queue.payment_events_dlq.id
    payment_request  = aws_sqs_queue.payment_request_dlq.id
    notification     = aws_sqs_queue.notification_dlq.id
    promotion_order  = aws_sqs_queue.promotion_order_dlq.id
  }
}

output "cloudwatch_log_group" {
  value = aws_cloudwatch_log_group.services.name
}
output "region" {
  description = "AWS region used for deployment"
  value       = var.aws_region
}
