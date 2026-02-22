# API Gateway REST API for booking endpoint
resource "aws_api_gateway_rest_api" "booking_api" {
  name        = "${var.bucket_name}-booking-api"
  description = "API for demo booking submissions"

  endpoint_configuration {
    types = ["REGIONAL"]
  }
}

# API Gateway Resource for /book endpoint
resource "aws_api_gateway_resource" "book" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  parent_id   = aws_api_gateway_rest_api.booking_api.root_resource_id
  path_part   = "book"
}

# API Gateway Resource for /demo-request endpoint
resource "aws_api_gateway_resource" "demo_request" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  parent_id   = aws_api_gateway_rest_api.booking_api.root_resource_id
  path_part   = "demo-request"
}

# API Gateway Method for POST /book
resource "aws_api_gateway_method" "book_post" {
  rest_api_id   = aws_api_gateway_rest_api.booking_api.id
  resource_id   = aws_api_gateway_resource.book.id
  http_method   = "POST"
  authorization = "NONE"
}

# API Gateway Method for POST /demo-request
resource "aws_api_gateway_method" "demo_request_post" {
  rest_api_id   = aws_api_gateway_rest_api.booking_api.id
  resource_id   = aws_api_gateway_resource.demo_request.id
  http_method   = "POST"
  authorization = "NONE"
}

# API Gateway Integration with Lambda for /book
resource "aws_api_gateway_integration" "lambda_integration" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.book.id
  http_method = aws_api_gateway_method.book_post.http_method

  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.booking_handler.invoke_arn
}

# API Gateway Integration with Lambda for /demo-request
resource "aws_api_gateway_integration" "demo_request_lambda_integration" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.demo_request.id
  http_method = aws_api_gateway_method.demo_request_post.http_method

  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.booking_handler.invoke_arn
}

# OPTIONS method for CORS preflight on /book
resource "aws_api_gateway_method" "book_options" {
  rest_api_id   = aws_api_gateway_rest_api.booking_api.id
  resource_id   = aws_api_gateway_resource.book.id
  http_method   = "OPTIONS"
  authorization = "NONE"
}

# OPTIONS method for CORS preflight on /demo-request
resource "aws_api_gateway_method" "demo_request_options" {
  rest_api_id   = aws_api_gateway_rest_api.booking_api.id
  resource_id   = aws_api_gateway_resource.demo_request.id
  http_method   = "OPTIONS"
  authorization = "NONE"
}

# Mock integration for OPTIONS /book
resource "aws_api_gateway_integration" "options_integration" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.book.id
  http_method = aws_api_gateway_method.book_options.http_method
  type        = "MOCK"

  request_templates = {
    "application/json" = "{\"statusCode\": 200}"
  }
}

# Mock integration for OPTIONS /demo-request
resource "aws_api_gateway_integration" "demo_request_options_integration" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.demo_request.id
  http_method = aws_api_gateway_method.demo_request_options.http_method
  type        = "MOCK"

  request_templates = {
    "application/json" = "{\"statusCode\": 200}"
  }
}

# OPTIONS method response /book - includes all CORS headers
resource "aws_api_gateway_method_response" "options_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.book.id
  http_method = aws_api_gateway_method.book_options.http_method
  status_code = "200"

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = true
    "method.response.header.Access-Control-Allow-Headers" = true
    "method.response.header.Access-Control-Allow-Methods" = true
  }

  response_models = {
    "application/json" = "Empty"
  }
}

# OPTIONS method response /demo-request - includes all CORS headers
resource "aws_api_gateway_method_response" "demo_request_options_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.demo_request.id
  http_method = aws_api_gateway_method.demo_request_options.http_method
  status_code = "200"

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = true
    "method.response.header.Access-Control-Allow-Headers" = true
    "method.response.header.Access-Control-Allow-Methods" = true
  }

  response_models = {
    "application/json" = "Empty"
  }
}

# OPTIONS integration response /book
resource "aws_api_gateway_integration_response" "options_integration_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.book.id
  http_method = aws_api_gateway_method.book_options.http_method
  status_code = aws_api_gateway_method_response.options_response.status_code

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = "'*'"
    "method.response.header.Access-Control-Allow-Headers" = "'Content-Type,X-Amz-Date,Authorization,X-Api-Key,X-Amz-Security-Token'"
    "method.response.header.Access-Control-Allow-Methods" = "'POST,OPTIONS'"
  }

  response_templates = {
    "application/json" = "{}"
  }

  depends_on = [aws_api_gateway_integration.options_integration]
}

