# ============================================================
# Security Group — Single SG for the EC2 instance
# Allows HTTP, HTTPS, SSH, SMTP, Submission, IMAPS
# ============================================================

resource "aws_security_group" "ec2" {
  name        = "hosting-ec2-sg"
  description = "EC2: HTTP(80), HTTPS(443), SSH(22), SMTP(25), Submission(587), IMAPS(993)"
  vpc_id      = aws_vpc.main.id

  # SSH — restricted to your IP
  ingress {
    description = "SSH access"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_allowed_cidr]
  }

  # HTTP — needed for Certbot validation and redirect to HTTPS
  ingress {
    description = "HTTP"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # HTTPS — all customer sites and the API
  ingress {
    description = "HTTPS"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # SMTP — inbound email from other mail servers
  ingress {
    description = "SMTP inbound"
    from_port   = 25
    to_port     = 25
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # SMTP Submission — authenticated sending from email clients/phones
  ingress {
    description = "SMTP submission"
    from_port   = 587
    to_port     = 587
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # IMAPS — email retrieval from email clients/phones
  ingress {
    description = "IMAPS"
    from_port   = 993
    to_port     = 993
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # All outbound traffic
  egress {
    description = "All outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = { Name = "hosting-ec2-sg" }
}
