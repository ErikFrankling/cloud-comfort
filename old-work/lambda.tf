# DynamoDB Table for demo bookings
resource "aws_dynamodb_table" "bookings" {
  name         = "${var.bucket_name}-bookings-${random_id.bucket_suffix.hex}"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

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
    Name = "${var.bucket_name}-bookings-${random_id.bucket_suffix.hex}"
  }
}

# S3 Bucket for Lambda code
resource "aws_s3_bucket" "lambda_code" {
  bucket = "${var.bucket_name}-lambda-code-${random_id.bucket_suffix.hex}"
}

# Bootstrap placeholder for Lambda code - will be replaced by GitHub Actions
resource "aws_s3_object" "lambda_placeholder" {
  bucket  = aws_s3_bucket.lambda_code.bucket
  key     = "lambda-booking-handler.zip"
  content = "placeholder"
}

# IAM Role for Lambda
resource "aws_iam_role" "lambda_role" {
  name = "${var.bucket_name}-lambda-role-${random_id.bucket_suffix.hex}"

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

# IAM Policy for Lambda to access DynamoDB and CloudWatch
resource "aws_iam_role_policy" "lambda_policy" {
  name = "${var.bucket_name}-lambda-policy-${random_id.bucket_suffix.hex}"
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

# IAM Policy for Lambda to read from S3
resource "aws_iam_role_policy" "lambda_s3_policy" {
  name = "${var.bucket_name}-lambda-s3-policy-${random_id.bucket_suffix.hex}"
  role = aws_iam_role.lambda_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject"
        ]
        Resource = "${aws_s3_bucket.lambda_code.arn}/*"
      }
    ]
  })
}

# Lambda function for handling bookings
# Initial placeholder will be replaced by GitHub Actions on first push
resource "aws_lambda_function" "booking_handler" {
  function_name = "${var.bucket_name}-booking-handler-${random_id.bucket_suffix.hex}"
  role          = aws_iam_role.lambda_role.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
  timeout       = 10

  # Read placeholder from S3 bucket - will be updated by GitHub Actions
  s3_bucket = aws_s3_bucket.lambda_code.bucket
  s3_key    = aws_s3_object.lambda_placeholder.key

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.bookings.name
    }
  }

  depends_on = [aws_iam_role_policy.lambda_policy, aws_s3_object.lambda_placeholder]

  # Allow GitHub Actions to update the code without Terraform trying to revert it
  lifecycle {
    ignore_changes = [s3_key, s3_bucket, source_code_hash]
  }
}

# CloudWatch Log Group for Lambda
resource "aws_cloudwatch_log_group" "lambda_logs" {
  name              = "/aws/lambda/${aws_lambda_function.booking_handler.function_name}"
  retention_in_days = 7
}

# Lambda permission for API Gateway
resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.booking_handler.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_rest_api.booking_api.execution_arn}/*/*"
}
