# ============================================================
# Amazon SES — Email Sending Service
# Customers send email via SES API/SMTP (no self-hosted mail server)
# ============================================================

# --- Platform Domain Identity ---
# Verifies your platform domain with SES

resource "aws_ses_domain_identity" "platform" {
  domain = var.platform_domain
}

# Verification TXT record
resource "aws_route53_record" "ses_verification" {
  zone_id = aws_route53_zone.platform.zone_id
  name    = "_amazonses.${var.platform_domain}"
  type    = "TXT"
  ttl     = 600
  records = [aws_ses_domain_identity.platform.verification_token]
}

resource "aws_ses_domain_identity_verification" "platform" {
  domain     = aws_ses_domain_identity.platform.id
  depends_on = [aws_route53_record.ses_verification]
}

# --- DKIM for SES ---
# SES signs emails with its own DKIM in addition to OpenDKIM

resource "aws_ses_domain_dkim" "platform" {
  domain = aws_ses_domain_identity.platform.domain
}

# DKIM CNAME records (SES provides 3 tokens)
resource "aws_route53_record" "ses_dkim" {
  count   = 3
  zone_id = aws_route53_zone.platform.zone_id
  name    = "${aws_ses_domain_dkim.platform.dkim_tokens[count.index]}._domainkey.${var.platform_domain}"
  type    = "CNAME"
  ttl     = 600
  records = ["${aws_ses_domain_dkim.platform.dkim_tokens[count.index]}.dkim.amazonses.com"]
}

# --- SES SMTP IAM User ---
# Postfix authenticates to SES using these credentials

resource "aws_iam_user" "ses_smtp" {
  name = "ses-smtp-user"

  tags = { Name = "ses-smtp-user" }
}

resource "aws_iam_user_policy" "ses_smtp" {
  name = "ses-send-email"
  user = aws_iam_user.ses_smtp.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["ses:SendRawEmail", "ses:SendEmail"]
      Resource = "*"
    }]
  })
}

resource "aws_iam_access_key" "ses_smtp" {
  user = aws_iam_user.ses_smtp.name

  # IMPORTANT: After terraform apply:
  # 1. Get the access key: terraform output -raw ses_smtp_access_key
  # 2. Get the secret key: terraform output -raw ses_smtp_secret_key
  # 3. Convert the secret to an SMTP password using AWS's algorithm
  # 4. Put both in /etc/postfix/sasl_passwd on the mail server
  #
  # Conversion: https://docs.aws.amazon.com/ses/latest/dg/smtp-credentials.html
  # Or use: aws sesv2 create-email-identity-policy (easier)
}

# --- SES Configuration Set ---
# Tracks bounce/complaint rates (required for production sending)

resource "aws_ses_configuration_set" "main" {
  name = "tishanyq-hosting"
}

# NOTE: Customer domains are verified dynamically by the Go control panel:
#   ses.VerifyDomainIdentity(domain)
# The control panel also creates Route53 records for customer domain verification.

# NOTE: SES starts in sandbox mode (can only send to verified emails).
# Request production access via AWS Console:
#   SES → Account Dashboard → Request Production Access
# Required before customers can send to any email address.
