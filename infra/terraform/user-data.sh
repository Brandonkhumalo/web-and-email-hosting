#!/bin/bash
# ============================================================
# EC2 Bootstrap Script
# Installs: Docker, Nginx, PostgreSQL 15, Certbot
#           Postfix, Dovecot, OpenDKIM, Roundcube (mail server)
# Configures: databases, systemd services, Docker network
# ============================================================
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
exec > /var/log/user-data.log 2>&1

echo "=== Starting EC2 bootstrap ==="

# --- System updates ---
apt-get update && apt-get upgrade -y

# --- Install Docker ---
apt-get install -y ca-certificates curl gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" > /etc/apt/sources-list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
systemctl enable docker && systemctl start docker

# --- Install Nginx ---
apt-get install -y nginx
systemctl enable nginx

# --- Install PostgreSQL 15 ---
apt-get install -y postgresql-15 postgresql-client-15
systemctl enable postgresql

# --- Install Certbot ---
apt-get install -y certbot python3-certbot-nginx

# --- Install Mail Server Stack ---
# Preconfigure Postfix to avoid interactive prompts
echo "postfix postfix/main_mailer_type select Internet Site" | debconf-set-selections
echo "postfix postfix/mailname string ${platform_domain}" | debconf-set-selections
apt-get install -y postfix postfix-pgsql
apt-get install -y dovecot-core dovecot-imapd dovecot-lmtpd dovecot-pgsql
apt-get install -y opendkim opendkim-tools

# --- Install PHP + Roundcube dependencies ---
apt-get install -y php-fpm php-pgsql php-mbstring php-xml php-intl php-zip php-curl php-gd php-imagick

# --- Install useful tools ---
apt-get install -y jq unzip htop

# --- Set hostname ---
hostnamectl set-hostname mail.${platform_domain}

# ============================================================
# PostgreSQL Configuration
# ============================================================

sudo -u postgres psql -c "ALTER USER postgres WITH PASSWORD '${db_password}';"
sudo -u postgres psql -c "CREATE DATABASE hostingplatform;"

# Create read-only user for Postfix/Dovecot mail lookups
sudo -u postgres psql -c "CREATE USER mailuser WITH PASSWORD '${mail_db_password}';"
sudo -u postgres psql -d hostingplatform -c "GRANT CONNECT ON DATABASE hostingplatform TO mailuser;"
sudo -u postgres psql -d hostingplatform -c "GRANT USAGE ON SCHEMA public TO mailuser;"

# Create Roundcube database and user
sudo -u postgres psql -c "CREATE DATABASE roundcubemail;"
sudo -u postgres psql -c "CREATE USER roundcube WITH PASSWORD '${roundcube_db_password}';"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE roundcubemail TO roundcube;"
sudo -u postgres psql -d roundcubemail -c "GRANT ALL ON SCHEMA public TO roundcube;"

# Update pg_hba.conf to allow md5 auth from localhost
PG_HBA=$(find /etc/postgresql -name pg_hba.conf)
sed -i 's/local\s\+all\s\+all\s\+peer/local   all   all   md5/' "$PG_HBA"
sed -i 's|host\s\+all\s\+all\s\+127.0.0.1/32\s\+scram-sha-256|host   all   all   127.0.0.1/32   md5|' "$PG_HBA"
systemctl restart postgresql

# ============================================================
# Directory Setup
# ============================================================

mkdir -p /var/www
mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
mkdir -p /opt/hosting-platform/bin

# Create vmail user for mailbox storage (UID/GID 5000)
groupadd -g 5000 vmail || true
useradd -u 5000 -g vmail -s /sbin/nologin -d /var/mail/vhosts -m vmail || true
mkdir -p /var/mail/vhosts
chown -R vmail:vmail /var/mail/vhosts
chmod 770 /var/mail/vhosts

# Create Docker network for customer containers
docker network create customer-net || true

# Create OpenDKIM directories
mkdir -p /etc/opendkim /etc/opendkim/keys
touch /etc/opendkim/key.table /etc/opendkim/signing.table
chown -R opendkim:opendkim /etc/opendkim

# ============================================================
# Nginx Configuration
# ============================================================

rm -f /etc/nginx/sites-enabled/default

