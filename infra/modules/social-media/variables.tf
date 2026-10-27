variable "project_name" { type = string }
variable "environment" { type = string }
variable "aws_region" { type = string }
variable "lambda_architecture" { type = string }
variable "lambda_runtime" { type = string }
variable "lambda_memory_mb" { type = number }
variable "lambda_timeout_seconds" { type = number }
variable "log_retention_days" { type = number }
variable "lambda_package_path" { type = string }
variable "users_lambda_package_path" { type = string }
variable "posts_lambda_package_path" { type = string }
variable "ads_lambda_package_path" { type = string }
variable "feed_lambda_package_path" { type = string }
variable "jwt_signing_key_parameter" { type = string }
variable "jwt_active_kid" { type = string }
variable "jwt_issuer" { type = string }
variable "jwt_audience" { type = string }
variable "jwt_signing_key" {
  type      = string
  sensitive = true
}
variable "media_bucket_name" { type = string }
variable "ads_cursor_key_parameter" { type = string }
variable "ads_cursor_key" {
  type      = string
  sensitive = true
}
variable "github_repository" { type = string }
variable "github_repository_owner_id" { type = string }
variable "github_repository_id" { type = string }
variable "github_environment" { type = string }
variable "github_actions_role_name" { type = string }
variable "terraform_state_bucket" { type = string }
