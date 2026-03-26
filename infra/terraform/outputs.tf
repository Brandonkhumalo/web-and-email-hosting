# ============================================================
# Terraform Outputs
# Values you'll need after terraform apply
# ============================================================

output "ec2_public_ip" {
  description = "Elastic IP of the EC2 instance"
  value       = aws_eip.main.public_ip
}

output "ec2_instance_id" {
  description = "EC2 instance ID"
  value       = aws_instance.main.id
}

output "platform_nameservers" {
  description = "Route53 nameservers — set these at your domain registrar"
  value       = aws_route53_zone.platform.name_servers
}

output "platform_zone_id" {
  description = "Route53 hosted zone ID for the platform domain"
  value       = aws_route53_zone.platform.zone_id
}

output "ses_smtp_iam_user" {
  description = "IAM user for SES SMTP credentials"
  value       = aws_iam_user.ses_smtp.name
}

output "vpc_id" {
  description = "VPC ID"
  value       = aws_vpc.main.id
}

output "public_subnet_id" {
  description = "Public subnet ID"
  value       = aws_subnet.public_a.id
}

output "ses_smtp_access_key" {
  description = "SES SMTP access key ID (for Postfix sasl_passwd)"
  value       = aws_iam_access_key.ses_smtp.id
  sensitive   = true
}

output "ses_smtp_secret_key" {
  description = "SES SMTP secret key (convert to SMTP password for Postfix)"
  value       = aws_iam_access_key.ses_smtp.secret
  sensitive   = true
}
