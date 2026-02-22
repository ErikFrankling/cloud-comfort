# Lambda function package
locals {
  lambda_zip_path = "${path.module}/lambda_function.zip"
}

# Generate Lambda function code
resource "local_file" "lambda_code" {
  content  = <<EOF
const AWS = require('aws-sdk');
const dynamoDB = new AWS.DynamoDB.DocumentClient();
const TABLE_NAME = process.env.TABLE_NAME;

exports.handler = async (event) => {
  console.log('Event:', JSON.stringify(event));
  
  const httpMethod = event.httpMethod || event.requestContext?.http?.method || 'GET';
  
  try {
    switch (httpMethod.toUpperCase()) {
      case 'GET':
        return await getItems();
      case 'POST':
        return await createItem(JSON.parse(event.body || '{}'));
      case 'DELETE':
        return await deleteItem(event.pathParameters?.id);
      default:
        return {
          statusCode: 405,
          headers: {
            'Content-Type': 'application/json',
            'Access-Control-Allow-Origin': '*'
          },
          body: JSON.stringify({ error: 'Method not allowed' })
        };
    }
  } catch (error) {
    console.error('Error:', error);
    return {
      statusCode: 500,
      headers: {
        'Content-Type': 'application/json',
        'Access-Control-Allow-Origin': '*'
      },
      body: JSON.stringify({ error: 'Internal server error', message: error.message })
    };
  }
};

async function getItems() {
  const params = {
    TableName: TABLE_NAME,
    Limit: 100
  };
  
  const result = await dynamoDB.scan(params).promise();
  
  return {
    statusCode: 200,
    headers: {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': '*'
    },
    body: JSON.stringify({ items: result.Items || [] })
  };
}

async function createItem(data) {
  const item = {
    id: data.id || Date.now().toString(),
    created_at: new Date().toISOString(),
    data: data.data || data
  };
  
  await dynamoDB.put({
    TableName: TABLE_NAME,
    Item: item
  }).promise();
  
  return {
    statusCode: 201,
    headers: {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': '*'
    },
    body: JSON.stringify({ message: 'Item created', item })
  };
}

async function deleteItem(id) {
  if (!id) {
    return {
      statusCode: 400,
      headers: {
        'Content-Type': 'application/json',
        'Access-Control-Allow-Origin': '*'
      },
      body: JSON.stringify({ error: 'ID is required' })
    };
  }
  
  await dynamoDB.delete({
    TableName: TABLE_NAME,
    Key: { id }
  }).promise();
  
  return {
    statusCode: 200,
    headers: {
      'Content-Type': 'application/json',
      'Access-Control-Allow-Origin': '*'
    },
    body: JSON.stringify({ message: 'Item deleted', id })
  };
}
EOF
  filename = "${path.module}/lambda_src/index.js"
}

# Lambda function code archive
data "archive_file" "lambda_zip" {
  type        = "zip"
  source_dir  = "${path.module}/lambda_src"
  output_path = local.lambda_zip_path
  depends_on  = [local_file.lambda_code]
}

# Lambda function
resource "aws_lambda_function" "api" {
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
  function_name    = "${var.project_name}-${var.environment}-api"
  role             = aws_iam_role.lambda_role.arn
  handler          = "index.handler"
  runtime          = "nodejs20.x"
  timeout          = 30
  memory_size      = 256

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
  name              = "/aws/lambda/${var.project_name}-${var.environment}-api"
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
