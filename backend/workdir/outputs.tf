# S3 Bucket Outputs
output "bucket_name" {
  description = "Name of the S3 website bucket"
  value       = aws_s3_bucket.website.bucket
}

output "bucket_arn" {
  description = "ARN of the S3 website bucket"
  value       = aws_s3_bucket.website.arn
}

output "bucket_id" {
  description = "ID of the S3 website bucket"
  value       = aws_s3_bucket.website.id
}

output "storage_bucket_name" {
  description = "Name of the S3 storage bucket"
  value       = aws_s3_bucket.storage.bucket
}

output "storage_bucket_arn" {
  description = "ARN of the S3 storage bucket"
  value       = aws_s3_bucket.storage.arn
}

# CloudFront Outputs
output "cloudfront_domain_name" {
  description = "Domain name of the CloudFront distribution"
  value       = aws_cloudfront_distribution.website.domain_name
}

output "cloudfront_distribution_id" {
  description = "ID of the CloudFront distribution"
  value       = aws_cloudfront_distribution.website.id
}

output "website_url" {
  description = "URL to access the website via CloudFront"
  value       = "https://${aws_cloudfront_distribution.website.domain_name}"
}

# DynamoDB Outputs
output "dynamodb_table_name" {
  description = "Name of the DynamoDB table"
  value       = aws_dynamodb_table.main.name
}

output "dynamodb_table_arn" {
  description = "ARN of the DynamoDB table"
  value       = aws_dynamodb_table.main.arn
}

# Lambda Outputs
output "lambda_function_name" {
  description = "Name of the Lambda function"
  value       = aws_lambda_function.api.function_name
}

output "lambda_function_arn" {
  description = "ARN of the Lambda function"
  value       = aws_lambda_function.api.arn
}

output "lambda_function_url" {
  description = "URL endpoint for the Lambda function"
  value       = aws_lambda_function_url.api.function_url
}
