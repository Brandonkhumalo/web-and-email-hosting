package services

import (
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"strings"

	"tishanyq-hosting/control-panel/internal/config"
)

// MailService handles mail-specific operations: password hashing,
// maildir path generation, and phone settings for email clients.
type MailService struct {
	serverHost string
	ec2IP      string
}

// NewMailService creates a new MailService from config.
func NewMailService(cfg *config.Config) *MailService {
	return &MailService{
		serverHost: cfg.MailServerHost,
		ec2IP:      cfg.EC2PublicIP,
	}
}

// ServerHost returns the mail server hostname (e.g., mail.tishanyq.co.zw).
func (s *MailService) ServerHost() string {
	return s.serverHost
}

// EC2PublicIP returns the server's public IP for SPF records.
func (s *MailService) EC2PublicIP() string {
	return s.ec2IP
}

// HashPassword generates a SHA512-CRYPT hash compatible with Dovecot.
// Format: $6$<salt>$<hash>
// This is the standard crypt(3) SHA-512 scheme used by Dovecot's default_pass_scheme.
func (s *MailService) HashPassword(password string) (string, error) {
	// Generate 16-byte random salt
	saltBytes := make([]byte, 12)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	salt := base64.RawStdEncoding.EncodeToString(saltBytes)[:16]

	// SHA-512 crypt with 5000 rounds (default)
	hash := sha512Crypt([]byte(password), []byte(salt))
	return fmt.Sprintf("$6$%s$%s", salt, hash), nil
}

// MaildirPath returns the maildir path for a given email address.
// e.g., "user@example.com" → "example.com/user/Maildir/"
// This path is relative to /var/mail/vhosts/
func (s *MailService) MaildirPath(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("%s/%s/Maildir/", parts[1], parts[0])
}

// PhoneSettings returns IMAP/SMTP connection info for email clients.
// This is returned when creating an email account so users can configure their phone.
func (s *MailService) PhoneSettings(email string) map[string]interface{} {
	return map[string]interface{}{
		"account_type":      "IMAP",
		"incoming_server":   s.serverHost,
		"incoming_port":     993,
		"incoming_security": "SSL/TLS",
		"outgoing_server":   s.serverHost,
		"outgoing_port":     587,
		"outgoing_security": "STARTTLS",
		"username":          email,
	}
}

// sha512Crypt implements a simplified SHA-512 crypt compatible with Dovecot.
// Uses 5000 rounds (the default for $6$ prefix).
func sha512Crypt(password, salt []byte) string {
	// Step 1: Initial hash = SHA512(password + salt + password)
	alt := sha512.New()
	alt.Write(password)
	alt.Write(salt)
	alt.Write(password)
	altResult := alt.Sum(nil)

	// Step 2: Main hash
	ctx := sha512.New()
	ctx.Write(password)
	ctx.Write(salt)

	// Add alternate hash bytes based on password length
	plen := len(password)
	for ; plen > 64; plen -= 64 {
		ctx.Write(altResult)
	}
	ctx.Write(altResult[:plen])

	// Add bits from password length
	for i := len(password); i > 0; i >>= 1 {
		if i%2 != 0 {
			ctx.Write(altResult)
		} else {
			ctx.Write(password)
		}
	}
	result := ctx.Sum(nil)

	// Step 3: 5000 rounds of hashing
	for i := 0; i < 5000; i++ {
		round := sha512.New()
		if i%2 != 0 {
			round.Write(password)
		} else {
			round.Write(result)
		}
		if i%3 != 0 {
			round.Write(salt)
		}
		if i%7 != 0 {
			round.Write(password)
		}
		if i%2 != 0 {
			round.Write(result)
		} else {
			round.Write(password)
		}
		result = round.Sum(nil)
	}

	// Step 4: Encode with custom base64
	return sha512CryptEncode(result)
}

// sha512CryptEncode encodes the SHA-512 result using crypt's custom base64 alphabet.
func sha512CryptEncode(hash []byte) string {
	const itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	encode := func(a, b, c byte, n int) string {
		v := uint(a)<<16 | uint(b)<<8 | uint(c)
		result := make([]byte, n)
		for i := 0; i < n; i++ {
			result[i] = itoa64[v&0x3f]
			v >>= 6
		}
		return string(result)
	}

	var buf strings.Builder
	buf.WriteString(encode(hash[0], hash[21], hash[42], 4))
	buf.WriteString(encode(hash[22], hash[43], hash[1], 4))
	buf.WriteString(encode(hash[44], hash[2], hash[23], 4))
	buf.WriteString(encode(hash[3], hash[24], hash[45], 4))
	buf.WriteString(encode(hash[25], hash[46], hash[4], 4))
	buf.WriteString(encode(hash[47], hash[5], hash[26], 4))
	buf.WriteString(encode(hash[6], hash[27], hash[48], 4))
	buf.WriteString(encode(hash[28], hash[49], hash[7], 4))
	buf.WriteString(encode(hash[50], hash[8], hash[29], 4))
	buf.WriteString(encode(hash[9], hash[30], hash[51], 4))
	buf.WriteString(encode(hash[31], hash[52], hash[10], 4))
	buf.WriteString(encode(hash[53], hash[11], hash[32], 4))
	buf.WriteString(encode(hash[12], hash[33], hash[54], 4))
	buf.WriteString(encode(hash[34], hash[55], hash[13], 4))
	buf.WriteString(encode(hash[56], hash[14], hash[35], 4))
	buf.WriteString(encode(hash[15], hash[36], hash[57], 4))
	buf.WriteString(encode(hash[37], hash[58], hash[16], 4))
	buf.WriteString(encode(hash[59], hash[17], hash[38], 4))
	buf.WriteString(encode(hash[18], hash[39], hash[60], 4))
	buf.WriteString(encode(hash[40], hash[61], hash[19], 4))
	buf.WriteString(encode(hash[62], hash[20], hash[41], 4))
	buf.WriteString(encode(0, 0, hash[63], 2))

	return buf.String()
}