# Catch-all server block (returns 444 for unknown hosts)
cat > /etc/nginx/sites-available/default-catch-all.conf << 'NGINXEOF'
server {
    listen 80 default_server;
    listen 443 ssl default_server;
    server_name _;
    ssl_certificate /etc/nginx/ssl/default.crt;
    ssl_certificate_key /etc/nginx/ssl/default.key;
    return 444;
}
NGINXEOF

mkdir -p /etc/nginx/ssl
openssl req -x509 -nodes -days 3650 -newkey rsa:2048 \
  -keyout /etc/nginx/ssl/default.key \
  -out /etc/nginx/ssl/default.crt \
  -subj "/CN=_"

ln -sf /etc/nginx/sites-available/default-catch-all.conf /etc/nginx/sites-enabled/

# API reverse proxy
cat > /etc/nginx/sites-available/api.${platform_domain}.conf << 'APIEOF'
server {
    listen 80;
    server_name api.${platform_domain};
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
APIEOF
ln -sf /etc/nginx/sites-available/api.${platform_domain}.conf /etc/nginx/sites-enabled/

# Roundcube webmail
cat > /etc/nginx/sites-available/webmail.${platform_domain}.conf << 'WEBMAILEOF'
server {
    listen 80;
    server_name webmail.${platform_domain};
    root /var/www/roundcube;
    index index.php;
    location / {
        try_files $uri $uri/ /index.php?$args;
    }
    location ~ \.php$ {
        include fastcgi_params;
        fastcgi_pass unix:/run/php/php-fpm.sock;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_intercept_errors on;
    }
    location ~ /\. { deny all; }
    location ~ ^/(config|temp|logs)/ { deny all; }
}
WEBMAILEOF
ln -sf /etc/nginx/sites-available/webmail.${platform_domain}.conf /etc/nginx/sites-enabled/

nginx -t && systemctl reload nginx

# ============================================================
# Postfix Configuration
# ============================================================

# Backup original config
cp /etc/postfix/main.cf /etc/postfix/main.cf.bak

cat > /etc/postfix/main.cf << POSTFIXEOF
myhostname = mail.${platform_domain}
mydomain = ${platform_domain}
myorigin = \$mydomain
mydestination = localhost
inet_interfaces = all
inet_protocols = ipv4

virtual_mailbox_domains = pgsql:/etc/postfix/sql/virtual_domains.cf
virtual_mailbox_maps = pgsql:/etc/postfix/sql/virtual_mailboxes.cf
virtual_alias_maps = pgsql:/etc/postfix/sql/virtual_aliases.cf
virtual_transport = lmtp:unix:private/dovecot-lmtp
virtual_uid_maps = static:5000
virtual_gid_maps = static:5000
virtual_mailbox_base = /var/mail/vhosts

relayhost = [email-smtp.${ses_region}.amazonaws.com]:587
smtp_sasl_auth_enable = yes
smtp_sasl_password_maps = hash:/etc/postfix/sasl_passwd
smtp_sasl_security_options = noanonymous
smtp_tls_security_level = encrypt
smtp_tls_CAfile = /etc/ssl/certs/ca-certificates.crt

smtpd_tls_security_level = may
smtpd_tls_protocols = !SSLv2, !SSLv3, !TLSv1, !TLSv1.1
smtpd_sasl_type = dovecot
smtpd_sasl_path = private/auth
smtpd_sasl_auth_enable = yes
smtpd_recipient_restrictions = permit_sasl_authenticated, permit_mynetworks, reject_unauth_destination

milter_protocol = 6
milter_default_action = accept
smtpd_milters = inet:localhost:8891
non_smtpd_milters = inet:localhost:8891

message_size_limit = 26214400
mailbox_size_limit = 0
POSTFIXEOF

# Add submission service to master.cf
cat >> /etc/postfix/master.cf << 'MASTEREOF'

submission inet n       -       y       -       -       smtpd
  -o syslog_name=postfix/submission
  -o smtpd_tls_security_level=encrypt
  -o smtpd_sasl_auth_enable=yes
  -o smtpd_sasl_type=dovecot
  -o smtpd_sasl_path=private/auth
  -o smtpd_recipient_restrictions=permit_sasl_authenticated,reject
  -o milter_macro_daemon_name=ORIGINATING
MASTEREOF

# PostgreSQL lookup configs
mkdir -p /etc/postfix/sql

cat > /etc/postfix/sql/virtual_domains.cf << SQLEOF
user = mailuser
password = ${mail_db_password}
hosts = localhost
dbname = hostingplatform
query = SELECT name FROM domains WHERE name = '%s' AND email_enabled = TRUE AND active = TRUE
SQLEOF

cat > /etc/postfix/sql/virtual_mailboxes.cf << SQLEOF
user = mailuser
password = ${mail_db_password}
hosts = localhost
dbname = hostingplatform
query = SELECT maildir FROM email_accounts WHERE email = '%s' AND active = TRUE AND mail_enabled = TRUE
SQLEOF

cat > /etc/postfix/sql/virtual_aliases.cf << SQLEOF
user = mailuser
password = ${mail_db_password}
hosts = localhost
dbname = hostingplatform
query = SELECT destination FROM email_aliases WHERE source = '%s' AND active = TRUE
SQLEOF

chmod 640 /etc/postfix/sql/*.cf
chown root:postfix /etc/postfix/sql/*.cf

# SES SMTP credentials placeholder (fill after terraform apply)
cat > /etc/postfix/sasl_passwd << 'SESEOF'
[email-smtp.us-east-1.amazonaws.com]:587 SES_ACCESS_KEY:SES_SECRET_KEY
SESEOF
postmap /etc/postfix/sasl_passwd
chmod 600 /etc/postfix/sasl_passwd /etc/postfix/sasl_passwd.db

# ============================================================
# Dovecot Configuration
# ============================================================

cat > /etc/dovecot/dovecot.conf << DOVECOTEOF
protocols = imap lmtp
listen = *

ssl = required
ssl_min_protocol = TLSv1.2

mail_location = maildir:/var/mail/vhosts/%d/%n/Maildir
mail_privileged_group = vmail
first_valid_uid = 5000
last_valid_uid = 5000

auth_mechanisms = plain login
disable_plaintext_auth = yes

passdb {
  driver = sql
  args = /etc/dovecot/dovecot-sql.conf.ext
}
userdb {
  driver = sql
  args = /etc/dovecot/dovecot-sql.conf.ext
}

service lmtp {
  unix_listener /var/spool/postfix/private/dovecot-lmtp {
    group = postfix
    mode = 0600
    user = postfix
  }
}

service auth {
  unix_listener /var/spool/postfix/private/auth {
    mode = 0660
    user = postfix
    group = postfix
  }
}

service imap-login {
  inet_listener imap { port = 0 }
  inet_listener imaps { port = 993; ssl = yes }
}

namespace inbox {
  inbox = yes
  separator = /
  mailbox Drafts { auto = subscribe; special_use = \Drafts }
  mailbox Sent { auto = subscribe; special_use = \Sent }
  mailbox Trash { auto = subscribe; special_use = \Trash }
  mailbox Junk { auto = subscribe; special_use = \Junk }
  mailbox Archive { auto = no; special_use = \Archive }
}

mail_plugins = \$mail_plugins quota
protocol imap {
  mail_plugins = \$mail_plugins imap_quota
  mail_max_userip_connections = 20
  imap_idle_notify_interval = 2 mins
}
plugin { quota = maildir:User quota; quota_grace = 10%% }

log_path = /var/log/dovecot.log
info_log_path = /var/log/dovecot-info.log
DOVECOTEOF

cat > /etc/dovecot/dovecot-sql.conf.ext << DOVSQLEOF
driver = pgsql
connect = host=localhost dbname=hostingplatform user=mailuser password=${mail_db_password}

password_query = SELECT email as user, password, \
  'maildir:/var/mail/vhosts/%d/%n/Maildir' as userdb_mail, \
  5000 as userdb_uid, 5000 as userdb_gid \
  FROM email_accounts \
  WHERE email = '%u' AND active = TRUE AND mail_enabled = TRUE

user_query = SELECT \
  'maildir:/var/mail/vhosts/%d/%n/Maildir' as home, \
  5000 as uid, 5000 as gid, \
  CONCAT('*:bytes=', quota) as quota_rule \
  FROM email_accounts \
  WHERE email = '%u' AND active = TRUE AND mail_enabled = TRUE

default_pass_scheme = SHA512-CRYPT
DOVSQLEOF

chmod 600 /etc/dovecot/dovecot-sql.conf.ext

# ============================================================
# OpenDKIM Configuration
# ============================================================

cat > /etc/opendkim.conf << 'DKIMEOF'
Syslog          yes
UMask           007
Socket          inet:8891@localhost
PidFile         /run/opendkim/opendkim.pid
OversignHeaders From
Canonicalization relaxed/simple
Mode            sv
SubDomains      no
AutoRestart     yes
AutoRestartRate 10/1M
Background      yes
DNSTimeout      5
SignatureAlgorithm rsa-sha256
KeyTable        refile:/etc/opendkim/key.table
SigningTable    refile:/etc/opendkim/signing.table
ExternalIgnoreList refile:/etc/opendkim/trusted.hosts
InternalHosts   refile:/etc/opendkim/trusted.hosts
DKIMEOF

cat > /etc/opendkim/trusted.hosts << 'TRUSTEOF'
127.0.0.1
localhost
TRUSTEOF

# ============================================================
# Install Roundcube
# ============================================================

ROUNDCUBE_VERSION="1.6.9"
cd /tmp
wget -q "https://github.com/roundcube/roundcubemail/releases/download/$${ROUNDCUBE_VERSION}/roundcubemail-$${ROUNDCUBE_VERSION}-complete.tar.gz"
tar xzf "roundcubemail-$${ROUNDCUBE_VERSION}-complete.tar.gz"
mv "roundcubemail-$${ROUNDCUBE_VERSION}" /var/www/roundcube
chown -R www-data:www-data /var/www/roundcube

# Initialize Roundcube database schema
sudo -u postgres psql -d roundcubemail < /var/www/roundcube/SQL/postgres.initial.sql

# Generate random DES key for Roundcube
ROUNDCUBE_DES_KEY=$(openssl rand -hex 12)

cat > /var/www/roundcube/config/config.inc.php << RCEOF
<?php
\$config = [];
\$config['db_dsnw'] = 'pgsql://roundcube:${roundcube_db_password}@localhost/roundcubemail';
\$config['imap_host'] = 'ssl://localhost:993';
\$config['imap_conn_options'] = ['ssl' => ['verify_peer' => false, 'verify_peer_name' => false]];
\$config['smtp_host'] = 'tls://localhost:587';
\$config['smtp_port'] = 587;
\$config['smtp_user'] = '%u';
\$config['smtp_pass'] = '%p';
\$config['smtp_conn_options'] = ['ssl' => ['verify_peer' => false, 'verify_peer_name' => false]];
\$config['product_name'] = 'Webmail';
\$config['des_key'] = '$ROUNDCUBE_DES_KEY';
\$config['plugins'] = ['archive', 'zipdownload'];
\$config['skin'] = 'elastic';
\$config['language'] = 'en_US';
\$config['enable_installer'] = false;
\$config['support_url'] = '';
\$config['ip_check'] = true;
\$config['session_lifetime'] = 30;
RCEOF

chown www-data:www-data /var/www/roundcube/config/config.inc.php

# ============================================================
# Systemd Service for Go API
# ============================================================

cat > /etc/systemd/system/hosting-api.service << 'SERVICEEOF'
[Unit]
Description=Hosting Platform Control Panel API
After=network.target postgresql.service docker.service
Requires=postgresql.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/hosting-platform
ExecStart=/opt/hosting-platform/bin/control-panel
EnvironmentFile=/opt/hosting-platform/.env
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=hosting-api

[Install]
WantedBy=multi-user.target
SERVICEEOF

systemctl daemon-reload

# ============================================================
# Start Services
# ============================================================

# Find the PHP-FPM socket path (version may vary)
PHP_FPM_SERVICE=$(systemctl list-units --type=service --all | grep php.*fpm | awk '{print $1}' | head -1)

systemctl enable postfix dovecot opendkim
systemctl restart postfix dovecot opendkim nginx
if [ -n "$PHP_FPM_SERVICE" ]; then
  systemctl enable "$PHP_FPM_SERVICE"
  systemctl restart "$PHP_FPM_SERVICE"
fi

echo "=== EC2 bootstrap complete ==="
echo "Next steps:"
echo "1. Upload Go binary to /opt/hosting-platform/bin/control-panel"
echo "2. Upload .env to /opt/hosting-platform/.env"
echo "3. Run: systemctl start hosting-api"
echo "4. Run: certbot --nginx -d api.${platform_domain} -d mail.${platform_domain} -d webmail.${platform_domain} --email ${certbot_email} --agree-tos --non-interactive"
echo "5. Update /etc/postfix/sasl_passwd with SES SMTP credentials, then run: postmap /etc/postfix/sasl_passwd && systemctl restart postfix"
echo "6. Update Dovecot/Postfix TLS certs to use Let's Encrypt paths, then restart services"
