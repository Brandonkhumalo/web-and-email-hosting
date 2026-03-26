# Setup Guide - Hosting Platform

This guide gets you from zero to live websites and email accounts on AWS.
Written for beginners. Follow each step in order.

---

## What You Get

- Host websites for your domain and customer domains
- Full email accounts (login on phone + webmail in browser)
- All on one server for ~$30/month

---

## What You Can Do RIGHT AWAY vs What Needs AWS Approval

| Feature | Available Immediately | Needs AWS Approval |
|---------|----------------------|-------------------|
| Host websites | Yes | No |
| Host backend apps | Yes | No |
| SSL certificates (HTTPS) | Yes | No |
| DNS management | Yes | No |
| Create email accounts | Yes | No |
| Login to email (phone/webmail) | Yes | No |
| SEND email | No | SES production access (1-2 days) |
| RECEIVE email from others | No | Port 25 unblock (1-3 days) |

**Start building websites immediately.** Request the email approvals in parallel — by the time your sites are live, email will be ready too.

---

## Step 1: Things You Need Before Starting (15 min)

### 1.1 AWS Account

Go to https://aws.amazon.com and create an account. You need a credit card.

### 1.2 Install Tools on Your Computer

You need 3 tools. Install all of them:

**AWS CLI** (talks to AWS from your terminal):
- Download: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html

**Terraform** (creates your server automatically):
- Download: https://developer.hashicorp.com/terraform/install

**Go** (builds the control panel API):
- Download: https://go.dev/dl/

After installing, verify they work:

    aws --version
    terraform --version
    go version

### 1.3 Own Your Domain

You need a domain name (e.g., tishanyq.co.zw). Buy one from any registrar if you don't have one yet.

---

## Step 2: AWS Account Setup (10 min)

### 2.1 Create an IAM User

Do NOT use your root AWS account for everything. Create a separate user:

1. Go to AWS Console > IAM > Users > Create User
2. User name: `hosting-admin`
3. Click "Attach policies directly"
4. Search for `AdministratorAccess` and check the box
5. Click Create User
6. Click on the user > Security credentials > Create access key
7. Choose "Command Line Interface (CLI)"
8. Save the **Access Key ID** and **Secret Access Key** — you need both later

### 2.2 Configure AWS CLI

Open your terminal and run:

    aws configure

It will ask you 4 things:

    AWS Access Key ID: (paste your access key)
    AWS Secret Access Key: (paste your secret key)
    Default region name: af-south-1
    Default output format: json

**Why af-south-1?** That's Cape Town, South Africa — the closest AWS region to Zimbabwe. Your server will be faster for your users.

**Note:** If SES is not available in af-south-1, use `eu-west-1` (Ireland) instead.

### 2.3 Create an SSH Key Pair

This is like a password to log into your server. Run:

    aws ec2 create-key-pair --key-name hosting-key --query KeyMaterial --output text > hosting-key.pem

On Mac/Linux, also run:

    chmod 400 hosting-key.pem

Keep this file safe. If you lose it, you cannot log into your server.

### 2.4 Find Your IP Address

You need this to restrict who can SSH into your server:

    curl https://checkip.amazonaws.com

Write down the IP (e.g., `102.68.45.12`).

---

## Step 3: Request Email Approvals (DO THIS NOW — takes 1-3 days)

Do this before anything else because it takes time. You can set up websites while waiting.

### 3.1 Request SES Production Access

SES (Simple Email Service) is how your server sends email. By default it is in "sandbox mode" which means you can only send email to yourself.

