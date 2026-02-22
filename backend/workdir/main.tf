locals {
  unique_suffix = substr(md5(var.bucket_name), 0, 8)
  website_bucket_name = "${var.bucket_name}-${local.unique_suffix}"
  lambda_bucket_name = "${var.bucket_name}-lambda-code-${local.unique_suffix}"
}

# S3 Bucket for website hosting
resource "aws_s3_bucket" "website" {
  bucket = local.website_bucket_name
}

resource "aws_s3_bucket_public_access_block" "website" {
  bucket = aws_s3_bucket.website.id

  block_public_acls       = false
  block_public_policy     = false
  ignore_public_acls      = false
  restrict_public_buckets = false
}

resource "aws_s3_bucket_website_configuration" "website" {
  bucket = aws_s3_bucket.website.id

  index_document {
    suffix = "index.html"
  }

  error_document {
    key = "index.html"
  }
}

resource "aws_s3_bucket_policy" "website" {
  bucket = aws_s3_bucket.website.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "PublicReadGetObject"
        Effect    = "Allow"
        Principal = "*"
        Action    = "s3:GetObject"
        Resource  = "${aws_s3_bucket.website.arn}/*"
      },
      {
        Sid       = "CloudFrontAccess"
        Effect    = "Allow"
        Principal = {
          Service = "cloudfront.amazonaws.com"
        }
        Action   = "s3:GetObject"
        Resource = "${aws_s3_bucket.website.arn}/*"
        Condition = {
          StringEquals = {
            "AWS:SourceArn" = aws_cloudfront_distribution.website.arn
          }
        }
      }
    ]
  })

  depends_on = [aws_s3_bucket_public_access_block.website]
}

# Origin Access Control for CloudFront
resource "aws_cloudfront_origin_access_control" "website" {
  name                              = "${local.website_bucket_name}-oac"
  description                       = "OAC for Cleversel website"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

# CloudFront Distribution
resource "aws_cloudfront_distribution" "website" {
  enabled             = true
  is_ipv6_enabled     = true
  comment             = "Cleversel Landing Page"
  default_root_object = "index.html"
  price_class         = "PriceClass_100"

  origin {
    domain_name              = aws_s3_bucket.website.bucket_regional_domain_name
    origin_id                = "S3-${local.website_bucket_name}"
    origin_access_control_id = aws_cloudfront_origin_access_control.website.id
  }

  default_cache_behavior {
    allowed_methods  = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods   = ["GET", "HEAD"]
    target_origin_id = "S3-${local.website_bucket_name}"

    forwarded_values {
      query_string = false
      cookies {
        forward = "none"
      }
    }

    viewer_protocol_policy = "redirect-to-https"
    min_ttl                = 0
    default_ttl            = 3600
    max_ttl                = 86400
  }

  custom_error_response {
    error_code         = 403
    response_code      = 200
    response_page_path = "/index.html"
  }

  custom_error_response {
    error_code         = 404
    response_code      = 200
    response_page_path = "/index.html"
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
    Name = "Cleversel Website"
  }
}

# S3 Bucket for Lambda code
resource "aws_s3_bucket" "lambda_code" {
  bucket = local.lambda_bucket_name
}

resource "aws_s3_bucket_public_access_block" "lambda_code" {
  bucket = aws_s3_bucket.lambda_code.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# DynamoDB table for bookings
resource "aws_dynamodb_table" "bookings" {
  name           = "${local.website_bucket_name}-bookings"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "id"

  attribute {
    name = "id"
    type = "S"
  }

  attribute {
    name = "email"
    type = "S"
  }

  attribute {
    name = "createdAt"
    type = "S"
  }

  global_secondary_index {
    name            = "EmailIndex"
    hash_key        = "email"
    range_key       = "createdAt"
    projection_type = "ALL"
  }

  tags = {
    Name = "Cleversel Bookings"
  }
}

# IAM Role for Lambda
resource "aws_iam_role" "lambda_role" {
  name = "${local.website_bucket_name}-lambda-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}

resource "aws_iam_role_policy" "lambda_dynamodb" {
  name = "${local.website_bucket_name}-lambda-dynamodb-policy"
  role = aws_iam_role.lambda_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:PutItem",
          "dynamodb:GetItem",
          "dynamodb:Query",
          "dynamodb:Scan"
        ]
        Resource = [
          aws_dynamodb_table.bookings.arn,
          "${aws_dynamodb_table.bookings.arn}/index/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:*:*:*"
      }
    ]
  })
}

# Bootstrap placeholder ZIP for Lambda (empty function that will be replaced by CI/CD)
resource "aws_s3_object" "lambda_placeholder" {
  bucket  = aws_s3_bucket.lambda_code.bucket
  key     = "lambda-booking-handler.zip"
  content_base64 = "UEsDBBQAAAAIAP1VjVgAAAAAAAAAAAAAAAAIABAAbGFtYmRhL2ZpbGVzL1BLAwQUAAAACAD9VY1YAAAAAAAAAAAAAAAADwAQAGxhbWJkYS9pbmRleC5qc1BLAQIUAxQAAAAIAP1VjVgAAAAAAAAAAAAAAAAIAAwAAAAAAAAAAAAAAFBLBQYAAAAAAQABADYAAAA2AAAAAAA=" # base64 of minimal zip
}

