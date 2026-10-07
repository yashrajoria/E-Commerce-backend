resource "aws_s3_bucket" "app_bucket" {
  bucket        = var.s3_bucket
  force_destroy = true
}

resource "aws_s3_bucket_acl" "app_bucket_acl" {
  bucket = aws_s3_bucket.app_bucket.id
  acl    = "private"
}

resource "aws_sqs_queue" "order_processing_dlq" {
  name = var.sqs_dlqs["order_processing"]
}

resource "aws_sqs_queue" "payment_events_dlq" {
  name = var.sqs_dlqs["payment_events"]
}

resource "aws_sqs_queue" "payment_request_dlq" {
  name = var.sqs_dlqs["payment_request"]
}

resource "aws_sqs_queue" "notification_dlq" {
  name = var.sqs_dlqs["notification"]
}

resource "aws_sqs_queue" "promotion_order_dlq" {
  name = var.sqs_dlqs["promotion_order"]
}

resource "aws_sqs_queue" "order_processing" {
  name                       = var.sqs_queues["order_processing"]
  visibility_timeout_seconds = 30
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.order_processing_dlq.arn
    maxReceiveCount     = var.sqs_max_receive_counts["order_processing"]
  })
}

resource "aws_sqs_queue" "payment_events" {
  name                       = var.sqs_queues["payment_events"]
  visibility_timeout_seconds = 30
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.payment_events_dlq.arn
    maxReceiveCount     = var.sqs_max_receive_counts["payment_events"]
  })
}

resource "aws_sqs_queue" "payment_request" {
  name                       = var.sqs_queues["payment_request"]
  visibility_timeout_seconds = 30
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.payment_request_dlq.arn
    maxReceiveCount     = var.sqs_max_receive_counts["payment_request"]
  })
}

resource "aws_sqs_queue" "notification" {
  name                       = var.sqs_queues["notification"]
  visibility_timeout_seconds = 30
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.notification_dlq.arn
    maxReceiveCount     = var.sqs_max_receive_counts["notification"]
  })
}

resource "aws_sqs_queue" "promotion_order" {
  name                       = var.sqs_queues["promotion_order"]
  visibility_timeout_seconds = 30
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.promotion_order_dlq.arn
    maxReceiveCount     = var.sqs_max_receive_counts["promotion_order"]
  })
}

resource "aws_cloudwatch_log_group" "services" {
  name              = "/ecommerce/services"
  retention_in_days = 30
}

resource "aws_secretsmanager_secret" "db_credentials" {
  name = "ecommerce/db_credentials"
}
