# ============================================================
# Terraform Variables
# Override via terraform.tfvars or -var flags
# ============================================================

variable "aws_region" {
  description = "AWS region for all resources"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (dev, staging, prod)"
  type        = string
  default     = "prod"
}

variable "platform_domain" {
  description = "Your platform's root domain (e.g., tishanyq.co.zw)"
  type        = string
}

variable "db_password" {
  description = "Password for the local PostgreSQL instance on EC2"
  type        = string
  sensitive   = true
}

variable "ssh_key_name" {
  description = "EC2 SSH key pair name"
  type        = string
}

variable "ssh_allowed_cidr" {
  description = "CIDR block allowed to SSH into the EC2 instance (your IP)"
  type        = string
  default     = "0.0.0.0/0"
}

variable "ses_region" {
  description = "AWS region for SES (may differ from main region)"
  type        = string
  default     = "us-east-1"
}

variable "ec2_instance_type" {
  description = "EC2 instance type (t3.small for 0-30 customers, t3.medium for 30-50)"
  type        = string
  default     = "t3.small"
}

variable "certbot_email" {
  description = "Email address for Let's Encrypt certificate notifications"
  type        = string
}

variable "mail_db_password" {
  description = "Password for the mailuser read-only PostgreSQL user (Postfix/Dovecot)"
  type        = string
  sensitive   = true
}

variable "roundcube_db_password" {
  description = "Password for the roundcube PostgreSQL user"
  type        = string
  sensitive   = true
}