# OPTIONS integration response /demo-request
resource "aws_api_gateway_integration_response" "demo_request_options_integration_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.demo_request.id
  http_method = aws_api_gateway_method.demo_request_options.http_method
  status_code = aws_api_gateway_method_response.demo_request_options_response.status_code

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = "'*'"
    "method.response.header.Access-Control-Allow-Headers" = "'Content-Type,X-Amz-Date,Authorization,X-Api-Key,X-Amz-Security-Token'"
    "method.response.header.Access-Control-Allow-Methods" = "'POST,OPTIONS'"
  }

  response_templates = {
    "application/json" = "{}"
  }

  depends_on = [aws_api_gateway_integration.demo_request_options_integration]
}

# POST method response for /book - includes CORS headers
resource "aws_api_gateway_method_response" "book_post_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.book.id
  http_method = aws_api_gateway_method.book_post.http_method
  status_code = "200"

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = true
    "method.response.header.Access-Control-Allow-Headers" = true
    "method.response.header.Access-Control-Allow-Methods" = true
  }
}

# POST method response for /demo-request - includes CORS headers
resource "aws_api_gateway_method_response" "demo_request_post_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.demo_request.id
  http_method = aws_api_gateway_method.demo_request_post.http_method
  status_code = "200"

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = true
    "method.response.header.Access-Control-Allow-Headers" = true
    "method.response.header.Access-Control-Allow-Methods" = true
  }
}

# POST integration response for /book
resource "aws_api_gateway_integration_response" "lambda_integration_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.book.id
  http_method = aws_api_gateway_method.book_post.http_method
  status_code = aws_api_gateway_method_response.book_post_response.status_code

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = "'*'"
    "method.response.header.Access-Control-Allow-Headers" = "'Content-Type,X-Amz-Date,Authorization,X-Api-Key,X-Amz-Security-Token'"
    "method.response.header.Access-Control-Allow-Methods" = "'POST,OPTIONS'"
  }

  depends_on = [aws_api_gateway_integration.lambda_integration]
}

# POST integration response for /demo-request
resource "aws_api_gateway_integration_response" "demo_request_lambda_integration_response" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id
  resource_id = aws_api_gateway_resource.demo_request.id
  http_method = aws_api_gateway_method.demo_request_post.http_method
  status_code = aws_api_gateway_method_response.demo_request_post_response.status_code

  response_parameters = {
    "method.response.header.Access-Control-Allow-Origin"  = "'*'"
    "method.response.header.Access-Control-Allow-Headers" = "'Content-Type,X-Amz-Date,Authorization,X-Api-Key,X-Amz-Security-Token'"
    "method.response.header.Access-Control-Allow-Methods" = "'POST,OPTIONS'"
  }

  depends_on = [aws_api_gateway_integration.demo_request_lambda_integration]
}

# Deploy the API Gateway
resource "aws_api_gateway_deployment" "booking_api_deployment" {
  rest_api_id = aws_api_gateway_rest_api.booking_api.id

  triggers = {
    redeployment = sha1(jsonencode([
      aws_api_gateway_resource.book.id,
      aws_api_gateway_method.book_post.id,
      aws_api_gateway_integration.lambda_integration.id,
      aws_api_gateway_method.book_options.id,
      aws_api_gateway_integration.options_integration.id,
      aws_api_gateway_method_response.options_response.id,
      aws_api_gateway_integration_response.options_integration_response.id,
      aws_api_gateway_method_response.book_post_response.id,
      aws_api_gateway_integration_response.lambda_integration_response.id,
      aws_api_gateway_resource.demo_request.id,
      aws_api_gateway_method.demo_request_post.id,
      aws_api_gateway_integration.demo_request_lambda_integration.id,
      aws_api_gateway_method.demo_request_options.id,
      aws_api_gateway_integration.demo_request_options_integration.id,
      aws_api_gateway_method_response.demo_request_options_response.id,
      aws_api_gateway_integration_response.demo_request_options_integration_response.id,
      aws_api_gateway_method_response.demo_request_post_response.id,
      aws_api_gateway_integration_response.demo_request_lambda_integration_response.id,
    ]))
  }

  lifecycle {
    create_before_destroy = true
  }
}

# API Gateway Stage
resource "aws_api_gateway_stage" "booking_api_stage" {
  deployment_id = aws_api_gateway_deployment.booking_api_deployment.id
  rest_api_id   = aws_api_gateway_rest_api.booking_api.id
  stage_name    = "prod"
}
