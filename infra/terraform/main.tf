# ============================================================
# Hosting Platform — Terraform Root
# Single-EC2 architecture: Nginx + Go API + PostgreSQL + Docker
# ============================================================

terraform {
  required_version = ">= 1.5"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  # Store Terraform state in S3 (create this bucket manually first)
  backend "s3" {
    bucket         = "yourplatform-terraform-state"
    key            = "hosting-platform/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "terraform-locks"
    encrypt        = true
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "hosting-platform"
      ManagedBy   = "terraform"
      Environment = var.environment
    }
  }
}

# NOTE: SSL is handled by Certbot (Let's Encrypt) on the EC2 instance.
# No ACM certificates needed since there's no ALB.
# Certbot runs: certbot --nginx -d {hostname} for each customer site.
