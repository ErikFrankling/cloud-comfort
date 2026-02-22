# Lambda function - uses S3 deployment package
# Note: The deployment package must be uploaded to S3 separately
resource "aws_lambda_function" "api" {
  function_name = "${var.project_name}-${var.environment}-${var.name_suffix}-api"
  role          = aws_iam_role.lambda_role.arn
  handler       = "index.handler"
  runtime       = "nodejs20.x"
  timeout       = 30
  memory_size   = 256

  # Placeholder - upload your deployment package to this S3 location
  s3_bucket = aws_s3_bucket.storage.id
  s3_key    = "lambda/deployment-package.zip"

  environment {
    variables = {
      TABLE_NAME = aws_dynamodb_table.main.name
    }
  }

  tracing_config {
    mode = "Active"
  }

  depends_on = [aws_cloudwatch_log_group.lambda_logs]

  tags = var.tags
}

# CloudWatch Log Group for Lambda
resource "aws_cloudwatch_log_group" "lambda_logs" {
  name              = "/aws/lambda/${var.project_name}-${var.environment}-${var.name_suffix}-api"
  retention_in_days = 14

  tags = var.tags
}

# Lambda function URL for HTTP access
resource "aws_lambda_function_url" "api" {
  function_name      = aws_lambda_function.api.function_name
  authorization_type = "NONE"

  cors {
    allow_credentials = false
    allow_origins     = ["*"]
    allow_methods     = ["GET", "POST", "DELETE"]
    allow_headers     = ["content-type", "x-amz-date", "authorization", "x-api-key"]
    max_age           = 86400
  }
}
