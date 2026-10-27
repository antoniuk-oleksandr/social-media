variable "project_name" {
  type    = string
  default = "social-media"
}

variable "environment" {
  type    = string
  default = "dev"

  validation {
    condition     = var.environment == "dev"
    error_message = "infra/dev is only for the dev environment."
  }
}

variable "aws_region" {
  type    = string
  default = "eu-central-1"
}

variable "lambda_architecture" {
  type    = string
  default = "arm64"

  validation {
    condition     = var.lambda_architecture == "arm64"
    error_message = "The MVP Lambda architecture must be arm64."
  }
}

variable "lambda_runtime" {
  type    = string
  default = "provided.al2023"
}

variable "lambda_memory_mb" {
  type    = number
  default = 256
}

variable "lambda_timeout_seconds" {
  type    = number
  default = 10
}

variable "log_retention_days" {
  type    = number
  default = 14
}

variable "lambda_package_path" {
  type        = string
  description = "Path to the prebuilt auth Lambda ZIP, relative to this directory."
  default     = "../../build/auth-lambda.zip"
}

variable "users_lambda_package_path" {
  type        = string
  description = "Path to the prebuilt users Lambda ZIP, relative to this directory."
  default     = "../../build/users-lambda.zip"
}

variable "posts_lambda_package_path" {
  type        = string
  description = "Path to the prebuilt posts Lambda ZIP, relative to this directory."
  default     = "../../build/posts-lambda.zip"
}

variable "ads_lambda_package_path" {
  type        = string
  description = "Path to the prebuilt ads Lambda ZIP, relative to this directory."
  default     = "../../build/ads-lambda.zip"
}

variable "feed_lambda_package_path" {
  type        = string
  description = "Path to the prebuilt feed Lambda ZIP, relative to this directory."
  default     = "../../build/feed-lambda.zip"
}

variable "jwt_signing_key_parameter" {
  type    = string
  default = "/social-media/dev/auth/jwt-signing-key"
}

variable "jwt_active_kid" {
  type    = string
  default = "v1"
}

variable "jwt_issuer" {
  type    = string
  default = "social-media-api"
}

variable "jwt_audience" {
  type    = string
  default = "social-media-api"
}

variable "jwt_signing_key" {
  type        = string
  sensitive   = true
  description = "JWT HS256 signing key. Supply through TF_VAR_jwt_signing_key or an uncommitted tfvars file."

  validation {
    condition     = length(var.jwt_signing_key) >= 32
    error_message = "The JWT signing key must contain at least 32 characters."
  }
}

variable "media_bucket_name" {
  type        = string
  default     = ""
  description = "Optional globally unique S3 bucket name for application media."
}

variable "ads_cursor_key_parameter" {
  type    = string
  default = "/social-media/dev/ads/cursor-signing-key"
}

variable "ads_cursor_key" {
  type        = string
  sensitive   = true
  description = "Ads cursor signing key. Supply through TF_VAR_ads_cursor_key or an uncommitted tfvars file."

  validation {
    condition     = length(var.ads_cursor_key) >= 32
    error_message = "The Ads cursor signing key must contain at least 32 characters."
  }
}

variable "github_repository" {
  type        = string
  default     = "antoniuk-oleksandr/social-media"
  description = "GitHub repository allowed to assume the deployment role."
}

variable "github_repository_owner_id" {
  type        = string
  default     = "101298599"
  description = "Immutable GitHub owner ID used in OIDC subject claims."
}

variable "github_repository_id" {
  type        = string
  default     = "1424609089"
  description = "Immutable GitHub repository ID used in OIDC subject claims."
}

variable "github_environment" {
  type        = string
  default     = "dev"
  description = "GitHub environment allowed to assume the deployment role."
}

variable "github_actions_role_name" {
  type        = string
  default     = "social-media-github-actions-deploy"
  description = "IAM role assumed by GitHub Actions through OIDC."
}

variable "terraform_state_bucket" {
  type        = string
  default     = "social-media-terraform-state-851725490860"
  description = "Existing S3 bucket used by the Terraform backend."
}
