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

### 1.2 Open AWS CloudShell

CloudShell is a browser terminal built into the AWS Console. No local installs needed.

1. Log into the AWS Console
2. Click the **`>_`** icon in the top-right navigation bar
3. A terminal opens at the bottom of the page

Install Terraform in CloudShell:

    curl -fsSL https://releases.hashicorp.com/terraform/1.14.8/terraform_1.14.8_linux_amd64.zip -o tf.zip
    unzip tf.zip && sudo mv terraform /usr/local/bin/
    terraform --version

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

### 2.2 Set Your Region

CloudShell uses your AWS Console credentials automatically — no configuration needed.

Make sure your AWS Console is set to **af-south-1** (Cape Town) — click the region name in the top-right of the console and select it. CloudShell inherits this region.

**Why af-south-1?** That's Cape Town, South Africa — the closest AWS region to Zimbabwe. Your server will be faster for your users.

**Note:** If SES is not available in af-south-1, use `eu-west-1` (Ireland) instead.

### 2.3 Create an SSH Key Pair

Terraform requires a key pair to create the EC2 instance. Run in CloudShell:

    aws ec2 create-key-pair --key-name tishanyq-hosting-key --query KeyMaterial --output text > tishanyq-hosting-key.pem

You won't need this key for day-to-day use — you'll connect via the AWS Console browser terminal instead (Session Manager).

### 2.4 Find Your IP Address (Optional)

This restricts SSH access to your IP. If you plan to ONLY use the browser console and never SSH directly, you can set this to `0.0.0.0/0` or any placeholder. Otherwise:

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

**Do this AFTER Step 6 (terraform apply)** because you need your server's IP address first.

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

