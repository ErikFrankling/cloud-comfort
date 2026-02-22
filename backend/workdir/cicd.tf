# --- GitHub Actions Secrets for AWS Credentials ---
resource "github_actions_secret" "aws_access_key" {
  repository      = var.github_repo
  secret_name     = "AWS_ACCESS_KEY_ID"
  plaintext_value = var.aws_access_key_id
}

resource "github_actions_secret" "aws_secret_key" {
  repository      = var.github_repo
  secret_name     = "AWS_SECRET_ACCESS_KEY"
  plaintext_value = var.aws_secret_access_key
}

# --- GitHub Actions Workflow ---
resource "github_repository_file" "deploy_workflow" {
  repository          = var.github_repo
  branch              = "main"
  file                = ".github/workflows/deploy.yml"
  overwrite_on_create = true

  content = <<-YAML
name: Deploy Cleversel Landing Page

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

      - name: Configure AWS Credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: $${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: $${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: $${{ env.AWS_REGION }}

      - name: Deploy to S3
        run: |
          aws s3 sync ./dist s3://$${{ env.S3_BUCKET }} --delete

      - name: Invalidate CloudFront Cache
        run: |
          aws cloudfront create-invalidation --distribution-id $${{ env.DISTRIBUTION_ID }} --paths "/*"
YAML
}
