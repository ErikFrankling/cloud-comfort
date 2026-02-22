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
