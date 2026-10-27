locals {
  name_prefix = "${var.project_name}-${var.environment}"

  users_table    = "${local.name_prefix}-users"
  uniques_table  = "${local.name_prefix}-uniques"
  refresh_table  = "${local.name_prefix}-refresh-sessions"
  posts_table    = "${local.name_prefix}-posts"
  auth_function  = "${local.name_prefix}-auth"
  users_function = "${local.name_prefix}-users"
  posts_function = "${local.name_prefix}-posts"
  ads_table      = "${local.name_prefix}-ads"
  ads_function   = "${local.name_prefix}-ads"
  feed_function  = "${local.name_prefix}-feed"

  media_bucket_name = var.media_bucket_name != "" ? var.media_bucket_name : "${local.name_prefix}-media-${data.aws_caller_identity.current.account_id}"
}

data "aws_caller_identity" "current" {}