# Lambda function for handling demo requests
resource "aws_lambda_function" "booking_handler" {
  function_name = "${local.website_bucket_name}-booking-handler"
  role          = aws_iam_role.lambda_role.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
  timeout       = 10
  memory_size   = 256

  s3_bucket = aws_s3_bucket.lambda_code.bucket
  s3_key    = aws_s3_object.lambda_placeholder.key

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.bookings.name
    }
  }

  lifecycle {
    ignore_changes = [s3_key, source_code_hash]
  }
}

# CloudWatch Log Group for Lambda
resource "aws_cloudwatch_log_group" "lambda_logs" {
  name              = "/aws/lambda/${aws_lambda_function.booking_handler.function_name}"
  retention_in_days = 7
}

# API Gateway
resource "aws_api_gateway_rest_api" "demo_api" {
  name        = "${local.website_bucket_name}-demo-api"
  description = "API for Cleversel demo requests"

  endpoint_configuration {
    types = ["REGIONAL"]
  }
}

resource "aws_api_gateway_resource" "demo" {
  rest_api_id = aws_api_gateway_rest_api.demo_api.id
  parent_id   = aws_api_gateway_rest_api.demo_api.root_resource_id
  path_part   = "demo-request"
}

resource "aws_api_gateway_method" "demo_post" {
  rest_api_id   = aws_api_gateway_rest_api.demo_api.id
  resource_id   = aws_api_gateway_resource.demo.id
  http_method   = "POST"
  authorization = "NONE"
}

resource "aws_api_gateway_integration" "lambda" {
  rest_api_id = aws_api_gateway_rest_api.demo_api.id
  resource_id = aws_api_gateway_resource.demo.id
  http_method = aws_api_gateway_method.demo_post.http_method

  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.booking_handler.invoke_arn
}

resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowExecutionFromAPIGateway"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.booking_handler.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_rest_api.demo_api.execution_arn}/*/*"
}

resource "aws_api_gateway_deployment" "prod" {
  depends_on = [
    aws_api_gateway_integration.lambda,
    aws_api_gateway_method.demo_post
  ]

  rest_api_id = aws_api_gateway_rest_api.demo_api.id
  stage_name  = "prod"

  lifecycle {
    create_before_destroy = true
  }
}

# GitHub Actions workflow file
resource "github_repository_file" "deploy_workflow" {
  repository          = "Cleversel-Website"
  file                = ".github/workflows/deploy.yml"
  content             = <<-EOT
name: Deploy Website and Lambda

on:
  push:
    branches:
      - main

permissions:
  contents: write
  id-token: write

env:
  AWS_REGION: ${var.aws_region}
  S3_BUCKET: ${aws_s3_bucket.website.bucket}
  DISTRIBUTION_ID: ${aws_cloudfront_distribution.website.id}
  LAMBDA_CODE_BUCKET: ${aws_s3_bucket.lambda_code.bucket}
  LAMBDA_FUNCTION_NAME: ${aws_lambda_function.booking_handler.function_name}
  API_ENDPOINT: ${aws_api_gateway_deployment.prod.invoke_url}/demo-request

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - name: Install Dependencies
        run: npm ci

      - name: Build Project
        run: npm run build
        env:
          VITE_API_ENDPOINT: ${{ env.API_ENDPOINT }}

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: ${{ env.AWS_REGION }}

      - name: Package and Deploy Lambda
        run: |
          cd lambda
          npm install
          zip -r ../lambda-booking-handler.zip index.js node_modules/
          cd ..
          aws s3 cp lambda-booking-handler.zip s3://${{ env.LAMBDA_CODE_BUCKET }}/lambda-booking-handler.zip
          aws lambda update-function-code --function-name ${{ env.LAMBDA_FUNCTION_NAME }} --s3-bucket ${{ env.LAMBDA_CODE_BUCKET }} --s3-key lambda-booking-handler.zip

      - name: Deploy to S3
        run: |
          aws s3 sync ./dist s3://${{ env.S3_BUCKET }} --delete

      - name: Invalidate CloudFront Cache
        run: |
          aws cloudfront create-invalidation --distribution-id ${{ env.DISTRIBUTION_ID }} --paths "/*"
  EOT
  commit_message      = "Update deploy workflow via Terraform"
  overwrite_on_create = true
}

# GitHub Secrets for AWS credentials
resource "github_actions_secret" "aws_access_key_id" {
  repository      = "Cleversel-Website"
  secret_name     = "AWS_ACCESS_KEY_ID"
  plaintext_value = var.aws_access_key_id
}

resource "github_actions_secret" "aws_secret_access_key" {
  repository      = "Cleversel-Website"
  secret_name     = "AWS_SECRET_ACCESS_KEY"
  plaintext_value = var.aws_secret_access_key
}