1. Go to AWS Console > SES > Account Dashboard
2. Click "Request production access"
3. Fill in:
   - Mail type: **Transactional**
   - Website URL: your domain (e.g., https://tishanyq.co.zw)
   - Use case description: Write something like: "We are a hosting company providing email hosting services for our business clients. Each client gets email accounts on their domain for business communication."
4. Submit and wait 1-2 business days

### 3.2 Request Port 25 Unblock

Port 25 is how other mail servers deliver email TO you. AWS blocks it by default.

**Do this AFTER Step 5 (terraform apply)** because you need your server's IP address first.

1. Go to AWS Console > Support > Create Case
2. Choose "Service limit increase"
3. Limit type: **EC2 Email Sending**
4. Fill in:
   - Region: af-south-1
   - Elastic IP: (your EC2 IP from terraform output)
   - Reverse DNS: `mail.yourdomain.com`
   - Use case: "Running a legitimate email hosting service for business clients"
5. Submit and wait 1-3 business days

---

## Step 4: Create Your Server Config File (5 min)

Create a file called `production.tfvars` inside the `infra/terraform/` folder:

    aws_region            = "af-south-1"
    environment           = "prod"
    platform_domain       = "tishanyq.co.zw"
    db_password           = "MyStr0ng!DbPass1"
    mail_db_password      = "MyStr0ng!MailPass2"
    roundcube_db_password = "MyStr0ng!RcPass3"
    ssh_key_name          = "hosting-key"
    ssh_allowed_cidr      = "102.68.45.12/32"
    ses_region            = "af-south-1"
    ec2_instance_type     = "t3.medium"
    certbot_email         = "admin@tishanyq.co.zw"

**Replace** the values:
- `platform_domain` = your actual domain
- `ssh_allowed_cidr` = your IP from step 2.4, with `/32` at the end
- Passwords = make up 3 different strong passwords (avoid @, ", ' characters)
- `certbot_email` = your email for SSL certificate notifications

---

## Step 5: Launch Your Server (10 min)

Run these commands:

    cd infra/terraform
    terraform init
    terraform plan -var-file=production.tfvars
    terraform apply -var-file=production.tfvars

Terraform will show you what it's going to create. Type `yes` to confirm.

Wait ~5 minutes. When done, it prints outputs. Save them:

    terraform output -json > ../../terraform-outputs.json

The important values:
- `ec2_public_ip` — your server's IP address
- `platform_nameservers` — 4 DNS servers to set at your registrar
- `platform_zone_id` — your Route53 zone ID

**Now go back and do Step 3.2** (Request Port 25 Unblock) using the `ec2_public_ip`.

---

## Step 6: Point Your Domain to Your Server (5 min + waiting)

Go to wherever you bought your domain (e.g., registrar website) and change the **nameservers** to the 4 values from `platform_nameservers`.

Example — you would replace the existing nameservers with something like:

    ns-123.awsdns-45.com
    ns-678.awsdns-90.net
    ns-111.awsdns-22.org
    ns-333.awsdns-44.co.uk

**This takes 15-60 minutes to take effect.** You can check with:

    dig NS tishanyq.co.zw

When it returns the Route53 nameservers, you're good.

---

## Step 7: Verify Your Server is Running (5 min)

SSH into your server:

    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP

Check that everything installed correctly:

    sudo systemctl status postgresql
    sudo systemctl status nginx
    sudo systemctl status docker
    sudo systemctl status postfix
    sudo systemctl status dovecot

All should say "active (running)". If postfix or dovecot are not running yet, that's OK — they may need SSL certificates first.

Check the database was created:

    sudo -u postgres psql -c "\l"

You should see `hostingplatform` and `roundcubemail` in the list.

---

## Step 8: Build and Deploy the API (10 min)

### 8.1 Build the API on Your Computer

    cd control-panel
    GOOS=linux GOARCH=amd64 go build -o bin/control-panel .

This creates a file called `bin/control-panel` that runs on Linux (your EC2).

### 8.2 Create Your .env File

Copy `.env.example` to `.env` and fill in the real values:

    PORT=8080
    ENVIRONMENT=production

    DB_HOST=localhost
    DB_PORT=5432
    DB_USER=postgres
    DB_PASSWORD=MyStr0ng!DbPass1
    DB_NAME=hostingplatform
    DB_SSLMODE=disable

    AWS_REGION=af-south-1
    AWS_ACCOUNT_ID=123456789012
    AWS_ACCESS_KEY_ID=AKIA...
    AWS_SECRET_ACCESS_KEY=wJalr...

    SES_REGION=af-south-1

    PLATFORM_DOMAIN=tishanyq.co.zw
    PLATFORM_ZONE_ID=Z1234567890

    EC2_PUBLIC_IP=13.245.67.89

    MAIL_SERVER_HOST=mail.tishanyq.co.zw

    NGINX_SITES_DIR=/etc/nginx/sites-available
    NGINX_ENABLED_DIR=/etc/nginx/sites-enabled
    SITES_ROOT_DIR=/var/www

    DOCKER_NETWORK=customer-net
    PORT_RANGE_START=10000
    PORT_RANGE_END=10999

    CERTBOT_EMAIL=admin@tishanyq.co.zw

    JWT_SECRET=paste-a-random-64-char-string-here
    JWT_EXPIRE_HOURS=24

To generate the JWT_SECRET, run:

    openssl rand -hex 32

**Where do the values come from?**
- `DB_PASSWORD` = same as `db_password` in your production.tfvars
- `AWS_ACCOUNT_ID` = 12-digit number in top-right of AWS Console
- `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` = from Step 2.1
- `PLATFORM_ZONE_ID` = from terraform output
- `EC2_PUBLIC_IP` = from terraform output

### 8.3 Upload to Your Server

    scp -i hosting-key.pem bin/control-panel ubuntu@YOUR_EC2_IP:/opt/hosting-platform/bin/
    scp -i hosting-key.pem .env ubuntu@YOUR_EC2_IP:/opt/hosting-platform/.env

### 8.4 Start the API

    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP 'sudo systemctl start hosting-api'

Check it started:

    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP 'sudo systemctl status hosting-api'

### 8.5 Get SSL Certificates

This gives your sites HTTPS (the green lock in the browser). DNS must be propagated first (Step 6).

    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP

Then on the server:

    sudo certbot --nginx \
      -d api.tishanyq.co.zw \
      -d mail.tishanyq.co.zw \
      -d webmail.tishanyq.co.zw \
      --email admin@tishanyq.co.zw \
      --agree-tos --non-interactive

### 8.6 Update Mail Server SSL

After getting the certificates, tell Postfix and Dovecot to use them:

    sudo postconf -e "smtpd_tls_cert_file=/etc/letsencrypt/live/mail.tishanyq.co.zw/fullchain.pem"
    sudo postconf -e "smtpd_tls_key_file=/etc/letsencrypt/live/mail.tishanyq.co.zw/privkey.pem"
    sudo systemctl restart postfix dovecot

### 8.7 Grant Mail Database Permissions

The mail server needs to read email accounts from the database:

    sudo -u postgres psql -d hostingplatform -c "GRANT SELECT ON domains, email_accounts, email_aliases TO mailuser;"

### 8.8 Verify Everything Works

    curl https://api.tishanyq.co.zw/health

Should return: `{"status":"ok"}`

---

## Step 9: Host Your First Website (5 min)

You can do this RIGHT NOW — no email approvals needed.

### 9.1 Register Yourself as a Customer

    curl -X POST https://api.tishanyq.co.zw/api/register \
      -H "Content-Type: application/json" \
      -d '{
        "email": "brandon@gmail.com",
        "password": "mypassword123",
        "name": "Brandon Khumalo",
        "company": "Tishanyq Digital"
      }'

The response contains a `token`. Copy it — you need it for every request below.

### 9.2 Add Your Domain

    curl -X POST https://api.tishanyq.co.zw/api/domains \
      -H "Authorization: Bearer YOUR_TOKEN_HERE" \
      -H "Content-Type: application/json" \
      -d '{"name": "tishanyq.co.zw"}'

The response shows `nameservers`. Since you already pointed your domain to Route53 in Step 6, this should already be working.

### 9.3 Verify the Domain

    curl -X POST https://api.tishanyq.co.zw/api/domains/1/verify \
      -H "Authorization: Bearer YOUR_TOKEN_HERE"

If it says `"verified": true`, your domain is active.

### 9.4 Create a Static Website

    curl -X POST https://api.tishanyq.co.zw/api/sites/static \
      -H "Authorization: Bearer YOUR_TOKEN_HERE" \
      -H "Content-Type: application/json" \
      -d '{"domain_id": 1, "subdomain": "@"}'

This creates your site. The response tells you the `site_root` path (e.g., `/var/www/tishanyq.co.zw/`).

### 9.5 Upload Your Website Files

SSH into the server and put your HTML/CSS/JS files in the site root:

    scp -i hosting-key.pem -r my-website/* ubuntu@YOUR_EC2_IP:/var/www/tishanyq.co.zw/

Or SSH in and edit directly:

    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP
    sudo nano /var/www/tishanyq.co.zw/index.html

### 9.6 Get SSL for Your Website

    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP
    sudo certbot --nginx -d tishanyq.co.zw --email admin@tishanyq.co.zw --agree-tos --non-interactive

Your website is now live at `https://tishanyq.co.zw`

---

## Step 10: Add Customer Websites

Repeat the same process for each customer domain. The customer must:

1. Own their domain
2. Change their nameservers to the Route53 values from the API response

You do:

    # Add the customer domain
    curl -X POST https://api.tishanyq.co.zw/api/domains \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"name": "customerdomain.co.zw"}'

    # Wait for them to point nameservers, then verify
    curl -X POST https://api.tishanyq.co.zw/api/domains/2/verify \
      -H "Authorization: Bearer YOUR_TOKEN"

    # Create their site
    curl -X POST https://api.tishanyq.co.zw/api/sites/static \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"domain_id": 2, "subdomain": "@"}'

    # Get SSL
    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP
    sudo certbot --nginx -d customerdomain.co.zw --agree-tos --non-interactive

    # Upload their files
    scp -i hosting-key.pem -r customer-files/* ubuntu@YOUR_EC2_IP:/var/www/customerdomain.co.zw/

---

## Step 11: Set Up Email (after AWS approvals)

Once AWS approves your SES production access AND port 25 unblock, you can set up email.

### 11.1 Configure SES SMTP Credentials for Postfix

Get your SES credentials:

    cd infra/terraform
    terraform output -raw ses_smtp_access_key
    terraform output -raw ses_smtp_secret_key

SSH into the server and update Postfix:

    ssh -i hosting-key.pem ubuntu@YOUR_EC2_IP
    sudo nano /etc/postfix/sasl_passwd

Change the line to (replace with your actual credentials):

    [email-smtp.af-south-1.amazonaws.com]:587 YOUR_ACCESS_KEY:YOUR_SECRET_KEY

Save the file, then run:

    sudo postmap /etc/postfix/sasl_passwd
    sudo chmod 600 /etc/postfix/sasl_passwd /etc/postfix/sasl_passwd.db
    sudo systemctl restart postfix

### 11.2 Create Email Accounts

Create email accounts using the API. Each account gets a real mailbox that users can log into.

    # Create brandon@tishanyq.co.zw
    curl -X POST https://api.tishanyq.co.zw/api/email/accounts \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{
        "domain_id": 1,
        "username": "brandon",
        "password": "EmailPass123!",
        "display_name": "Brandon Khumalo"
      }'

The response includes `phone_settings` — the IMAP/SMTP server info.

Create more accounts by changing the username:

    # info@tishanyq.co.zw
    curl -X POST https://api.tishanyq.co.zw/api/email/accounts \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"domain_id": 1, "username": "info", "password": "InfoPass123!", "display_name": "Tishanyq Info"}'

    # admin@tishanyq.co.zw
    curl -X POST https://api.tishanyq.co.zw/api/email/accounts \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"domain_id": 1, "username": "admin", "password": "AdminPass123!", "display_name": "Tishanyq Admin"}'

    # Repeat for: support@, sales@, hr@, finance@, etc.

### 11.3 Set Up Email on Phones

Each user adds the account to their phone email app manually:

**iPhone (iOS Mail):**
1. Settings > Mail > Accounts > Add Account > Other
2. Add Mail Account
3. Fill in name, email (brandon@tishanyq.co.zw), password, description
4. Choose IMAP
5. Incoming mail server: `mail.tishanyq.co.zw`, port `993`, SSL ON
6. Outgoing mail server: `mail.tishanyq.co.zw`, port `587`, STARTTLS ON
7. Username: full email address (brandon@tishanyq.co.zw)

**Android (Gmail app):**
1. Gmail > Settings > Add Account > Other
2. Enter email address > Manual Setup
3. Choose IMAP
4. Incoming: server `mail.tishanyq.co.zw`, port `993`, security SSL/TLS
5. Outgoing: server `mail.tishanyq.co.zw`, port `587`, security STARTTLS
6. Username: full email address

**Android (Samsung Email):**
1. Samsung Email > Add Account > Other
2. Enter email and password
3. Choose IMAP
4. Same settings as Gmail above

**Outlook (desktop/mobile):**
1. Add Account > Manual Setup > IMAP
2. Incoming: `mail.tishanyq.co.zw`, port 993, encryption SSL/TLS
3. Outgoing: `mail.tishanyq.co.zw`, port 587, encryption STARTTLS
4. Username: full email address

### 11.4 Use Webmail (Browser)

Go to: `https://webmail.tishanyq.co.zw`

Log in with:
- Username: full email (e.g., brandon@tishanyq.co.zw)
- Password: the password you set when creating the account

### 11.5 Set Up Email for Customer Domains

Same process. Create accounts on their domain:

    curl -X POST https://api.tishanyq.co.zw/api/email/accounts \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{
        "domain_id": 2,
        "username": "john",
        "password": "JohnPass123!",
        "display_name": "John Smith"
      }'

Their phone settings will use `mail.tishanyq.co.zw` as the server — the same server handles all domains.

### 11.6 Create Email Forwarding (Optional)

Forward emails from one address to another:

    # Forward info@tishanyq.co.zw to brandon@tishanyq.co.zw
    curl -X POST https://api.tishanyq.co.zw/api/email/aliases \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{
        "domain_id": 1,
        "source": "info",
        "destination": "brandon@tishanyq.co.zw"
      }'

### 11.7 Change an Email Password

    curl -X PUT https://api.tishanyq.co.zw/api/email/accounts/1/password \
      -H "Authorization: Bearer YOUR_TOKEN" \
      -H "Content-Type: application/json" \
      -d '{"password": "NewPassword123!"}'

The user will need to update the password on their phone too.

---

## Step 12: Verification Checklist

### Websites (should work immediately)
- [ ] https://tishanyq.co.zw loads your website
- [ ] https://api.tishanyq.co.zw/health returns {"status":"ok"}
- [ ] Customer websites load on their domains

### Email (works after AWS approvals)
- [ ] Can create email accounts via API
- [ ] Can log into webmail at https://webmail.tishanyq.co.zw
- [ ] Can send email from webmail to a Gmail account
- [ ] Can receive email from Gmail to your@tishanyq.co.zw
- [ ] Can set up phone email and send/receive
- [ ] Customer email accounts work the same way

---

## Cost Breakdown

| Resource | Monthly Cost |
|----------|-------------|
| EC2 t3.medium | ~$30 |
| Elastic IP | $3.65 |
| EBS 40GB gp3 | ~$3.20 |
| Route53 (per zone) | $0.50 |
| SES | Free (62,000 emails/mo) |
| SSL certificates | Free (Let's Encrypt) |
| **Total (your domain)** | **~$37/mo** |
| **Each customer domain** | **+$0.50/mo** |

---

## Scaling

| Customers | What to Do | Cost |
|-----------|-----------|------|
| 1-30 | Nothing, t3.medium handles it | ~$37/mo |
| 30-50 | Upgrade to t3.large | ~$60/mo |
| 50-100 | Move database to RDS | ~$75/mo |
| 100+ | Move to multi-server setup | ~$120/mo |

To upgrade the EC2:

    # Edit production.tfvars
    ec2_instance_type = "t3.large"

    cd infra/terraform
    terraform apply -var-file=production.tfvars

    # Re-upload your binary and restart
    scp -i hosting-key.pem bin/control-panel ubuntu@EC2_IP:/opt/hosting-platform/bin/
    ssh -i hosting-key.pem ubuntu@EC2_IP 'sudo systemctl restart hosting-api'

---

## Deploying Code Updates

When you update the Go API code:

    cd control-panel
    GOOS=linux GOARCH=amd64 go build -o bin/control-panel .
    scp -i hosting-key.pem bin/control-panel ubuntu@EC2_IP:/opt/hosting-platform/bin/
    ssh -i hosting-key.pem ubuntu@EC2_IP 'sudo systemctl restart hosting-api'

---

## Troubleshooting

**"API won't start"**
Check the logs:

    ssh -i hosting-key.pem ubuntu@EC2_IP
    sudo journalctl -u hosting-api -f

**"Website shows 444 or blank page"**
The Nginx config might not have loaded. Check:

    sudo nginx -t
    sudo systemctl reload nginx

**"Certbot fails"**
DNS must be propagated first. Check:

    dig A tishanyq.co.zw

If it doesn't return your EC2 IP, wait longer for DNS propagation.

**"Can't send email"**
- Check if SES is still in sandbox: AWS Console > SES > Account Dashboard
- Check Postfix logs: `sudo tail -f /var/log/mail.log`

**"Can't receive email"**
- Check if port 25 is unblocked: AWS support ticket status
- Check Postfix is running: `sudo systemctl status postfix`
- Check MX record exists: `dig MX tishanyq.co.zw`

**"Can't log into email on phone"**
- Make sure you're using the full email as username (brandon@tishanyq.co.zw, not just brandon)
- IMAP port must be 993 with SSL/TLS
- SMTP port must be 587 with STARTTLS
- Check Dovecot logs: `sudo tail -f /var/log/dovecot.log`

**"Webmail shows error"**
- Check PHP-FPM is running: `sudo systemctl status php*-fpm`
- Check Nginx webmail config: `sudo nginx -t`
- Check Roundcube logs: `sudo cat /var/www/roundcube/logs/errors.log`
