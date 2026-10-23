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
