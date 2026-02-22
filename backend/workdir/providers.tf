terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    github = {
      source  = "integrations/github"
      version = "~> 6.2.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

provider "github" {
  owner = var.github_owner
}
