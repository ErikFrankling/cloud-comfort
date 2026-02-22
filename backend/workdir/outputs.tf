output "cloudfront_domain" {
  description = "CloudFront distribution domain name"
  value       = aws_cloudfront_distribution.website.domain_name
}

output "website_bucket" {
  description = "S3 bucket name for website hosting"
  value       = aws_s3_bucket.website.bucket
}

output "api_endpoint" {
  description = "API Gateway endpoint for demo requests"
  value       = "${aws_api_gateway_deployment.prod.invoke_url}/demo-request"
}

output "distribution_id" {
  description = "CloudFront Distribution ID for cache invalidation"
  value       = aws_cloudfront_distribution.website.id
}

output "lambda_function_name" {
  description = "Lambda function name for booking handler"
  value       = aws_lambda_function.booking_handler.function_name
}

output "dynamodb_table" {
  description = "DynamoDB table for storing bookings"
  value       = aws_dynamodb_table.bookings.name
}
