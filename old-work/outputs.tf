output "cloudfront_domain" {
  description = "CloudFront distribution domain name"
  value       = aws_cloudfront_distribution.website.domain_name
}

output "cloudfront_distribution_id" {
  description = "CloudFront distribution ID"
  value       = aws_cloudfront_distribution.website.id
}

output "s3_bucket_name" {
  description = "S3 bucket name for website"
  value       = aws_s3_bucket.website.bucket
}

output "lambda_s3_bucket" {
  description = "S3 bucket for Lambda code"
  value       = aws_s3_bucket.lambda_code.bucket
}

output "api_gateway_base_url" {
  description = "API Gateway base URL"
  value       = aws_api_gateway_stage.booking_api_stage.invoke_url
}

output "api_endpoint_book" {
  description = "API Gateway endpoint for /book"
  value       = "${aws_api_gateway_stage.booking_api_stage.invoke_url}/book"
}

output "api_endpoint_demo_request" {
  description = "API Gateway endpoint for /demo-request"
  value       = "${aws_api_gateway_stage.booking_api_stage.invoke_url}/demo-request"
}

output "dynamodb_table_name" {
  description = "DynamoDB table for bookings"
  value       = aws_dynamodb_table.bookings.name
}

output "website_url" {
  description = "Website URL via CloudFront"
  value       = "https://${aws_cloudfront_distribution.website.domain_name}"
}
