variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "github_owner" {
  description = "GitHub repository owner"
  type        = string
  default     = "ErikFrankling"
}

variable "github_repo" {
  description = "GitHub repository name"
  type        = string
  default     = "Cleversel-Website"
}

variable "bucket_name" {
  description = "S3 bucket name for website hosting"
  type        = string
  default     = "cleversel-website-bucket"
}

variable "aws_access_key_id" {
  description = "AWS Access Key ID for GitHub Actions"
  type        = string
  sensitive   = true
  default     = ""
}

variable "aws_secret_access_key" {
  description = "AWS Secret Access Key for GitHub Actions"
  type        = string
  sensitive   = true
  default     = ""
}
