output "cloudfront_url" {
  description = "CloudFront distribution URL for Cleversel landing page"
  value       = "https://${aws_cloudfront_distribution.website.domain_name}"
}

output "cloudfront_distribution_id" {
  description = "CloudFront distribution ID"
  value       = aws_cloudfront_distribution.website.id
}

output "s3_bucket_name" {
  description = "S3 bucket name for the website"
  value       = aws_s3_bucket.website.bucket
}
