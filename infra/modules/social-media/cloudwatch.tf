locals {
  observability_lambda_functions = {
    auth  = aws_lambda_function.auth.function_name
    users = aws_lambda_function.users.function_name
    posts = aws_lambda_function.posts.function_name
    ads   = aws_lambda_function.ads.function_name
    feed  = aws_lambda_function.feed.function_name
  }

  observability_lambda_log_groups = {
    auth  = aws_cloudwatch_log_group.auth.name
    users = aws_cloudwatch_log_group.users.name
    posts = aws_cloudwatch_log_group.posts.name
    ads   = aws_cloudwatch_log_group.ads.name
    feed  = aws_cloudwatch_log_group.feed.name
  }

  observability_tables = {
    users            = aws_dynamodb_table.users.name
    uniques          = aws_dynamodb_table.uniques.name
    refresh_sessions = aws_dynamodb_table.refresh_sessions.name
    posts            = aws_dynamodb_table.posts.name
    ads              = aws_dynamodb_table.ads.name
  }
}

resource "aws_cloudwatch_log_metric_filter" "application_errors" {
  for_each       = local.observability_lambda_log_groups
  name           = "${local.name_prefix}-${each.key}-application-errors"
  log_group_name = each.value
  pattern        = "{ $.level = \"error\" }"

  metric_transformation {
    name          = "${each.key}ApplicationErrors"
    namespace     = var.observability_namespace
    value         = "1"
  }
}

resource "aws_cloudwatch_log_metric_filter" "dependency_failures" {
  for_each       = local.observability_lambda_log_groups
  name           = "${local.name_prefix}-${each.key}-dependency-failures"
  log_group_name = each.value
  pattern        = "{ $.dependency_operation = * }"

  metric_transformation {
    name          = "${each.key}DependencyFailures"
    namespace     = var.observability_namespace
    value         = "1"
  }
}

resource "aws_cloudwatch_metric_alarm" "lambda_errors" {
  for_each            = local.observability_lambda_functions
  alarm_name          = "${local.name_prefix}-${each.key}-lambda-errors"
  alarm_description   = "Lambda errors for ${each.value}"
  namespace           = "AWS/Lambda"
  metric_name         = "Errors"
  dimensions          = { FunctionName = each.value }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "lambda_throttles" {
  for_each            = local.observability_lambda_functions
  alarm_name          = "${local.name_prefix}-${each.key}-lambda-throttles"
  alarm_description   = "Lambda throttles for ${each.value}"
  namespace           = "AWS/Lambda"
  metric_name         = "Throttles"
  dimensions          = { FunctionName = each.value }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "lambda_duration" {
  for_each            = local.observability_lambda_functions
  alarm_name          = "${local.name_prefix}-${each.key}-lambda-duration"
  alarm_description   = "High Lambda duration for ${each.value}"
  namespace           = "AWS/Lambda"
  metric_name         = "Duration"
  dimensions          = { FunctionName = each.value }
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 5000
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "api_gateway_5xx" {
  alarm_name          = "${local.name_prefix}-api-5xx"
  alarm_description   = "API Gateway returned 5xx responses"
  namespace           = "AWS/ApiGateway"
  metric_name         = "5XXError"
  dimensions          = { ApiId = aws_apigatewayv2_api.http.id, Stage = aws_apigatewayv2_stage.default.name }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "api_gateway_4xx" {
  alarm_name          = "${local.name_prefix}-api-4xx"
  alarm_description   = "Elevated API Gateway 4xx responses"
  namespace           = "AWS/ApiGateway"
  metric_name         = "4XXError"
  dimensions          = { ApiId = aws_apigatewayv2_api.http.id, Stage = aws_apigatewayv2_stage.default.name }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 10
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "api_gateway_latency" {
  alarm_name          = "${local.name_prefix}-api-latency"
  alarm_description   = "High API Gateway latency"
  namespace           = "AWS/ApiGateway"
  metric_name         = "Latency"
  dimensions          = { ApiId = aws_apigatewayv2_api.http.id, Stage = aws_apigatewayv2_stage.default.name }
  extended_statistic  = "p99"
  period              = 300
  evaluation_periods  = 1
  threshold           = 5000
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "dynamodb_throttles" {
  for_each            = local.observability_tables
  alarm_name          = "${local.name_prefix}-${each.key}-dynamodb-throttles"
  alarm_description   = "DynamoDB throttling for ${each.value}"
  namespace           = "AWS/DynamoDB"
  metric_name         = "ThrottledRequests"
  dimensions          = { TableName = each.value }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "dynamodb_system_errors" {
  for_each            = local.observability_tables
  alarm_name          = "${local.name_prefix}-${each.key}-dynamodb-system-errors"
  alarm_description   = "DynamoDB system errors for ${each.value}"
  namespace           = "AWS/DynamoDB"
  metric_name         = "SystemErrors"
  dimensions          = { TableName = each.value }
  statistic           = "Sum"
  period              = 300
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"
  alarm_actions       = var.alarm_actions
}

resource "aws_cloudwatch_dashboard" "main" {
  dashboard_name = "${local.name_prefix}-observability"
  dashboard_body = jsonencode({
    widgets = [
      {
        type   = "metric"
        width  = 24
        height = 6
        properties = {
          title  = "Lambda errors, throttles, and duration"
          region = var.aws_region
          view   = "timeSeries"
          stat   = "Sum"
          period = 300
          metrics = concat(
            [for name in values(local.observability_lambda_functions) : ["AWS/Lambda", "Errors", "FunctionName", name]],
            [for name in values(local.observability_lambda_functions) : ["AWS/Lambda", "Throttles", "FunctionName", name]],
          )
        }
      },
      {
        type   = "metric"
        width  = 24
        height = 6
        properties = {
          title  = "API Gateway failures and latency"
          region = var.aws_region
          view   = "timeSeries"
          period = 300
          metrics = [
            ["AWS/ApiGateway", "5XXError", "ApiId", aws_apigatewayv2_api.http.id, "Stage", aws_apigatewayv2_stage.default.name],
            ["AWS/ApiGateway", "4XXError", "ApiId", aws_apigatewayv2_api.http.id, "Stage", aws_apigatewayv2_stage.default.name],
            ["AWS/ApiGateway", "Latency", "ApiId", aws_apigatewayv2_api.http.id, "Stage", aws_apigatewayv2_stage.default.name],
          ]
        }
      },
      {
        type   = "metric"
        width  = 24
        height = 6
        properties = {
          title  = "DynamoDB throttles"
          region = var.aws_region
          view   = "timeSeries"
          period = 300
          metrics = concat(
            [for name in values(local.observability_tables) : ["AWS/DynamoDB", "ThrottledRequests", "TableName", name]],
            [for name in values(local.observability_tables) : ["AWS/DynamoDB", "SystemErrors", "TableName", name]],
          )
        }
      },
      {
        type   = "metric"
        width  = 24
        height = 6
        properties = {
          title  = "Structured application and dependency failures"
          region = var.aws_region
          view   = "timeSeries"
          period = 300
          metrics = concat(
            [for name in keys(local.observability_lambda_functions) : [var.observability_namespace, "${name}ApplicationErrors"]],
            [for name in keys(local.observability_lambda_functions) : [var.observability_namespace, "${name}DependencyFailures"]],
          )
        }
      },
    ]
  })
}
