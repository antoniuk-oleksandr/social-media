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
      "dynamodb:ConditionCheckItem",
      "dynamodb:GetItem",
      "dynamodb:PutItem",
      "dynamodb:Query",
      "dynamodb:TransactWriteItems"
    ]
    resources = [
      aws_dynamodb_table.users.arn,
      "${aws_dynamodb_table.users.arn}/index/*",
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

data "aws_iam_openid_connect_provider" "github" {
  url = "https://token.actions.githubusercontent.com"
}

data "aws_iam_policy_document" "github_actions_assume_role" {
  statement {
    effect = "Allow"

    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.github.arn]
    }

    actions = ["sts:AssumeRoleWithWebIdentity"]

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values = [
        "repo:${split("/", var.github_repository)[0]}@${var.github_repository_owner_id}/${split("/", var.github_repository)[1]}@${var.github_repository_id}:environment:${var.github_environment}",
        "repo:${split("/", var.github_repository)[0]}@${var.github_repository_owner_id}/${split("/", var.github_repository)[1]}@${var.github_repository_id}:ref:refs/heads/${var.github_environment}",
      ]
    }
  }
}

resource "aws_iam_role" "github_actions_deploy" {
  name               = var.github_actions_role_name
  assume_role_policy = data.aws_iam_policy_document.github_actions_assume_role.json
}

data "aws_iam_policy_document" "github_actions_deploy" {
  statement {
    sid       = "UseTerraformState"
    effect    = "Allow"
    actions   = ["s3:ListBucket"]
    resources = ["arn:aws:s3:::${var.terraform_state_bucket}"]
  }

  statement {
    sid    = "ReadWriteTerraformState"
    effect = "Allow"
    actions = [
      "s3:DeleteObject",
      "s3:GetObject",
      "s3:PutObject"
    ]
    resources = ["arn:aws:s3:::${var.terraform_state_bucket}/*"]
  }

  statement {
    sid    = "DeployAuthLambda"
    effect = "Allow"
    actions = [
      "lambda:GetFunction",
      "lambda:GetFunctionConfiguration",
      "lambda:GetFunctionCodeSigningConfig",
      "lambda:ListVersionsByFunction",
      "lambda:UpdateFunctionCode",
      "lambda:UpdateFunctionConfiguration"
    ]
    resources = [
      "arn:aws:lambda:${var.aws_region}:${data.aws_caller_identity.current.account_id}:function:${local.auth_function}"
    ]
  }

  statement {
    sid       = "PassAuthLambdaRole"
    effect    = "Allow"
    actions   = ["iam:PassRole"]
    resources = [aws_iam_role.auth_lambda.arn]

    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values   = ["lambda.amazonaws.com"]
    }
  }

  statement {
    sid       = "ReadSigningKeyParameter"
    effect    = "Allow"
    actions   = ["ssm:GetParameter"]
    resources = [aws_ssm_parameter.jwt_signing_key.arn]
  }
}

resource "aws_iam_role_policy" "github_actions_deploy" {
  name   = "${var.github_actions_role_name}-policy"
  role   = aws_iam_role.github_actions_deploy.id
  policy = data.aws_iam_policy_document.github_actions_deploy.json
}
