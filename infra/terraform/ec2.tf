# ============================================================
# EC2 — Single instance running the entire platform
# Nginx + Go API + PostgreSQL + Docker + Certbot
# Postfix + Dovecot + OpenDKIM + Roundcube (mail server)
# ============================================================

# Latest Ubuntu 22.04 LTS AMI
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# --- Elastic IP ---
# Permanent public IP for DNS records and consistent access

resource "aws_eip" "main" {
  domain = "vpc"

  tags = { Name = "tishanyq-hosting-eip" }
}

# --- IAM Role for EC2 (SSM Session Manager access) ---
# Allows you to connect to the instance from the AWS Console browser terminal
# instead of needing SSH keys locally

resource "aws_iam_role" "ec2" {
  name = "tishanyq-hosting-ec2-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.ec2.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "ec2" {
  name = "tishanyq-hosting-ec2-profile"
  role = aws_iam_role.ec2.name
}

# --- EC2 Instance ---

resource "aws_instance" "main" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.ec2_instance_type
  key_name               = var.ssh_key_name
  subnet_id              = aws_subnet.public_a.id
  vpc_security_group_ids = [aws_security_group.ec2.id]
  iam_instance_profile   = aws_iam_instance_profile.ec2.name

  root_block_device {
    volume_size = 40
    volume_type = "gp3"
    encrypted   = true
  }

  user_data = templatefile("${path.module}/user-data.sh", {
    platform_domain       = var.platform_domain
    db_password           = var.db_password
    mail_db_password      = var.mail_db_password
    roundcube_db_password = var.roundcube_db_password
    certbot_email         = var.certbot_email
    ses_region            = var.ses_region
  })

  tags = { Name = "tishanyq-hosting" }

  lifecycle {
    # Set to true once in production to prevent accidental deletion
    prevent_destroy = false
  }
}

# --- Attach Elastic IP to EC2 ---

resource "aws_eip_association" "main" {
  instance_id   = aws_instance.main.id
  allocation_id = aws_eip.main.id
}
