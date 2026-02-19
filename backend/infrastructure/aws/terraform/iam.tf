// IAM resources for GitHub Actions OIDC trust and CI policy
data "aws_caller_identity" "me" {}

locals {
  account_id = data.aws_caller_identity.me.account_id
  region     = var.aws_region
}

data "aws_iam_policy_document" "github_oidc_trust" {
  statement {
    effect = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = ["arn:aws:iam::${local.account_id}:oidc-provider/token.actions.githubusercontent.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_repo}:*"]
    }
  }
}

resource "aws_iam_role" "github_oidc_role" {
  name               = var.ci_role_name
  assume_role_policy = data.aws_iam_policy_document.github_oidc_trust.json
  description        = "OIDC role for GitHub Actions to run CI (terraform, deploy)"
}

data "aws_iam_policy_document" "ci_policy" {
  statement {
    sid    = "S3Access"
    effect = "Allow"
    actions = [
      "s3:ListBucket",
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject"
    ]
    resources = [
      "arn:aws:s3:::${var.s3_bucket}",
      "arn:aws:s3:::${var.s3_bucket}/*"
    ]
  }

  statement {
    sid    = "DynamoDBAccess"
    effect = "Allow"
    actions = [
      "dynamodb:DescribeTable",
      "dynamodb:CreateTable",
      "dynamodb:PutItem",
      "dynamodb:GetItem",
      "dynamodb:Query",
      "dynamodb:Scan",
      "dynamodb:UpdateItem"
    ]
    resources = [
      for tbl in values(var.ddb_tables) : "arn:aws:dynamodb:${local.region}:${local.account_id}:table/${tbl}"
    ]
  }

  statement {
    sid    = "SQSAccess"
    effect = "Allow"
    actions = [
      "sqs:CreateQueue",
      "sqs:SendMessage",
      "sqs:ReceiveMessage",
      "sqs:GetQueueUrl",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes"
    ]
    resources = [
      for q in values(var.sqs_queues) : "arn:aws:sqs:${local.region}:${local.account_id}:${q}"
    ]
  }

  statement {
    sid    = "CloudWatchLogs"
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:CreateLogStream",
      "logs:PutLogEvents"
    ]
    resources = ["arn:aws:logs:${local.region}:${local.account_id}:log-group:/ecommerce/services*"]
  }

  statement {
    sid    = "SecretsManager"
    effect = "Allow"
    actions = ["secretsmanager:GetSecretValue","secretsmanager:CreateSecret","secretsmanager:DescribeSecret"]
    resources = ["arn:aws:secretsmanager:${local.region}:${local.account_id}:secret:*"]
  }
}

resource "aws_iam_policy" "ci_policy" {
  name        = "${var.project_name}-ci-policy"
  description = "Least-privilege policy for CI (GitHub Actions)"
  policy      = data.aws_iam_policy_document.ci_policy.json
}

resource "aws_iam_role_policy_attachment" "ci_attach" {
  role       = aws_iam_role.github_oidc_role.name
  policy_arn = aws_iam_policy.ci_policy.arn
}

output "ci_role_arn" {
  value = aws_iam_role.github_oidc_role.arn
}
