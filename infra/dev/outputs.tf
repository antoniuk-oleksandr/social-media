output "api_endpoint" {
  value = aws_apigatewayv2_api.http.api_endpoint
}

output "auth_function_name" {
  value = aws_lambda_function.auth.function_name
}

output "users_table_name" {
  value = aws_dynamodb_table.users.name
}

output "uniques_table_name" {
  value = aws_dynamodb_table.uniques.name
}

output "refresh_sessions_table_name" {
  value = aws_dynamodb_table.refresh_sessions.name
}

output "media_bucket_name" {
  value = aws_s3_bucket.media.bucket
}
