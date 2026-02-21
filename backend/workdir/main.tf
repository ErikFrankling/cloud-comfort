terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = "eu-north-1"
}

# ---- VARIABLES ----
variable "terraform-s3-bucket-test" {
  description = "S3 bucket name"
  type        = string
}

variable "environment" {
  description = "Environment tag"
  type        = string
  default     = "dev"
}

# ---- RESOURCES ----
resource "aws_s3_bucket" "my_bucket" {
  bucket = var.bucket_name
}

# ---- OUTPUTS ----
output "bucket_id" {
  value = aws_s3_bucket.my_bucket.id
}
