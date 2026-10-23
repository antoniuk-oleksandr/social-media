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
