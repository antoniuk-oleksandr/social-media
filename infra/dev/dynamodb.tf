resource "aws_dynamodb_table" "users" {
  name         = local.users_table
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "userId"

  attribute {
    name = "userId"
    type = "S"
  }

  attribute {
    name = "username"
    type = "S"
  }

  attribute {
    name = "email"
    type = "S"
  }

  global_secondary_index {
    name            = "UsernameIndex"
    hash_key        = "username"
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "EmailIndex"
    hash_key        = "email"
    projection_type = "ALL"
  }

  point_in_time_recovery {
    enabled = false
  }
}

resource "aws_dynamodb_table" "uniques" {
  name         = local.uniques_table
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "uniqueKey"

  attribute {
    name = "uniqueKey"
    type = "S"
  }
}

resource "aws_dynamodb_table" "refresh_sessions" {
  name         = local.refresh_table
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "tokenHash"

  attribute {
    name = "tokenHash"
    type = "S"
  }

  attribute {
    name = "sessionId"
    type = "S"
  }

  global_secondary_index {
    name            = "SessionIdIndex"
    hash_key        = "sessionId"
    projection_type = "ALL"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }
}

resource "aws_dynamodb_table" "posts" {
  name         = local.posts_table
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "postId"

  attribute {
    name = "postId"
    type = "S"
  }

  attribute {
    name = "feedPartition"
    type = "S"
  }

  attribute {
    name = "createdAtEpoch"
    type = "N"
  }

  global_secondary_index {
    name            = "FeedPostsIndex"
    hash_key        = "feedPartition"
    range_key       = "createdAtEpoch"
    projection_type = "ALL"
  }
}
