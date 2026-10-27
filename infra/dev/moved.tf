// Preserve the existing dev state addresses after extracting the shared module.
// These moves avoid replacing deployed resources during the next plan.
moved {
  from = aws_ssm_parameter.jwt_signing_key
  to   = module.social_media.aws_ssm_parameter.jwt_signing_key
}
moved {
  from = aws_ssm_parameter.ads_cursor_signing_key
  to   = module.social_media.aws_ssm_parameter.ads_cursor_signing_key
}
moved {
  from = aws_cloudwatch_log_group.auth
  to   = module.social_media.aws_cloudwatch_log_group.auth
}
moved {
  from = aws_lambda_function.auth
  to   = module.social_media.aws_lambda_function.auth
}
moved {
  from = aws_cloudwatch_log_group.users
  to   = module.social_media.aws_cloudwatch_log_group.users
}
moved {
  from = aws_lambda_function.users
  to   = module.social_media.aws_lambda_function.users
}
moved {
  from = aws_cloudwatch_log_group.posts
  to   = module.social_media.aws_cloudwatch_log_group.posts
}
moved {
  from = aws_lambda_function.posts
  to   = module.social_media.aws_lambda_function.posts
}
moved {
  from = aws_cloudwatch_log_group.ads
  to   = module.social_media.aws_cloudwatch_log_group.ads
}
moved {
  from = aws_lambda_function.ads
  to   = module.social_media.aws_lambda_function.ads
}
moved {
  from = aws_cloudwatch_log_group.feed
  to   = module.social_media.aws_cloudwatch_log_group.feed
}
moved {
  from = aws_lambda_function.feed
  to   = module.social_media.aws_lambda_function.feed
}
moved {
  from = aws_s3_bucket.media
  to   = module.social_media.aws_s3_bucket.media
}
moved {
  from = aws_s3_bucket_ownership_controls.media
  to   = module.social_media.aws_s3_bucket_ownership_controls.media
}
moved {
  from = aws_s3_bucket_public_access_block.media
  to   = module.social_media.aws_s3_bucket_public_access_block.media
}
moved {
  from = aws_s3_bucket_versioning.media
  to   = module.social_media.aws_s3_bucket_versioning.media
}
moved {
  from = aws_s3_bucket_server_side_encryption_configuration.media
  to   = module.social_media.aws_s3_bucket_server_side_encryption_configuration.media
}
moved {
  from = aws_iam_role.auth_lambda
  to   = module.social_media.aws_iam_role.auth_lambda
}
moved {
  from = aws_iam_role_policy.auth_lambda
  to   = module.social_media.aws_iam_role_policy.auth_lambda
}
moved {
  from = aws_iam_role.users_lambda
  to   = module.social_media.aws_iam_role.users_lambda
}
moved {
  from = aws_iam_role_policy.users_lambda
  to   = module.social_media.aws_iam_role_policy.users_lambda
}
moved {
  from = aws_iam_role.github_actions_deploy
  to   = module.social_media.aws_iam_role.github_actions_deploy
}
moved {
  from = aws_iam_role_policy.github_actions_deploy
  to   = module.social_media.aws_iam_role_policy.github_actions_deploy
}
moved {
  from = aws_iam_role.posts_lambda
  to   = module.social_media.aws_iam_role.posts_lambda
}
moved {
  from = aws_iam_role_policy.posts_lambda
  to   = module.social_media.aws_iam_role_policy.posts_lambda
}
moved {
  from = aws_iam_role.ads_lambda
  to   = module.social_media.aws_iam_role.ads_lambda
}
moved {
  from = aws_iam_role_policy.ads_lambda
  to   = module.social_media.aws_iam_role_policy.ads_lambda
}
moved {
  from = aws_dynamodb_table.users
  to   = module.social_media.aws_dynamodb_table.users
}
moved {
  from = aws_dynamodb_table.uniques
  to   = module.social_media.aws_dynamodb_table.uniques
}
moved {
  from = aws_dynamodb_table.refresh_sessions
  to   = module.social_media.aws_dynamodb_table.refresh_sessions
}
moved {
  from = aws_dynamodb_table.posts
  to   = module.social_media.aws_dynamodb_table.posts
}
moved {
  from = aws_dynamodb_table.ads
  to   = module.social_media.aws_dynamodb_table.ads
}
moved {
  from = aws_apigatewayv2_api.http
  to   = module.social_media.aws_apigatewayv2_api.http
}
moved {
  from = aws_apigatewayv2_integration.auth
  to   = module.social_media.aws_apigatewayv2_integration.auth
}
moved {
  from = aws_apigatewayv2_integration.users
  to   = module.social_media.aws_apigatewayv2_integration.users
}
moved {
  from = aws_apigatewayv2_integration.posts
  to   = module.social_media.aws_apigatewayv2_integration.posts
}
moved {
  from = aws_apigatewayv2_integration.ads
  to   = module.social_media.aws_apigatewayv2_integration.ads
}
moved {
  from = aws_apigatewayv2_integration.feed
  to   = module.social_media.aws_apigatewayv2_integration.feed
}
moved {
  from = aws_apigatewayv2_route.feed
  to   = module.social_media.aws_apigatewayv2_route.feed
}
moved {
  from = aws_apigatewayv2_route.ads_create
  to   = module.social_media.aws_apigatewayv2_route.ads_create
}
moved {
  from = aws_apigatewayv2_route.ads_list
  to   = module.social_media.aws_apigatewayv2_route.ads_list
}
moved {
  from = aws_apigatewayv2_route.ads_by_id
  to   = module.social_media.aws_apigatewayv2_route.ads_by_id
}
moved {
  from = aws_apigatewayv2_route.ads_update_by_id
  to   = module.social_media.aws_apigatewayv2_route.ads_update_by_id
}
moved {
  from = aws_apigatewayv2_route.ads_delete_by_id
  to   = module.social_media.aws_apigatewayv2_route.ads_delete_by_id
}
moved {
  from = aws_apigatewayv2_route.users_by_id
  to   = module.social_media.aws_apigatewayv2_route.users_by_id
}
moved {
  from = aws_apigatewayv2_route.users_me
  to   = module.social_media.aws_apigatewayv2_route.users_me
}
moved {
  from = aws_apigatewayv2_route.posts_create
  to   = module.social_media.aws_apigatewayv2_route.posts_create
}
moved {
  from = aws_apigatewayv2_route.posts_by_id
  to   = module.social_media.aws_apigatewayv2_route.posts_by_id
}
moved {
  from = aws_apigatewayv2_route.posts_delete_by_id
  to   = module.social_media.aws_apigatewayv2_route.posts_delete_by_id
}
moved {
  from = aws_apigatewayv2_route.auth_register
  to   = module.social_media.aws_apigatewayv2_route.auth_register
}
moved {
  from = aws_apigatewayv2_route.auth_login
  to   = module.social_media.aws_apigatewayv2_route.auth_login
}
moved {
  from = aws_apigatewayv2_route.auth_refresh
  to   = module.social_media.aws_apigatewayv2_route.auth_refresh
}
moved {
  from = aws_apigatewayv2_route.auth_logout
  to   = module.social_media.aws_apigatewayv2_route.auth_logout
}
moved {
  from = aws_apigatewayv2_stage.default
  to   = module.social_media.aws_apigatewayv2_stage.default
}
moved {
  from = aws_lambda_permission.api_gateway_auth
  to   = module.social_media.aws_lambda_permission.api_gateway_auth
}
moved {
  from = aws_lambda_permission.api_gateway_users
  to   = module.social_media.aws_lambda_permission.api_gateway_users
}
moved {
  from = aws_lambda_permission.api_gateway_posts
  to   = module.social_media.aws_lambda_permission.api_gateway_posts
}
moved {
  from = aws_lambda_permission.api_gateway_feed
  to   = module.social_media.aws_lambda_permission.api_gateway_feed
}
moved {
  from = aws_lambda_permission.api_gateway_ads
  to   = module.social_media.aws_lambda_permission.api_gateway_ads
}
