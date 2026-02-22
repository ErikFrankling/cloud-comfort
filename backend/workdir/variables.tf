variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "eu-north-1"
}

variable "bucket_name" {
  description = "Base name for S3 bucket (will have random suffix added)"
  type        = string
  default     = "cleversel-website-bucket"
}

variable "github_token" {
  description = "GitHub token for managing repository"
  type        = string
  sensitive   = true
  default     = ""
}

variable "aws_access_key_id" {
  description = "AWS access key for GitHub Actions"
  type        = string
  sensitive   = true
  default     = ""
}

variable "aws_secret_access_key" {
  description = "AWS secret key for GitHub Actions"
  type        = string
  sensitive   = true
  default     = ""
}
