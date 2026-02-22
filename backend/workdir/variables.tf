variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "eu-north-1"
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
  description = "S3 bucket name for the website"
  type        = string
  default     = "cleversel-landing-page"
}

variable "aws_access_key_id" {
  description = "AWS access key for GitHub Actions secret"
  type        = string
  sensitive   = true
  default     = ""
}

variable "aws_secret_access_key" {
  description = "AWS secret key for GitHub Actions secret"
  type        = string
  sensitive   = true
  default     = ""
}
