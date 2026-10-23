data "aws_iam_policy_document" "lambda_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }

    actions = ["sts:AssumeRole"]
  }
}

resource "aws_iam_role" "auth_lambda" {
  name               = "${local.auth_function}-execution"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume_role.json
}

data "aws_iam_policy_document" "auth_lambda" {
  statement {
    sid       = "WriteApplicationLogs"
    effect    = "Allow"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.auth.arn}:*"]
  }

  statement {
    sid    = "UseAuthTables"
    effect = "Allow"
    actions = [
      "dynamodb:DescribeTable",
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:TransactWriteItems"
    ]
    resources = [
      aws_dynamodb_table.users.arn,
      aws_dynamodb_table.uniques.arn,
      aws_dynamodb_table.refresh_sessions.arn
    ]
  }

  statement {
    sid       = "ReadJWTSigningKey"
    effect    = "Allow"
    actions   = ["ssm:GetParameter"]
    resources = [aws_ssm_parameter.jwt_signing_key.arn]
  }
}

resource "aws_iam_role_policy" "auth_lambda" {
  name   = "${local.auth_function}-execution"
  role   = aws_iam_role.auth_lambda.id
  policy = data.aws_iam_policy_document.auth_lambda.json
}