You need to create a config file **on your own computer** (not in AWS yet — you'll upload it later).

1. Open the project folder you downloaded/cloned earlier
2. Navigate to: `infra/terraform/` (so you're inside `hosting/infra/terraform/`)
3. Create a new text file in that folder and name it exactly: `production.tfvars`
4. Open `production.tfvars` in any text editor (Notepad, VS Code, etc.) and paste this inside:

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

## Step 5: Upload Files to CloudShell (5 min)

Now you need to upload the Terraform files and your config file to CloudShell so it can build your server.

First, create the folder structure in CloudShell:

    mkdir -p infra/terraform

Then click the **Actions > Upload file** button (top-right of the CloudShell panel) and upload these 9 files **one at a time** from the `infra/terraform/` folder on your computer:

| File | What it does |
|------|-------------|
| `main.tf` | AWS provider config |
| `variables.tf` | Input variable definitions |
| `outputs.tf` | Output values (IP, nameservers, etc.) |
| `vpc.tf` | VPC and subnet |
| `ec2.tf` | EC2 instance and Elastic IP |
| `security-groups.tf` | Firewall rules |
| `route53.tf` | DNS zone and records |
| `ses.tf` | Email sending service |
| `user-data.sh` | Server bootstrap script (installs all software) |

Also upload the `production.tfvars` file you created in Step 4.

CloudShell uploads files to your home directory (`~`). After uploading, move them all into the terraform folder:

    mv ~/main.tf ~/variables.tf ~/outputs.tf ~/vpc.tf ~/ec2.tf infra/terraform/ 2>/dev/null
    mv ~/security-groups.tf ~/route53.tf ~/ses.tf ~/user-data.sh infra/terraform/ 2>/dev/null
    mv ~/production.tfvars infra/terraform/ 2>/dev/null

**Note:** The `2>/dev/null` hides errors for files that were already moved. If CloudShell says a file "already exists and will not be overwritten" during upload, delete the old copy first with `rm ~/filename`, then re-upload it.

Verify everything is there:

    ls infra/terraform/

You should see all 10 files listed.

**Tip:** You can also zip the `infra/terraform/` folder on your computer, upload the single zip file, and unzip it:

    # Upload hosting-terraform.zip to CloudShell, then:
    unzip hosting-terraform.zip

---

## Step 6: Launch Your Server (10 min)

First, make sure Terraform is installed. If you set it up in Step 1.2, you're good. If not:

**CloudShell (Ubuntu-based):**

    curl -fsSL https://releases.hashicorp.com/terraform/1.14.8/terraform_1.14.8_linux_amd64.zip -o tf.zip
    unzip tf.zip && sudo mv terraform /usr/local/bin/

**Amazon Linux 2023 (if running commands directly on EC2):**

    sudo dnf install -y yum-utils
    sudo yum-config-manager --add-repo https://rpm.releases.hashicorp.com/AmazonLinux/hashicorp.repo
    sudo dnf install -y terraform

Verify it's installed:

    terraform --version

Then run these commands:

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

## Step 7: Point Your Domain to Your Server (5 min + waiting)

Go to your domain registrar's DNS settings and add the following records, replacing `YOUR_ELASTIC_IP` with the `elastic_ip` value from Terraform output:

| Type  | Name/Host         | Value                                      |
|-------|-------------------|--------------------------------------------|
| A     | @                 | YOUR_ELASTIC_IP                            |
| A     | api               | YOUR_ELASTIC_IP                            |
| A     | mail              | YOUR_ELASTIC_IP                            |
| A     | webmail           | YOUR_ELASTIC_IP                            |
| MX    | @                 | 10 mail.yourdomain.com                     |
| TXT   | @                 | v=spf1 include:amazonses.com ~all          |
| TXT   | _dmarc            | v=DMARC1; p=quarantine; rua=mailto:admin@yourdomain.com |
| SRV   | _imaps._tcp       | 0 1 993 mail.yourdomain.com                |
| SRV   | _submission._tcp  | 0 1 587 mail.yourdomain.com                |

> **Note:** Replace `yourdomain.com` with your actual domain (e.g., `tishanyq.co.zw`).

> **Note:** You also need to add the 3 DKIM CNAME records from Route53. Find them in **AWS Console > Route53 > Hosted Zones > your domain** — look for the `_domainkey` CNAME records and copy them to your registrar.

**DNS changes take 15-60 minutes to take effect.** You can check with:

    dig A yourdomain.com

When it returns your Elastic IP, you're good.

---

## Step 8: Verify Your Server is Running (5 min)

Connect to your server from the browser:

1. Go to **AWS Console > EC2 > Instances**
2. Select your `tishanyq-hosting` instance
3. Click **Connect** (top right)
4. Choose the **Session Manager** tab
5. Click **Connect** — a browser terminal opens

> **Note:** If Session Manager says "not connected", wait 5 minutes for the SSM agent to register. The instance needs outbound internet access (which it has) and the IAM role (which Terraform creates automatically).

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

## Step 9: Build and Deploy the API (10 min)

All commands in this step are run **on the server** via Session Manager (browser terminal). Connect to your instance first (see Step 8).

Once connected, switch to a proper shell (Session Manager starts as `ssm-user`):

    sudo -i
    cd /opt/tishanyq-hosting

### 8.1 Install Go and Build the API on the Server

    # Install Go (if not already installed by user-data)
    snap install go --classic

    # Clone your code or upload it (see below)
    # Option A: Clone from GitHub
    git clone https://github.com/YOUR_USERNAME/hosting.git /tmp/hosting-build
    cd /tmp/hosting-build/control-panel
    go build -o /opt/tishanyq-hosting/bin/control-panel .

    # Option B: If not using GitHub, upload via S3
    # From your local machine (or CloudShell): aws s3 cp control-panel/ s3://tishanyq-hosting-uploads/control-panel/ --recursive
    # On the server: aws s3 cp s3://tishanyq-hosting-uploads/control-panel/ /tmp/hosting-build/ --recursive
    # Then build as above

### 8.2 Create Your .env File

On the server, create the .env file:

    nano /opt/tishanyq-hosting/.env

Paste in these values (replace with your real values):

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

To generate the JWT_SECRET, run (on the server):

    openssl rand -hex 32

**Where do the values come from?**
- `DB_PASSWORD` = same as `db_password` in your production.tfvars
- `AWS_ACCOUNT_ID` = 12-digit number in top-right of AWS Console
- `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY` = from Step 2.1
- `PLATFORM_ZONE_ID` = from terraform output
- `EC2_PUBLIC_IP` = from terraform output

### 8.3 Start the API

    systemctl start hosting-api

Check it started:

    systemctl status hosting-api

### 8.4 Get SSL Certificates

This gives your sites HTTPS (the green lock in the browser). DNS must be propagated first (Step 7).

Run on the server:

    sudo certbot --nginx \
      -d api.tishanyq.co.zw \
      -d mail.tishanyq.co.zw \
      -d webmail.tishanyq.co.zw \
      --email admin@tishanyq.co.zw \
      --agree-tos --non-interactive

### 8.5 Update Mail Server SSL

After getting the certificates, tell Postfix and Dovecot to use them (on the server):

    postconf -e "smtpd_tls_cert_file=/etc/letsencrypt/live/mail.tishanyq.co.zw/fullchain.pem"
    postconf -e "smtpd_tls_key_file=/etc/letsencrypt/live/mail.tishanyq.co.zw/privkey.pem"
    systemctl restart postfix dovecot

### 8.6 Grant Mail Database Permissions

The mail server needs to read email accounts from the database (on the server):

    sudo -u postgres psql -d hostingplatform -c "GRANT SELECT ON domains, email_accounts, email_aliases TO mailuser;"

### 8.7 Verify Everything Works

Open a new browser tab and visit:

    https://api.tishanyq.co.zw/health

Should return: `{"status":"ok"}`

---

## Step 10: Host Your First Website (5 min)

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

The response shows `nameservers`. Since you already pointed your domain to Route53 in Step 7, this should already be working.

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

Connect to your server via Session Manager (Step 8), then create your website files:

    sudo nano /var/www/tishanyq.co.zw/index.html

Or upload files via S3:

    # From your local machine (or CloudShell):
    aws s3 cp my-website/ s3://tishanyq-hosting-uploads/sites/tishanyq.co.zw/ --recursive

    # On the server (via Session Manager):
    sudo aws s3 cp s3://tishanyq-hosting-uploads/sites/tishanyq.co.zw/ /var/www/tishanyq.co.zw/ --recursive

### 9.6 Get SSL for Your Website

On the server (via Session Manager):

    sudo certbot --nginx -d tishanyq.co.zw --email admin@tishanyq.co.zw --agree-tos --non-interactive

Your website is now live at `https://tishanyq.co.zw`

---

## Step 11: Add Customer Websites

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

    # Get SSL (on the server via Session Manager)
    sudo certbot --nginx -d customerdomain.co.zw --agree-tos --non-interactive

    # Upload their files via S3
    # Local/CloudShell: aws s3 cp customer-files/ s3://tishanyq-hosting-uploads/sites/customerdomain.co.zw/ --recursive
    # Server: sudo aws s3 cp s3://tishanyq-hosting-uploads/sites/customerdomain.co.zw/ /var/www/customerdomain.co.zw/ --recursive

---

## Step 12: Set Up Email (after AWS approvals)

Once AWS approves your SES production access AND port 25 unblock, you can set up email.

### 11.1 Configure SES SMTP Credentials for Postfix

Get your SES credentials:

    cd infra/terraform
    terraform output -raw ses_smtp_access_key
    terraform output -raw ses_smtp_secret_key

Connect to the server via Session Manager and update Postfix:

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

## Step 13: Verification Checklist

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

    # After the instance restarts, connect via Session Manager and rebuild
    cd /tmp/hosting-build/control-panel && go build -o /opt/tishanyq-hosting/bin/control-panel .
    sudo systemctl restart hosting-api

---

## Deploying Code Updates

When you update the Go API code, connect to the server via Session Manager and:

    sudo -i
    cd /tmp/hosting-build/control-panel
    git pull
    go build -o /opt/tishanyq-hosting/bin/control-panel .
    systemctl restart hosting-api

---

## Troubleshooting

**"API won't start"**
Connect via Session Manager and check the logs:

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
