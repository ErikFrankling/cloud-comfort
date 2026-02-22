# S3 Bucket for website hosting
resource "aws_s3_bucket" "website" {
  bucket = "${var.bucket_name}-${random_id.bucket_suffix.hex}"
}

resource "random_id" "bucket_suffix" {
  byte_length = 4
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

# Disable block public access to allow bucket policy
resource "aws_s3_bucket_public_access_block" "website" {
  bucket = aws_s3_bucket.website.id

  block_public_acls       = false
  block_public_policy     = false
  ignore_public_acls      = false
  restrict_public_buckets = false
}

# CloudFront Origin Access Control
resource "aws_cloudfront_origin_access_control" "website" {
  name                              = "${var.bucket_name}-${random_id.bucket_suffix.hex}-oac"
  description                       = "OAC for S3 website"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

# Get current AWS account ID for the bucket policy
data "aws_caller_identity" "current" {}

# CloudFront Distribution - created before bucket policy to avoid circular dependency
resource "aws_cloudfront_distribution" "website" {
  enabled             = true
  is_ipv6_enabled     = true
  comment             = "CloudFront distribution for ${var.github_repo}"
  default_root_object = "index.html"
  price_class         = "PriceClass_100"

  origin {
    domain_name              = aws_s3_bucket.website.bucket_regional_domain_name
    origin_id                = "S3-${aws_s3_bucket.website.bucket}"
    origin_access_control_id = aws_cloudfront_origin_access_control.website.id
  }

  default_cache_behavior {
    allowed_methods  = ["GET", "HEAD", "OPTIONS"]
    cached_methods   = ["GET", "HEAD"]
    target_origin_id = "S3-${aws_s3_bucket.website.bucket}"

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
    compress               = true
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
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

  tags = {
    Name = aws_s3_bucket.website.bucket
  }

  depends_on = [aws_s3_bucket_public_access_block.website]
}

# S3 Bucket Policy - allows CloudFront OAC to read objects
resource "aws_s3_bucket_policy" "website" {
  bucket = aws_s3_bucket.website.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowCloudFrontOACRead"
        Effect = "Allow"
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

  depends_on = [aws_cloudfront_distribution.website, aws_s3_bucket_public_access_block.website]
}

# GitHub Actions Workflow for deployment
resource "github_repository_file" "deploy_workflow" {
  repository          = var.github_repo
  file                = ".github/workflows/deploy.yml"
  content             = <<-EOT
name: Deploy Website and Lambda

on:
  push:
    branches:
      - main

permissions:
  contents: read
  id-token: write

env:
  AWS_REGION: ${var.aws_region}
  S3_BUCKET: ${aws_s3_bucket.website.bucket}
  DISTRIBUTION_ID: ${aws_cloudfront_distribution.website.id}
  LAMBDA_CODE_BUCKET: ${aws_s3_bucket.lambda_code.bucket}
  LAMBDA_FUNCTION_NAME: ${aws_lambda_function.booking_handler.function_name}

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
          VITE_API_ENDPOINT: "${aws_api_gateway_stage.booking_api_stage.invoke_url}/demo-request"

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: $${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: $${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: $${{ env.AWS_REGION }}

      - name: Package and Deploy Lambda
        run: |
          cd lambda
          npm install
          zip -r ../lambda-booking-handler.zip index.js node_modules/
          cd ..
          aws s3 cp lambda-booking-handler.zip s3://$${{ env.LAMBDA_CODE_BUCKET }}/lambda-booking-handler.zip
          aws lambda update-function-code --function-name $${{ env.LAMBDA_FUNCTION_NAME }} --s3-bucket $${{ env.LAMBDA_CODE_BUCKET }} --s3-key lambda-booking-handler.zip

      - name: Deploy to S3
        run: |
          aws s3 sync ./dist s3://$${{ env.S3_BUCKET }} --delete

      - name: Invalidate CloudFront Cache
        run: |
          aws cloudfront create-invalidation --distribution-id $${{ env.DISTRIBUTION_ID }} --paths "/*"
  EOT
  commit_message      = "Fix Lambda deployment - use npm install instead of npm ci"
  overwrite_on_create = true
}

# Store the CloudFront Distribution ID in the repo for reference
resource "github_repository_file" "distribution_id" {
  repository          = var.github_repo
  file                = "infra/distribution_id"
  content             = aws_cloudfront_distribution.website.id
  commit_message      = "Add CloudFront distribution ID"
  overwrite_on_create = true
}

# Store the API Gateway endpoints in the repo
resource "github_repository_file" "api_endpoints" {
  repository = var.github_repo
  file       = "infra/api_endpoints.json"
  content = jsonencode({
    book         = "${aws_api_gateway_stage.booking_api_stage.invoke_url}/book"
    demo_request = "${aws_api_gateway_stage.booking_api_stage.invoke_url}/demo-request"
    base_url     = aws_api_gateway_stage.booking_api_stage.invoke_url
  })
  commit_message      = "Add API Gateway endpoints"
  overwrite_on_create = true
}

# Create Lambda handler file in the repo
resource "github_repository_file" "lambda_handler" {
  repository          = var.github_repo
  file                = "lambda/index.js"
  content             = <<-EOF
const AWS = require('aws-sdk');
const dynamodb = new AWS.DynamoDB.DocumentClient();

const TABLE_NAME = process.env.TABLE_NAME;

exports.handler = async (event) => {
    console.log('Event:', JSON.stringify(event));
    
    // Enable CORS
    const headers = {
        'Access-Control-Allow-Origin': '*',
        'Access-Control-Allow-Headers': 'Content-Type,X-Amz-Date,Authorization,X-Api-Key,X-Amz-Security-Token',
        'Access-Control-Allow-Methods': 'OPTIONS,POST,GET'
    };
    
    // Handle OPTIONS request for CORS preflight
    if (event.httpMethod === 'OPTIONS') {
        return {
            statusCode: 200,
            headers: headers,
            body: JSON.stringify({ message: 'CORS preflight successful' })
        };
    }
    
    try {
        // Parse the request body
        const body = JSON.parse(event.body);
        const { name, email, company, message, preferredDate } = body;
        
        // Validate required fields
        if (!name || !email || !company) {
            return {
                statusCode: 400,
                headers: headers,
                body: JSON.stringify({ 
                    error: 'Missing required fields: name, email, and company are required' 
                })
            };
        }
        
        // Create booking item
        const bookingId = Date.now().toString(36) + Math.random().toString(36).substr(2);
        const timestamp = new Date().toISOString();
        
        const bookingItem = {
            id: bookingId,
            name: name,
            email: email.toLowerCase(),
            company: company,
            message: message || '',
            preferredDate: preferredDate || '',
            createdAt: timestamp,
            status: 'pending'
        };
        
        // Save to DynamoDB
        await dynamodb.put({
            TableName: TABLE_NAME,
            Item: bookingItem
        }).promise();
        
        console.log('Booking saved:', bookingId);
        
        return {
            statusCode: 200,
            headers: headers,
            body: JSON.stringify({
                success: true,
                message: 'Booking request received successfully',
                bookingId: bookingId
            })
        };
        
    } catch (error) {
        console.error('Error processing booking:', error);
        
        return {
            statusCode: 500,
            headers: headers,
            body: JSON.stringify({
                error: 'Internal server error',
                message: error.message
            })
        };
    }
};
EOF
  commit_message      = "Add Lambda handler for booking API"
  overwrite_on_create = true
}

# Create Lambda package.json
resource "github_repository_file" "lambda_package_json" {
  repository          = var.github_repo
  file                = "lambda/package.json"
  content             = <<-EOF
{
  "name": "cleversel-booking-api",
  "version": "1.0.0",
  "description": "Lambda handler for demo booking API",
  "main": "index.js",
  "dependencies": {
    "aws-sdk": "^2.1490.0"
  }
}
EOF
  commit_message      = "Add Lambda package.json"
  overwrite_on_create = true
}

# GitHub Secrets for AWS credentials
resource "github_actions_secret" "aws_access_key_id" {
  repository      = var.github_repo
  secret_name     = "AWS_ACCESS_KEY_ID"
  plaintext_value = var.aws_access_key_id
}

resource "github_actions_secret" "aws_secret_access_key" {
  repository      = var.github_repo
  secret_name     = "AWS_SECRET_ACCESS_KEY"
  plaintext_value = var.aws_secret_access_key
}
