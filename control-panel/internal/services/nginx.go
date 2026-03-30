package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"tishanyq-hosting/control-panel/internal/config"
)

// NginxService manages Nginx server block configs for customer sites.
// It generates configs, symlinks them, and reloads Nginx safely.
type NginxService struct {
	sitesAvailableDir string // e.g., /etc/nginx/sites-available
	sitesEnabledDir   string // e.g., /etc/nginx/sites-enabled
	sitesRootDir      string // e.g., /var/www
}

// NewNginxService creates a new NginxService from config.
func NewNginxService(cfg *config.Config) *NginxService {
	return &NginxService{
		sitesAvailableDir: cfg.NginxSitesDir,
		sitesEnabledDir:   cfg.NginxEnabledDir,
		sitesRootDir:      cfg.SitesRootDir,
	}
}

// CreateStaticSiteConfig generates an Nginx server block for a static site.
// The site serves files from /var/www/{hostname}/.
// Returns the config file path and site root directory.
func (s *NginxService) CreateStaticSiteConfig(hostname string) (configPath string, siteRoot string, err error) {
	siteRoot = filepath.Join(s.sitesRootDir, hostname)

	// Create site root directory with a default index.html
	if err := os.MkdirAll(siteRoot, 0755); err != nil {
		return "", "", fmt.Errorf("create site root %s: %w", siteRoot, err)
	}

	indexPath := filepath.Join(siteRoot, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		defaultHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>%s</title></head>
<body>
<h1>Welcome to %s</h1>
<p>Your site is live. Upload your files to replace this page.</p>
</body>
</html>`, hostname, hostname)
		if err := os.WriteFile(indexPath, []byte(defaultHTML), 0644); err != nil {
			return "", "", fmt.Errorf("write default index.html: %w", err)
		}
	}

	// Generate Nginx server block config
	// Certbot will add SSL directives later via: certbot --nginx -d {hostname}
	config := fmt.Sprintf(`server {
    listen 80;
    server_name %s;

    root %s;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;

    # Cache static assets
    location ~* \.(css|js|jpg|jpeg|png|gif|ico|svg|woff|woff2)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }
}
`, hostname, siteRoot)

	configPath = filepath.Join(s.sitesAvailableDir, hostname+".conf")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return "", "", fmt.Errorf("write nginx config %s: %w", configPath, err)
	}

	// Symlink to sites-enabled
	enabledPath := filepath.Join(s.sitesEnabledDir, hostname+".conf")
	os.Remove(enabledPath) // Remove existing symlink if any
	if err := os.Symlink(configPath, enabledPath); err != nil {
		return "", "", fmt.Errorf("symlink config: %w", err)
	}

	// Test and reload Nginx
	if err := s.TestAndReload(); err != nil {
		// Rollback: remove config and symlink
		os.Remove(enabledPath)
		os.Remove(configPath)
		return "", "", fmt.Errorf("nginx reload failed: %w", err)
	}

	log.Printf("Nginx: created static site config for %s", hostname)
	return configPath, siteRoot, nil
}

// CreateProxySiteConfig generates a reverse proxy Nginx server block.
// Proxies all traffic to a Docker container on the given port.
func (s *NginxService) CreateProxySiteConfig(hostname string, port int) (configPath string, err error) {
	config := fmt.Sprintf(`server {
    listen 80;
    server_name %s;

    location / {
        proxy_pass http://127.0.0.1:%d;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";

        # Timeouts for backend apps
        proxy_connect_timeout 30s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
`, hostname, port)

	configPath = filepath.Join(s.sitesAvailableDir, hostname+".conf")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return "", fmt.Errorf("write nginx config %s: %w", configPath, err)
	}

	// Symlink to sites-enabled
	enabledPath := filepath.Join(s.sitesEnabledDir, hostname+".conf")
	os.Remove(enabledPath)
	if err := os.Symlink(configPath, enabledPath); err != nil {
		return "", fmt.Errorf("symlink config: %w", err)
	}

	// Test and reload
	if err := s.TestAndReload(); err != nil {
		os.Remove(enabledPath)
		os.Remove(configPath)
		return "", fmt.Errorf("nginx reload failed: %w", err)
	}

	log.Printf("Nginx: created proxy config for %s -> port %d", hostname, port)
	return configPath, nil
}

// DeleteSiteConfig removes the Nginx config and symlink for a hostname.
func (s *NginxService) DeleteSiteConfig(hostname string) error {
	configPath := filepath.Join(s.sitesAvailableDir, hostname+".conf")
	enabledPath := filepath.Join(s.sitesEnabledDir, hostname+".conf")

	os.Remove(enabledPath)
	os.Remove(configPath)

	if err := s.TestAndReload(); err != nil {
		return fmt.Errorf("nginx reload after delete: %w", err)
	}

	log.Printf("Nginx: deleted config for %s", hostname)
	return nil
}

// DeleteSiteRoot removes the /var/www/{hostname}/ directory.
func (s *NginxService) DeleteSiteRoot(hostname string) error {
	siteRoot := filepath.Join(s.sitesRootDir, hostname)
	if err := os.RemoveAll(siteRoot); err != nil {
		return fmt.Errorf("remove site root %s: %w", siteRoot, err)
	}
	log.Printf("Nginx: deleted site root %s", siteRoot)
	return nil
}

// TestAndReload runs "nginx -t" to validate config, then reloads.
// If the test fails, Nginx is NOT reloaded (safe operation).
func (s *NginxService) TestAndReload() error {
	// Test config first
	testCmd := exec.Command("nginx", "-t")
	if output, err := testCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nginx config test failed: %s: %w", string(output), err)
	}

	// Reload
	reloadCmd := exec.Command("nginx", "-s", "reload")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nginx reload failed: %s: %w", string(output), err)
	}

	return nil
}
