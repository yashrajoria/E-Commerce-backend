resource "aws_s3_bucket" "app_bucket" {
  bucket = var.s3_bucket
  acl    = "private"
  force_destroy = true
}

resource "aws_dynamodb_table" "products" {
  name         = var.ddb_tables["products"]
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"
  attribute {
    name = "id"
    type = "S"
  }
}

resource "aws_dynamodb_table" "categories" {
  name         = var.ddb_tables["categories"]
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"
  attribute {
    name = "id"
    type = "S"
  }
}

resource "aws_dynamodb_table" "inventory" {
  name         = var.ddb_tables["inventory"]
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"
  attribute {
    name = "id"
    type = "S"
  }
}

resource "aws_sqs_queue" "order_processing" {
  name = var.sqs_queues["order_processing"]
}

resource "aws_sqs_queue" "payment_events" {
  name = var.sqs_queues["payment_events"]
}

resource "aws_sqs_queue" "payment_request" {
  name = var.sqs_queues["payment_request"]
}

resource "aws_cloudwatch_log_group" "services" {
  name              = "/ecommerce/services"
  retention_in_days = 30
}

resource "aws_secretsmanager_secret" "db_credentials" {
  name = "ecommerce/db_credentials"
}
