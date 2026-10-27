module "social_media" {
  source = "../modules/social-media"

  project_name           = var.project_name
  environment            = var.environment
  aws_region             = var.aws_region
  lambda_architecture    = var.lambda_architecture
  lambda_runtime         = var.lambda_runtime
  lambda_memory_mb       = var.lambda_memory_mb
  lambda_timeout_seconds = var.lambda_timeout_seconds
  log_retention_days     = var.log_retention_days

  lambda_package_path       = abspath(var.lambda_package_path)
  users_lambda_package_path = abspath(var.users_lambda_package_path)
  posts_lambda_package_path = abspath(var.posts_lambda_package_path)
  ads_lambda_package_path   = abspath(var.ads_lambda_package_path)
  feed_lambda_package_path  = abspath(var.feed_lambda_package_path)

  jwt_signing_key_parameter = var.jwt_signing_key_parameter
  jwt_active_kid            = var.jwt_active_kid
  jwt_issuer                = var.jwt_issuer
  jwt_audience              = var.jwt_audience
  jwt_signing_key           = var.jwt_signing_key
  ads_cursor_key_parameter  = var.ads_cursor_key_parameter
  ads_cursor_key            = var.ads_cursor_key

  github_repository          = var.github_repository
  github_repository_owner_id = var.github_repository_owner_id
  github_repository_id       = var.github_repository_id
  github_environment         = var.github_environment
  github_actions_role_name   = var.github_actions_role_name
  terraform_state_bucket     = var.terraform_state_bucket
  media_bucket_name          = var.media_bucket_name
}
