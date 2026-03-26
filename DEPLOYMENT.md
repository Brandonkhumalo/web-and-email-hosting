# Deployment Guide - Single EC2 Hosting Platform

Follow each phase in order.

---

## Cost: ~$22/month

| Resource | Monthly Cost |
|----------|-------------|
| EC2 t3.small | ~$15 |
| Elastic IP | $3.65 |
| EBS 30GB gp3 | ~$2.40 |
| Route53 (1 zone) | $0.50 |
| SES | Free tier |
| **Total** | **~$22** |

---

## Prerequisites

1. AWS Account
2. A domain name (e.g. tishanyqhost.com)
3. AWS CLI installed and configured
4. Terraform v1.5+ installed

---

## Phase 1: AWS Account Setup

### 1.1 Create IAM User

1. IAM > Users > Create User
2. Name: hosting-platform-admin
3. Attach policy: AdministratorAccess
4. Create access key for CLI
5. Save Access Key ID and Secret

### 1.2 Configure AWS CLI

    aws configure

### 1.3 Create EC2 Key Pair

    aws ec2 create-key-pair \
      --key-name hosting-mail-server \
      --query KeyMaterial \
      --output text > hosting-key.pem
    chmod 400 hosting-key.pem

### 1.4 Find Your IP (for SSH restriction)

    curl https://checkip.amazonaws.com

---

## Phase 2: Terraform Deploy

### 2.1 Create production.tfvars

File: infra/terraform/production.tfvars

    aws_region        = "us-east-1"
    environment       = "prod"
    platform_domain   = "yourplatform.com"
    db_password       = "ChangeMe_Str0ng!Pass1"
    ssh_key_name      = "hosting-mail-server"
    ssh_allowed_cidr  = "YOUR_IP/32"
    ses_region        = "us-east-1"
    ec2_instance_type = "t3.small"
    certbot_email     = "admin@yourplatform.com"

### 2.2 Deploy

    cd infra/terraform
    terraform init
    terraform plan -var-file=production.tfvars
    terraform apply -var-file=production.tfvars

Takes ~5 minutes. Creates: VPC, subnet, security group, EC2, Elastic IP, Route53 zone, SES.

### 2.3 Save Outputs

    terraform output -json > ../../terraform-outputs.json

Key outputs: ec2_public_ip, platform_nameservers, platform_zone_id

### 2.4 Point Domain to Route53

Set your domain nameservers to the 4 values from platform_nameservers output at your registrar. Wait 15-60 min.

---

## Phase 3: EC2 Setup

The user-data script auto-installs Docker, Nginx, PostgreSQL 15, and Certbot. Verify after ~5 min:

### 3.1 SSH In

    ssh -i hosting-key.pem ubuntu@EC2_PUBLIC_IP

### 3.2 Verify Services

    sudo systemctl status postgresql nginx docker

### 3.3 Verify Database

    sudo -u postgres psql -c "\l"
    # Should show: hostingplatform

---

## Phase 4: Deploy the Go API

### 4.1 Build the Binary

On your local machine:

    cd control-panel
    GOOS=linux GOARCH=amd64 go build -o bin/control-panel .

### 4.2 Create .env

Copy .env.example to .env and fill in values:

    PORT=8080
    ENVIRONMENT=production

    DB_HOST=localhost
    DB_PORT=5432
    DB_USER=postgres
    DB_PASSWORD=<db_password from tfvars>
    DB_NAME=hostingplatform
    DB_SSLMODE=disable

    AWS_REGION=us-east-1
    AWS_ACCOUNT_ID=<your 12-digit account ID>
    AWS_ACCESS_KEY_ID=<from IAM user>
    AWS_SECRET_ACCESS_KEY=<from IAM user>

    SES_REGION=us-east-1

    PLATFORM_DOMAIN=yourplatform.com
    PLATFORM_ZONE_ID=<terraform output: platform_zone_id>

    EC2_PUBLIC_IP=<terraform output: ec2_public_ip>

    NGINX_SITES_DIR=/etc/nginx/sites-available
    NGINX_ENABLED_DIR=/etc/nginx/sites-enabled
    SITES_ROOT_DIR=/var/www

    DOCKER_NETWORK=customer-net
    PORT_RANGE_START=10000
    PORT_RANGE_END=10999

    CERTBOT_EMAIL=admin@yourplatform.com

    JWT_SECRET=<generate: openssl rand -hex 32>
    JWT_EXPIRE_HOURS=24

### 4.3 Upload and Start

    scp -i hosting-key.pem bin/control-panel ubuntu@EC2_IP:/opt/hosting-platform/bin/
    scp -i hosting-key.pem .env ubuntu@EC2_IP:/opt/hosting-platform/.env

    ssh -i hosting-key.pem ubuntu@EC2_IP 'sudo systemctl start hosting-api'
    ssh -i hosting-key.pem ubuntu@EC2_IP 'sudo systemctl status hosting-api'

### 4.4 Get SSL for API

    ssh -i hosting-key.pem ubuntu@EC2_IP \
      'sudo certbot --nginx -d api.yourplatform.com --email admin@yourplatform.com --agree-tos --non-interactive'

### 4.5 Verify

    curl https://api.yourplatform.com/health
    # Should return: {"status":"ok"}

---

## Phase 5: SES Production Access

SES starts in sandbox mode. Request production access:

1. AWS Console > SES > Account dashboard
2. Request production access
3. Mail type: Transactional
4. Wait 1-2 business days

---

## Phase 6: Verification Checklist

- [ ] EC2 running with Docker, Nginx, PostgreSQL
- [ ] dig NS yourplatform.com returns Route53 nameservers
- [ ] API responds at https://api.yourplatform.com/health
- [ ] Can register (POST /api/register) and login
- [ ] Can create domain (POST /api/domains)
- [ ] Can create static site (POST /api/sites/static)
- [ ] Can create backend site (POST /api/sites/backend)
- [ ] Can create email account (POST /api/email/accounts)
- [ ] SES in production mode

---

## Scaling Guide

| Customers | Action | Cost |
|-----------|--------|------|
| 0-30 | t3.small as-is | ~$22/mo |
| 30-50 | Upgrade to t3.medium | ~$35/mo |
| 50-100 | t3.large, move PostgreSQL to RDS | ~$75/mo |
| 100+ | Add ALB + ECS (full architecture) | ~$120/mo |

Per-customer variable costs:
- Route53 zone: $0.50/mo per domain
- Static site: $0 (served from disk)
- Backend container: $0 (runs on EC2)
- SES: $0.10 per 1000 emails after free tier

### Upgrade EC2

    # Edit production.tfvars
    ec2_instance_type = "t3.medium"

    terraform apply -var-file=production.tfvars
    # Re-upload binary and .env, restart service

---

## Deploying Updates

    cd control-panel
    GOOS=linux GOARCH=amd64 go build -o bin/control-panel .
    scp -i hosting-key.pem bin/control-panel ubuntu@EC2_IP:/opt/hosting-platform/bin/
    ssh -i hosting-key.pem ubuntu@EC2_IP 'sudo systemctl restart hosting-api'

---

## Troubleshooting

**API won't start**: Check logs with journalctl -u hosting-api -f

**Nginx config error**: Run nginx -t on the EC2 to see which config file has an issue

**Certbot fails**: DNS must be propagated first. Check: dig A hostname

**Docker container won't start**: Run docker logs container-name

**Database connection refused**: Check PostgreSQL status: systemctl status postgresql
