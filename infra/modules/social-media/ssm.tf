resource "aws_ssm_parameter" "jwt_signing_key" {
  name        = var.jwt_signing_key_parameter
  description = "Social Media dev JWT signing key"
  type        = "SecureString"
  value       = var.jwt_signing_key
  tier        = "Standard"

  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "ads_cursor_signing_key" {
  name        = var.ads_cursor_key_parameter
  description = "Social Media dev Ads cursor signing key"
  type        = "SecureString"
  value       = var.ads_cursor_key
  tier        = "Standard"

  lifecycle {
    ignore_changes = [value]
  }
}

