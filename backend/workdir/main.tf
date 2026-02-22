# S3 Bucket for media storage (profile pics, tweet images)
resource "aws_s3_bucket" "media" {
  bucket_prefix = "${var.project_name}-media-"
}

resource "aws_s3_bucket_public_access_block" "media" {
  bucket = aws_s3_bucket.media.id

  block_public_acls       = false
  block_public_policy     = false
  ignore_public_acls      = false
  restrict_public_buckets = false
}

resource "aws_s3_bucket_policy" "media" {
  bucket = aws_s3_bucket.media.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "PublicReadGetObject"
        Effect    = "Allow"
        Principal = "*"
        Action    = "s3:GetObject"
        Resource  = "${aws_s3_bucket.media.arn}/*"
      }
    ]
  })
}

# CloudFront CDN for media
resource "aws_cloudfront_distribution" "media" {
  enabled         = true
  is_ipv6_enabled = true
  price_class     = "PriceClass_100"

  origin {
    domain_name = aws_s3_bucket.media.bucket_regional_domain_name
    origin_id   = "S3-${aws_s3_bucket.media.bucket}"

    s3_origin_config {
      origin_access_identity = aws_cloudfront_origin_access_identity.media.cloudfront_access_identity_path
    }
  }

  default_cache_behavior {
    allowed_methods  = ["GET", "HEAD", "OPTIONS"]
    cached_methods   = ["GET", "HEAD"]
    target_origin_id = "S3-${aws_s3_bucket.media.bucket}"

    forwarded_values {
      query_string = false
      cookies {
        forward = "none"
      }
    }

    viewer_protocol_policy = "allow-all"
    min_ttl                = 0
    default_ttl            = 3600
    max_ttl                = 86400
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }

  tags = {
    Name        = "${var.project_name}-cdn"
    Environment = var.environment
  }
}

resource "aws_cloudfront_origin_access_identity" "media" {
  comment = "OAI for ${var.project_name} media bucket"
}

# DynamoDB Tables

# Users table
resource "aws_dynamodb_table" "users" {
  name         = "${var.project_name}-users"
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

  global_secondary_index {
    name            = "username-index"
    hash_key        = "username"
    projection_type = "ALL"
  }

  tags = {
    Name        = "${var.project_name}-users"
    Environment = var.environment
  }
}

# Tweets table
resource "aws_dynamodb_table" "tweets" {
  name         = "${var.project_name}-tweets"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "tweetId"
  range_key    = "timestamp"

  attribute {
    name = "tweetId"
    type = "S"
  }

  attribute {
    name = "timestamp"
    type = "N"
  }

  attribute {
    name = "userId"
    type = "S"
  }

  global_secondary_index {
    name            = "user-tweets-index"
    hash_key        = "userId"
    range_key       = "timestamp"
    projection_type = "ALL"
  }

  tags = {
    Name        = "${var.project_name}-tweets"
    Environment = var.environment
  }
}

# Timelines table (home feed)
resource "aws_dynamodb_table" "timelines" {
  name         = "${var.project_name}-timelines"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "userId"
  range_key    = "timestamp"

  attribute {
    name = "userId"
    type = "S"
  }

  attribute {
    name = "timestamp"
    type = "N"
  }

  tags = {
    Name        = "${var.project_name}-timelines"
    Environment = var.environment
  }
}

# Follows table (who follows who)
resource "aws_dynamodb_table" "follows" {
  name         = "${var.project_name}-follows"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "userId"
  range_key    = "followsId"

  attribute {
    name = "userId"
    type = "S"
  }

  attribute {
    name = "followsId"
    type = "S"
  }

  global_secondary_index {
    name            = "followed-by-index"
    hash_key        = "followsId"
    range_key       = "userId"
    projection_type = "ALL"
  }

  tags = {
    Name        = "${var.project_name}-follows"
    Environment = var.environment
  }
}

# SQS Queue for tweet fanout (timeline generation)
resource "aws_sqs_queue" "tweet_fanout_dlq" {
  name = "${var.project_name}-tweet-fanout-dlq"

  tags = {
    Name        = "${var.project_name}-tweet-fanout-dlq"
    Environment = var.environment
  }
}

resource "aws_sqs_queue" "tweet_fanout" {
  name                       = "${var.project_name}-tweet-fanout"
  message_retention_seconds  = 86400
  visibility_timeout_seconds = 30
  receive_wait_time_seconds  = 5

  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.tweet_fanout_dlq.arn
    maxReceiveCount     = 3
  })

  tags = {
    Name        = "${var.project_name}-tweet-fanout"
    Environment = var.environment
  }
}

# Cognito User Pool for authentication
resource "aws_cognito_user_pool" "users" {
  name = "${var.project_name}-user-pool"

  username_attributes      = ["email"]
  auto_verified_attributes = ["email"]

  password_policy {
    minimum_length    = 8
    require_lowercase = true
    require_numbers   = true
    require_symbols   = true
    require_uppercase = true
  }

  account_recovery_setting {
    recovery_mechanism {
      name     = "verified_email"
      priority = 1
    }
  }

  schema {
    name                = "username"
    attribute_data_type = "String"
    mutable             = true
    required            = false
    string_attribute_constraints {
      min_length = 3
      max_length = 15
    }
  }

  tags = {
    Name        = "${var.project_name}-user-pool"
    Environment = var.environment
  }
}

resource "aws_cognito_user_pool_client" "api_client" {
  name         = "${var.project_name}-api-client"
  user_pool_id = aws_cognito_user_pool.users.id

  generate_secret               = false
  allowed_oauth_flows           = ["implicit"]
  allowed_oauth_scopes          = ["email", "openid", "profile"]
  supported_identity_providers  = ["COGNITO"]
  callback_urls                 = ["http://localhost:3000/callback"]
  logout_urls                   = ["http://localhost:3000/logout"]
  explicit_auth_flows           = ["ALLOW_USER_PASSWORD_AUTH", "ALLOW_REFRESH_TOKEN_AUTH"]
  prevent_user_existence_errors = "ENABLED"
}
