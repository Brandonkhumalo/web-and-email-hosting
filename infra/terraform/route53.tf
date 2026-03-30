# ============================================================
# Route53 — DNS for the platform itself
# Per-customer hosted zones are created dynamically by the Go API
# ============================================================

resource "aws_route53_zone" "platform" {
  name = var.platform_domain

  tags = { Name = "platform-dns" }
}

# --- Platform domain → EC2 Elastic IP ---
resource "aws_route53_record" "platform_a" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = var.platform_domain
  type    = "A"
  ttl     = 300
  records = [aws_eip.main.public_ip]
}

# --- api.tishanyq.co.zw → EC2 Elastic IP ---
resource "aws_route53_record" "api" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = "api.${var.platform_domain}"
  type    = "A"
  ttl     = 300
  records = [aws_eip.main.public_ip]
}

# --- mail.tishanyq.co.zw → EC2 Elastic IP ---
resource "aws_route53_record" "mail" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = "mail.${var.platform_domain}"
  type    = "A"
  ttl     = 300
  records = [aws_eip.main.public_ip]
}

# --- webmail.tishanyq.co.zw → EC2 Elastic IP ---
resource "aws_route53_record" "webmail" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = "webmail.${var.platform_domain}"
  type    = "A"
  ttl     = 300
  records = [aws_eip.main.public_ip]
}

# --- MX record (receive email on this server) ---
resource "aws_route53_record" "platform_mx" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = var.platform_domain
  type    = "MX"
  ttl     = 300
  records = ["10 mail.${var.platform_domain}"]
}

# --- SPF record (authorize this server + SES to send email) ---
resource "aws_route53_record" "platform_spf" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = var.platform_domain
  type    = "TXT"
  ttl     = 300
  records = ["v=spf1 ip4:${aws_eip.main.public_ip} include:amazonses.com -all"]
}

# --- DMARC record ---
resource "aws_route53_record" "platform_dmarc" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = "_dmarc.${var.platform_domain}"
  type    = "TXT"
  ttl     = 300
  records = ["v=DMARC1; p=quarantine; rua=mailto:postmaster@${var.platform_domain}"]
}

# --- Auto-discover SRV records (helps phones auto-configure email) ---
resource "aws_route53_record" "autodiscover_imap" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = "_imaps._tcp.${var.platform_domain}"
  type    = "SRV"
  ttl     = 300
  records = ["0 1 993 mail.${var.platform_domain}"]
}

resource "aws_route53_record" "autodiscover_submission" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = "_submission._tcp.${var.platform_domain}"
  type    = "SRV"
  ttl     = 300
  records = ["0 1 587 mail.${var.platform_domain}"]
}

# NOTE: Per-customer DNS is managed dynamically by the Go control panel:
# 1. route53.CreateHostedZone(customerDomain) → returns NS records
# 2. Customer points their domain's nameservers to these NS records
# 3. Control panel creates A, MX, SPF, DMARC, SRV records
# 4. Certbot obtains SSL certificate for the domain
