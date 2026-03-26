package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all environment-based configuration for the control panel.
type Config struct {
	// Server
	Port        string
	Environment string

	// Database — supports DATABASE_URL (RDS-style) or individual DB_* vars for local dev
	DatabaseURL string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	DBSSLMode   string

	// AWS
	AWSRegion    string
	AWSAccountID string

	// SES
	SESRegion string

	// Route53 (platform's own zone)
	PlatformDomain string
	PlatformZoneID string

	// EC2 (the server this runs on)
	EC2PublicIP string

	// Mail server (Postfix + Dovecot on same EC2)
	MailServerHost string

	// Nginx
	NginxSitesDir   string // /etc/nginx/sites-available
	NginxEnabledDir string // /etc/nginx/sites-enabled
	SitesRootDir    string // /var/www

	// Docker (customer backend containers)
	DockerNetwork  string
	PortRangeStart int
	PortRangeEnd   int

	// SSL (Let's Encrypt via Certbot)
	CertbotEmail string

	// JWT
	JWTSecret      string
	JWTExpireHours int
}

// Load reads all configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		Environment: getEnv("ENVIRONMENT", "development"),

		// Database — if DATABASE_URL is set, it takes priority over individual DB_* vars
		DatabaseURL: os.Getenv("DATABASE_URL"),
		DBHost:      getEnv("DB_HOST", "localhost"),
		DBPort:      getEnv("DB_PORT", "5432"),
		DBUser:      getEnv("DB_USER", "postgres"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBName:      getEnv("DB_NAME", "hostingplatform"),
		DBSSLMode:   getEnv("DB_SSLMODE", "disable"),

		// AWS
		AWSRegion:    getEnv("AWS_REGION", "us-east-1"),
		AWSAccountID: requireEnv("AWS_ACCOUNT_ID"),

		// SES
		SESRegion: getEnv("SES_REGION", "us-east-1"),

		// Route53
		PlatformDomain: requireEnv("PLATFORM_DOMAIN"),
		PlatformZoneID: requireEnv("PLATFORM_ZONE_ID"),

		// EC2
		EC2PublicIP: requireEnv("EC2_PUBLIC_IP"),

		// Mail server
		MailServerHost: getEnv("MAIL_SERVER_HOST", "mail."+os.Getenv("PLATFORM_DOMAIN")),

		// Nginx
		NginxSitesDir:   getEnv("NGINX_SITES_DIR", "/etc/nginx/sites-available"),
		NginxEnabledDir: getEnv("NGINX_ENABLED_DIR", "/etc/nginx/sites-enabled"),
		SitesRootDir:    getEnv("SITES_ROOT_DIR", "/var/www"),

		// Docker
		DockerNetwork:  getEnv("DOCKER_NETWORK", "customer-net"),
		PortRangeStart: getEnvInt("PORT_RANGE_START", 10000),
		PortRangeEnd:   getEnvInt("PORT_RANGE_END", 10999),

		// Certbot
		CertbotEmail: requireEnv("CERTBOT_EMAIL"),

		// JWT
		JWTSecret:      requireEnv("JWT_SECRET"),
		JWTExpireHours: getEnvInt("JWT_EXPIRE_HOURS", 24),
	}

	// Validate: need either DATABASE_URL or DB_PASSWORD to connect
	if cfg.DatabaseURL == "" && cfg.DBPassword == "" {
		panic("either DATABASE_URL or DB_PASSWORD must be set")
	}

	return cfg, nil
}

// DSN returns the PostgreSQL connection string.
// If DATABASE_URL is set (e.g. from RDS), it is used directly.
// Otherwise, individual DB_* variables are composed into a connection string.
func (c *Config) DSN() string {
	if c.DatabaseURL != "" {
		return c.DatabaseURL
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}

// getEnv returns the value of an environment variable, or a default if not set.
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// requireEnv returns the value of an environment variable.
// Panics if the variable is not set (fail fast on startup).
func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return val
}

// getEnvInt returns an environment variable as an integer, or a default.
func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}
