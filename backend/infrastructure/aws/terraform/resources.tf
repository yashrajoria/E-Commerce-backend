resource "aws_s3_bucket" "app_bucket" {
  bucket = var.s3_bucket
  force_destroy = true
}

resource "aws_s3_bucket_acl" "app_bucket_acl" {
  bucket = aws_s3_bucket.app_bucket.id
  acl    = "private"
}

resource "aws_dynamodb_table" "products" {
  name         = var.ddb_tables["products"]
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }
  attribute {
    name = "sku"
    type = "S"
  }
  attribute {
    name = "is_featured"
    type = "S"
  }
  attribute {
    name = "created_at"
    type = "S"
  }

  global_secondary_index {
    name            = "sku-index"
    hash_key        = "sku"
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "featured-index"
    hash_key        = "is_featured"
    range_key       = "created_at"
    projection_type = "ALL"
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
  attribute {
    name = "name"
    type = "S"
  }

  global_secondary_index {
    name            = "name-index"
    hash_key        = "name"
    projection_type = "ALL"
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

resource "aws_dynamodb_table" "product_categories" {
  name         = var.ddb_tables["product_categories"]
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "category_id"
  range_key    = "product_id"

  attribute {
    name = "category_id"
    type = "S"
  }
  attribute {
    name = "product_id"
    type = "S"
  }

  global_secondary_index {
    name            = "product-index"
    hash_key        = "product_id"
    range_key       = "category_id"
    projection_type = "ALL"
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
