# Hosting Platform

## Project Overview
Self-hosted hosting platform on a single EC2 instance. Provides static website hosting (Nginx), backend app hosting (Docker containers), full email hosting (Postfix+Dovecot+Roundcube), and DNS management (Route53) for customers. Each customer gets their own domain with sites, email accounts, and webmail.

## Architecture
- **Single EC2**: Runs everything — Nginx, Go API, PostgreSQL, Docker, Certbot, Postfix, Dovecot, Roundcube
- **Control Panel API**: Go (Gin) — manages domains, sites, email accounts via AWS SDK + local services
- **Reverse Proxy**: Nginx — serves static sites, proxies backend containers, serves Roundcube webmail
- **Customer Backends**: Docker containers on the same EC2, managed via Docker CLI
- **Email**: Postfix (SMTP) + Dovecot (IMAP) + SES (outbound relay) + Roundcube (webmail)
- **SSL**: Let's Encrypt via Certbot (--nginx plugin)
- **DNS**: Route53 — platform zone + per-customer hosted zones
- **Database**: Single PostgreSQL instance on EC2: `hostingplatform`
- **Infrastructure**: Terraform — VPC, EC2, EIP, Route53, SES

## Project Structure
```
hosting/
├── control-panel/        # Go API (Gin + pgx + AWS SDK)
│   ├── main.go
│   ├── Dockerfile
│   └── internal/
│       ├── config/       # Environment variable loading
│       ├── database/     # PostgreSQL connection + migrations
│       ├── models/       # Data structures + request DTOs
│       ├── middleware/    # JWT authentication
│       ├── handlers/     # HTTP request handlers (domains, sites, email)
│       └── services/     # Business logic (nginx, docker, dns, ses, ssl, hosting, mail)
├── mail-server/          # Mail server config templates
│   ├── postfix/          # Postfix SMTP config + SQL lookups
│   ├── dovecot/          # Dovecot IMAP config + SQL auth
│   ├── opendkim/         # DKIM signing config
│   ├── roundcube/        # Roundcube webmail config
│   └── nginx/            # Nginx server block for webmail
├── infra/terraform/      # AWS infrastructure
│   ├── ec2.tf            # Single EC2 instance + Elastic IP
│   ├── user-data.sh      # EC2 bootstrap (all software installation)
│   ├── vpc.tf            # VPC + single public subnet
│   ├── security-groups.tf # SG: 22, 80, 443, 25, 587, 993
│   ├── route53.tf        # Platform DNS (A, MX, SPF, DMARC, SRV)
│   ├── ses.tf            # Email sending service
│   └── main.tf           # Provider config
├── docker-compose.yml    # Local dev: PostgreSQL + Redis
└── .env.example          # All required environment variables
```

## Key Commands

### Local Development
```bash
docker compose up -d
cd control-panel && go run main.go
```

### Production Deployment
```bash
cd control-panel && GOOS=linux GOARCH=amd64 go build -o bin/control-panel .
scp -i key.pem bin/control-panel ubuntu@<EC2_IP>:/opt/hosting-platform/bin/
scp -i key.pem .env ubuntu@<EC2_IP>:/opt/hosting-platform/.env
ssh -i key.pem ubuntu@<EC2_IP> 'sudo systemctl restart hosting-api'
```

## Conventions
- All secrets via environment variables, never hardcoded
- Go API uses `internal/` package layout
- Nginx reverse-proxies to Go API and customer Docker containers
- Customer backends: Docker containers with `--restart unless-stopped`, ports 10000-10999
- Email accounts stored in `email_accounts` table with SHA512-CRYPT passwords
- Dovecot/Postfix read from same database via read-only `mailuser` PostgreSQL role
- Maildir storage: /var/mail/vhosts/{domain}/{user}/Maildir/
- Outbound email: Postfix → SES relay for deliverability
- Webmail: Roundcube at webmail.{domain}, served by Nginx + PHP-FPM
- Per-customer AWS resources (Route53 zones, SES identities) created dynamically by the Go API
- Terraform manages shared infrastructure only

## API Routes
- POST /api/register, POST /api/login (public)
- POST /api/domains, GET /api/domains, POST /api/domains/:id/verify, DELETE /api/domains/:id
- POST /api/sites/static, POST /api/sites/backend, GET /api/sites, DELETE /api/sites/:id
- POST /api/email/accounts, GET /api/email/domains/:domain_id/accounts
- PUT /api/email/accounts/:id/password, DELETE /api/email/accounts/:id
- POST /api/email/aliases, GET /api/email/domains/:domain_id/aliases, DELETE /api/email/aliases/:id

## Email Access
- **Phone**: IMAP mail.{domain}:993 (SSL/TLS), SMTP mail.{domain}:587 (STARTTLS)
- **Webmail**: https://webmail.{domain}
- **Works on**: iOS Mail, Gmail, Outlook, Samsung Email, Thunderbird

## Cost
~$30/month base (t3.medium EC2 + Elastic IP + Route53 + EBS 40GB)
