package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// SSLService manages Let's Encrypt SSL certificates via Certbot.
// Certbot uses the Nginx plugin to automatically configure SSL.
type SSLService struct {
	certbotEmail string
}

// NewSSLService creates a new SSL service.
func NewSSLService(certbotEmail string) *SSLService {
	return &SSLService{
		certbotEmail: certbotEmail,
	}
}

// RequestCertificate obtains a Let's Encrypt SSL certificate for a hostname.
// Uses the Certbot Nginx plugin which automatically:
//   - Obtains the certificate via HTTP-01 challenge
//   - Modifies the Nginx config to add SSL directives
//   - Sets up auto-renewal (via systemd timer)
//
// Prerequisites: The hostname must have a DNS A record pointing to this EC2's IP,
// and an Nginx server block must exist for the hostname.
//
// Returns the certificate and key file paths.
func (s *SSLService) RequestCertificate(ctx context.Context, hostname string) (certPath, keyPath string, err error) {
	// certbot --nginx -d {hostname} --email {email} --agree-tos --non-interactive
	cmd := exec.CommandContext(ctx, "certbot",
		"--nginx",
		"-d", hostname,
		"--email", s.certbotEmail,
		"--agree-tos",
		"--non-interactive",
		"--redirect", // Add HTTP->HTTPS redirect
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("certbot failed for %s: %s: %w", hostname, strings.TrimSpace(string(output)), err)
	}

	// Standard Let's Encrypt cert paths
	certPath = fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", hostname)
	keyPath = fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", hostname)

	log.Printf("SSL: obtained certificate for %s", hostname)
	return certPath, keyPath, nil
}

// GetCertificateStatus checks if a certificate exists and is valid.
// Returns: "issued", "pending", or "none".
func (s *SSLService) GetCertificateStatus(hostname string) string {
	certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", hostname)
	if _, err := os.Stat(certPath); err == nil {
		return "issued"
	}
	return "none"
}

// DeleteCertificate removes a Let's Encrypt certificate.
func (s *SSLService) DeleteCertificate(ctx context.Context, hostname string) error {
	cmd := exec.CommandContext(ctx, "certbot", "delete",
		"--cert-name", hostname,
		"--non-interactive",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Non-fatal — cert might not exist
		log.Printf("SSL: certbot delete for %s: %s", hostname, strings.TrimSpace(string(output)))
		return nil
	}

	log.Printf("SSL: deleted certificate for %s", hostname)
	return nil
}
