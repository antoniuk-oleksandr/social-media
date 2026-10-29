output "api_endpoint" { value = aws_apigatewayv2_api.http.api_endpoint }
output "auth_function_name" { value = aws_lambda_function.auth.function_name }
output "users_function_name" { value = aws_lambda_function.users.function_name }
output "users_table_name" { value = aws_dynamodb_table.users.name }
output "uniques_table_name" { value = aws_dynamodb_table.uniques.name }
output "refresh_sessions_table_name" { value = aws_dynamodb_table.refresh_sessions.name }
output "media_bucket_name" { value = aws_s3_bucket.media.bucket }
output "jwt_signing_key_parameter" { value = aws_ssm_parameter.jwt_signing_key.name }
output "ads_cursor_key_parameter" { value = aws_ssm_parameter.ads_cursor_signing_key.name }
output "github_actions_role_arn" { value = aws_iam_role.github_actions_deploy.arn }
output "observability_dashboard_name" { value = aws_cloudwatch_dashboard.main.dashboard_name }
output "observability_alarm_names" {
  value = concat(
    [aws_cloudwatch_metric_alarm.api_gateway_5xx.alarm_name],
    [aws_cloudwatch_metric_alarm.api_gateway_4xx.alarm_name],
    [aws_cloudwatch_metric_alarm.api_gateway_latency.alarm_name],
    [for alarm in aws_cloudwatch_metric_alarm.dynamodb_throttles : alarm.alarm_name],
    [for alarm in aws_cloudwatch_metric_alarm.dynamodb_system_errors : alarm.alarm_name],
    [for alarm in aws_cloudwatch_metric_alarm.lambda_errors : alarm.alarm_name],
    [for alarm in aws_cloudwatch_metric_alarm.lambda_throttles : alarm.alarm_name],
    [for alarm in aws_cloudwatch_metric_alarm.lambda_duration : alarm.alarm_name],
  )
}
