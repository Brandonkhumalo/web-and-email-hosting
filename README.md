# Hosting Platform

A self-hosted hosting platform that provides **email**, **static web hosting**, and **backend app hosting** for customers — all managed through a single Go API with AWS infrastructure.

## Features

### Email Hosting (Postfix + Dovecot + SES)
- Full email server with **phone/mobile access** (IMAP + SMTP)
- Works on iOS Mail, Gmail app, Outlook, Samsung Email, Thunderbird
- Virtual mailboxes stored in PostgreSQL (no Linux user accounts needed)
- Outbound email relayed through **Amazon SES** for high deliverability
- **DKIM signing** (OpenDKIM) per customer domain
- **SPF, DMARC** records auto-configured via Route53
- **Rspamd** spam filtering with greylisting
- Mail stored on **EFS** (survives server replacement)
- Auto-discover SRV records for easy phone setup

### Static Web Hosting (S3 + CloudFront)
- One-click static site provisioning per domain
- S3 for storage, CloudFront CDN for global delivery
- Free SSL via ACM (auto-renewing)
- Custom domain support with Route53 DNS

### Backend App Hosting (ECS + ALB)
- Docker container hosting on AWS ECS Fargate
- Shared ALB with host-header routing per customer
- Auto-scaling, health checks, CloudWatch logging
- Push Docker images to ECR to deploy

### Control Panel API (Go)
- JWT-authenticated REST API
- Domain management with Route53 hosted zones
- Automatic SSL certificate provisioning (ACM)
- SES domain verification for email sending
- Email account and alias CRUD

## Architecture

```
                    ┌─────────────┐
                    │   Customer  │
                    │   Phone/PC  │
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
         IMAP/SMTP    HTTPS (web)   HTTPS (api)
         993/587       443           443
              │            │            │
    ┌─────────▼──┐  ┌──────▼─────┐  ┌──▼──────────┐
    │   Dovecot  │  │ CloudFront │  │     ALB      │
    │   Postfix  │  │   (CDN)    │  │  (routing)   │
    │   (EC2)    │  └──────┬─────┘  └──┬───────────┘
    └─────┬──────┘         │           │
          │          ┌─────▼──┐   ┌────▼───────┐
          │          │   S3   │   │ ECS Fargate │
          │          │(static)│   │ (backends)  │
          │          └────────┘   └────┬────────┘
          │                            │
    ┌─────▼────────────────────────────▼──┐
    │         RDS PostgreSQL              │
    │  hostingplatform | mailserver DBs   │
    └─────────────────────────────────────┘
          │
    ┌─────▼──────┐
    │ Amazon SES │ (outbound email relay)
    └────────────┘
```

## Project Structure

```
hosting-platform/
├── control-panel/              # Go API
│   ├── main.go                 # Entry point, routes, auth handlers
│   ├── Dockerfile
│   ├── go.mod
│   └── internal/
│       ├── config/             # Environment variable loading
│       ├── models/             # Data structs + request/response DTOs
│       ├── database/           # PostgreSQL pools + migrations
│       ├── middleware/         # JWT authentication
│       ├── handlers/           # HTTP handlers (domains, sites, email)
│       └── services/           # AWS SDK integrations
│           ├── dns.go          # Route53
│           ├── hosting.go      # S3, CloudFront, ECS, ALB
│           ├── mail.go         # Password hashing
│           ├── ses.go          # SES domain verification
│           └── ssl.go          # ACM certificates
│
├── mail-server/                # Email server configs
│   ├── Dockerfile
│   ├── sql/init.sql            # Mail database schema
│   ├── postfix/                # SMTP config + SQL lookups
│   ├── dovecot/                # IMAP config + SQL auth
│   ├── opendkim/               # DKIM signing
│   ├── rspamd/                 # Spam filtering
│   └── scripts/
│       ├── setup.sh            # Full server provisioning
│       ├── add-domain.sh       # Per-domain email setup
│       └── backup.sh           # S3 backup with retention
│
├── infra/terraform/            # AWS infrastructure
│   ├── main.tf                 # Provider, ACM cert
│   ├── variables.tf            # All configurable vars
│   ├── outputs.tf              # IPs, ARNs, endpoints
│   ├── vpc.tf                  # VPC, subnets, NAT
│   ├── security-groups.tf      # All firewall rules
│   ├── mail-server.tf          # EC2 + EIP + EFS
│   ├── hosting.tf              # ECS cluster + ALB + ECR
│   ├── database.tf             # RDS PostgreSQL
│   ├── ses.tf                  # SES + SMTP credentials
│   └── route53.tf              # Platform DNS
│
├── dashboard/                  # Next.js admin panel (future)
├── docker-compose.yml          # Local dev environment
├── .env.example                # All environment variables
└── CLAUDE.md                   # AI assistant context
```

