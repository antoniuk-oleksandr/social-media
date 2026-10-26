resource "aws_cloudwatch_log_group" "auth" {
  name              = "/aws/lambda/${local.auth_function}"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "auth" {
  function_name = local.auth_function
  role          = aws_iam_role.auth_lambda.arn
  runtime       = var.lambda_runtime
  architectures = [var.lambda_architecture]
  handler       = "bootstrap"

  filename         = var.lambda_package_path
  source_code_hash = filebase64sha256(var.lambda_package_path)

  memory_size = var.lambda_memory_mb
  timeout     = var.lambda_timeout_seconds

  environment {
    variables = {
      APP_ENV                     = var.environment
      JWT_KID                     = var.jwt_active_kid
      JWT_ISSUER                  = var.jwt_issuer
      JWT_AUDIENCE                = var.jwt_audience
      LOG_LEVEL                   = "info"
      IAM_POLICY_REVISION         = "2"
      JWT_ACCESS_SECRET_PARAMETER = aws_ssm_parameter.jwt_signing_key.name
      USERS_TABLE                 = aws_dynamodb_table.users.name
      UNIQUES_TABLE               = aws_dynamodb_table.uniques.name
      REFRESH_TABLE               = aws_dynamodb_table.refresh_sessions.name
    }
  }

  depends_on = [aws_iam_role_policy.auth_lambda]
}

resource "aws_cloudwatch_log_group" "users" {
  name              = "/aws/lambda/${local.users_function}"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "users" {
  function_name = local.users_function
  role          = aws_iam_role.users_lambda.arn
  runtime       = var.lambda_runtime
  architectures = [var.lambda_architecture]
  handler       = "bootstrap"

  filename         = var.users_lambda_package_path
  source_code_hash = filebase64sha256(var.users_lambda_package_path)

  memory_size = var.lambda_memory_mb
  timeout     = var.lambda_timeout_seconds

  environment {
    variables = {
      APP_ENV                     = var.environment
      JWT_KID                     = var.jwt_active_kid
      JWT_ISSUER                  = var.jwt_issuer
      JWT_AUDIENCE                = var.jwt_audience
      JWT_ACCESS_SECRET_PARAMETER = aws_ssm_parameter.jwt_signing_key.name
      USERS_TABLE                 = aws_dynamodb_table.users.name
      UNIQUES_TABLE               = aws_dynamodb_table.uniques.name
      LOG_LEVEL                   = "info"
    }
  }

  depends_on = [aws_iam_role_policy.users_lambda]
}

resource "aws_cloudwatch_log_group" "posts" {
  name              = "/aws/lambda/${local.posts_function}"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "posts" {
  function_name = local.posts_function
  role          = aws_iam_role.posts_lambda.arn
  runtime       = var.lambda_runtime
  architectures = [var.lambda_architecture]
  handler       = "bootstrap"

  filename         = var.posts_lambda_package_path
  source_code_hash = filebase64sha256(var.posts_lambda_package_path)
  memory_size      = var.lambda_memory_mb
  timeout          = var.lambda_timeout_seconds

  environment {
    variables = {
      APP_ENV                     = var.environment
      JWT_KID                     = var.jwt_active_kid
      JWT_ISSUER                  = var.jwt_issuer
      JWT_AUDIENCE                = var.jwt_audience
      JWT_ACCESS_SECRET_PARAMETER = aws_ssm_parameter.jwt_signing_key.name
      POSTS_TABLE                 = aws_dynamodb_table.posts.name
      LOG_LEVEL                   = "info"
    }
  }
  depends_on = [aws_iam_role_policy.posts_lambda]
}

resource "aws_cloudwatch_log_group" "ads" {
  name              = "/aws/lambda/${local.ads_function}"
  retention_in_days = var.log_retention_days
}

resource "aws_lambda_function" "ads" {
  function_name    = local.ads_function
  role             = aws_iam_role.ads_lambda.arn
  runtime          = var.lambda_runtime
  architectures    = [var.lambda_architecture]
  handler          = "bootstrap"
  filename         = var.ads_lambda_package_path
  source_code_hash = filebase64sha256(var.ads_lambda_package_path)
  memory_size      = var.lambda_memory_mb
  timeout          = var.lambda_timeout_seconds

  environment {
    variables = {
      APP_ENV                     = var.environment
      JWT_KID                     = var.jwt_active_kid
      JWT_ISSUER                  = var.jwt_issuer
      JWT_AUDIENCE                = var.jwt_audience
      JWT_ACCESS_SECRET_PARAMETER = aws_ssm_parameter.jwt_signing_key.name
      ADS_CURSOR_KEY_PARAMETER    = aws_ssm_parameter.ads_cursor_signing_key.name
      ADS_CURSOR_KEY_ID           = var.jwt_active_kid
      ADS_TABLE                   = aws_dynamodb_table.ads.name
      LOG_LEVEL                   = "info"
    }
  }
  depends_on = [aws_iam_role_policy.ads_lambda]
}
