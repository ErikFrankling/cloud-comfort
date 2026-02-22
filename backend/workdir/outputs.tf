output "cloudfront_domain" {
  description = "CloudFront CDN domain for media"
  value       = aws_cloudfront_distribution.media.domain_name
}

output "cognito_user_pool_id" {
  description = "Cognito User Pool ID"
  value       = aws_cognito_user_pool.users.id
}

output "cognito_client_id" {
  description = "Cognito App Client ID"
  value       = aws_cognito_user_pool_client.api_client.id
}

output "dynamodb_tables" {
  description = "DynamoDB table names"
  value = {
    users     = aws_dynamodb_table.users.name
    tweets    = aws_dynamodb_table.tweets.name
    timelines = aws_dynamodb_table.timelines.name
    follows   = aws_dynamodb_table.follows.name
  }
}

output "sqs_queue_url" {
  description = "SQS queue URL for tweet fanout"
  value       = aws_sqs_queue.tweet_fanout.url
}

output "s3_media_bucket" {
  description = "S3 bucket for media storage"
  value       = aws_s3_bucket.media.bucket
}