## Getting Started

### Prerequisites
- Go 1.22+
- Docker & Docker Compose
- AWS CLI configured with credentials
- Terraform 1.5+

### 1. Local Development

```bash
# Clone and set up environment
cp .env.example .env
# Edit .env with your values

# Start PostgreSQL + Redis
docker compose up -d

# Run the control panel API
cd control-panel
go mod tidy
go run main.go
```

The API will be available at `http://localhost:8080`.

### 2. Deploy Infrastructure

```bash
cd infra/terraform

# Create a terraform.tfvars file
cat > terraform.tfvars <<EOF
platform_domain  = "yourplatform.com"
db_password      = "your-secure-password"
mail_db_password = "your-mail-db-password"
ssh_key_name     = "your-ec2-keypair"
ssh_allowed_cidr = "YOUR_IP/32"
EOF

terraform init
terraform plan
terraform apply
```

### 3. Set Up Mail Server

After Terraform creates the EC2 instance:

```bash
# SSH into the mail server
ssh -i your-key.pem ubuntu@<mail-server-ip>

# Copy configs and run setup
sudo bash /opt/mail-server/scripts/setup.sh
```

### 4. Test Phone Email Access

```bash
# Create a test email account via the API
curl -X POST http://localhost:8080/api/email/accounts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "domain_id": 1,
    "username": "test",
    "password": "securepassword123"
  }'
```

Then on your phone:
- **Account Type**: IMAP
- **Incoming Server**: mail.yourplatform.com
- **Incoming Port**: 993 (SSL/TLS)
- **Outgoing Server**: mail.yourplatform.com
- **Outgoing Port**: 587 (STARTTLS)
- **Username**: test@yourcustomdomain.com
- **Password**: securepassword123

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/register` | Create customer account |
| POST | `/api/login` | Login, get JWT token |
| POST | `/api/domains` | Add domain + create Route53 zone |
| GET | `/api/domains` | List customer's domains |
| GET | `/api/domains/:id/nameservers` | Get NS records to set at registrar |
| POST | `/api/domains/:id/verify` | Check NS propagation |
| GET | `/api/domains/:id/dns` | Get all required DNS records |
| DELETE | `/api/domains/:id` | Remove domain + cleanup |
| POST | `/api/sites/static` | Create S3 + CloudFront site |
| POST | `/api/sites/backend` | Create ECS backend service |
| GET | `/api/sites` | List customer's sites |
| DELETE | `/api/sites/:id` | Tear down site |
| POST | `/api/email/accounts` | Create email mailbox |
| GET | `/api/email/domains/:id/accounts` | List email accounts |
| PUT | `/api/email/accounts/:id/password` | Change email password |
| DELETE | `/api/email/accounts/:id` | Delete email account |
| POST | `/api/email/aliases` | Create forwarding rule |
| GET | `/api/email/domains/:id/aliases` | List aliases |
| DELETE | `/api/email/aliases/:id` | Delete alias |

## DNS Records Per Customer Domain

When a customer adds `clientcompany.com`, these records are auto-created:

| Type | Name | Value | Purpose |
|------|------|-------|---------|
| MX | clientcompany.com | 10 mail.yourplatform.com | Routes incoming email |
| TXT | clientcompany.com | v=spf1 include:amazonses.com ... -all | SPF authorization |
| TXT | mail._domainkey.clientcompany.com | v=DKIM1; k=rsa; p=... | DKIM verification |
| TXT | _dmarc.clientcompany.com | v=DMARC1; p=quarantine; ... | DMARC policy |
| SRV | _imaps._tcp.clientcompany.com | 0 1 993 mail.yourplatform.com | Phone auto-discover |
| SRV | _submission._tcp.clientcompany.com | 0 1 587 mail.yourplatform.com | Phone auto-discover |
| A | clientcompany.com | → CloudFront (alias) | Static site |
| A | api.clientcompany.com | → ALB (alias) | Backend app |

## Build Order

| Phase | What | Depends On |
|-------|------|------------|
| 1 | Terraform: VPC, RDS, ECS cluster, ALB | Nothing |
| 2 | Terraform: EC2 mail server, EFS, SES, Route53 | Phase 1 |
| 3 | Mail server: setup.sh, Postfix, Dovecot configs | Phase 2 |
| 4 | Test phone access: create mailbox, configure phone | Phase 3 |
| 5 | Go control panel: domain + DNS management | Phase 1 RDS |
| 6 | Go control panel: static site provisioning | Phase 5 |
| 7 | Go control panel: email account management | Phase 3 + 5 |
| 8 | Go control panel: backend hosting (ECS) | Phase 5 |
| 9 | Next.js dashboard | Phase 5-8 API done |

## License

Private — Tishanyq Digital
