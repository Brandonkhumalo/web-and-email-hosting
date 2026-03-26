package models

import "time"

// Customer represents a hosting platform customer (business/person).
type Customer struct {
	ID        int64     `json:"id" db:"id"`
	Email     string    `json:"email" db:"email"`
	Password  string    `json:"-" db:"password"` // bcrypt hashed, never exposed in JSON
	Name      string    `json:"name" db:"name"`
	Company   string    `json:"company,omitempty" db:"company"`
	PlanID    *int64    `json:"plan_id,omitempty" db:"plan_id"`
	Active    bool      `json:"active" db:"active"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Plan represents a billing tier (starter, pro, enterprise).
type Plan struct {
	ID               int64  `json:"id" db:"id"`
	Name             string `json:"name" db:"name"`
	MaxSites         int    `json:"max_sites" db:"max_sites"`
	MaxEmailAccounts int    `json:"max_email_accounts" db:"max_email_accounts"`
	MaxStorageMB     int    `json:"max_storage_mb" db:"max_storage_mb"`
	PriceCents       int    `json:"price_cents" db:"price_cents"`
	Active           bool   `json:"active" db:"active"`
}

// Domain represents a customer's domain managed by the platform.
type Domain struct {
	ID            int64     `json:"id" db:"id"`
	CustomerID    int64     `json:"customer_id" db:"customer_id"`
	Name          string    `json:"name" db:"name"`
	Route53ZoneID string    `json:"zone_id" db:"route53_zone_id"`
	Nameservers   []string  `json:"nameservers" db:"-"`
	NSVerified    bool      `json:"ns_verified" db:"ns_verified"`
	SSLStatus     string    `json:"ssl_status" db:"ssl_status"`
	CertPath      string    `json:"cert_path,omitempty" db:"cert_path"`
	CertKeyPath   string    `json:"cert_key_path,omitempty" db:"cert_key_path"`
	EmailEnabled  bool      `json:"email_enabled" db:"email_enabled"`
	SESVerified   bool      `json:"ses_verified" db:"ses_verified"`
	Active        bool      `json:"active" db:"active"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// Site represents a hosted website (static or backend).
type Site struct {
	ID         int64     `json:"id" db:"id"`
	CustomerID int64     `json:"customer_id" db:"customer_id"`
	DomainID   int64     `json:"domain_id" db:"domain_id"`
	Type       SiteType  `json:"type" db:"type"`
	Subdomain  string    `json:"subdomain" db:"subdomain"`
	Active     bool      `json:"active" db:"active"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`

	// Nginx config
	NginxConfigPath string `json:"nginx_config_path,omitempty" db:"nginx_config_path"`

	// Static site fields
	SiteRoot string `json:"site_root,omitempty" db:"site_root"`

	// Backend site fields (Docker container)
	ContainerName string `json:"container_name,omitempty" db:"container_name"`
	HostPort      int    `json:"host_port,omitempty" db:"host_port"`
	ContainerPort int    `json:"container_port,omitempty" db:"container_port"`
	DockerImage   string `json:"docker_image,omitempty" db:"docker_image"`
}

// SiteType distinguishes static (Nginx file server) from backend (Docker container) sites.
type SiteType string

const (
	SiteTypeStatic  SiteType = "static"
	SiteTypeBackend SiteType = "backend"
)

// EmailAccount represents a full email mailbox hosted by Postfix + Dovecot.
// Users can log in via IMAP (phone/webmail) and send via SMTP.
type EmailAccount struct {
	ID          int64     `json:"id" db:"id"`
	DomainID    int64     `json:"domain_id" db:"domain_id"`
	Email       string    `json:"email" db:"email"`
	DisplayName string    `json:"display_name" db:"display_name"`
	Password    string    `json:"-" db:"password"`                  // SHA512-CRYPT hash, never in JSON
	Maildir     string    `json:"maildir,omitempty" db:"maildir"`   // e.g., "example.com/user/Maildir/"
	Quota       int64     `json:"quota" db:"quota"`                 // bytes (default 1GB)
	MailEnabled bool      `json:"mail_enabled" db:"mail_enabled"`   // true = Postfix/Dovecot active
	SESVerified bool      `json:"ses_verified" db:"ses_verified"`
	Active      bool      `json:"active" db:"active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// EmailAlias represents a forwarding rule.
type EmailAlias struct {
	ID          int64     `json:"id" db:"id"`
	DomainID    int64     `json:"domain_id" db:"domain_id"`
	Source      string    `json:"source" db:"source"`
	Destination string    `json:"destination" db:"destination"`
	Active      bool      `json:"active" db:"active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// --- Request/Response DTOs ---

type CreateCustomerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Name     string `json:"name" binding:"required"`
	Company  string `json:"company"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Customer  Customer  `json:"customer"`
}

type CreateDomainRequest struct {
	Name string `json:"name" binding:"required"`
}

type CreateStaticSiteRequest struct {
	DomainID  int64  `json:"domain_id" binding:"required"`
	Subdomain string `json:"subdomain"` // defaults to "@" (root domain)
}

type CreateBackendSiteRequest struct {
	DomainID      int64             `json:"domain_id" binding:"required"`
	Subdomain     string            `json:"subdomain"`      // defaults to "api"
	Image         string            `json:"image" binding:"required"` // Docker image (e.g., myrepo/myapp:v1)
	ContainerPort int               `json:"container_port"` // defaults to 8080
	EnvVars       map[string]string `json:"env_vars"`       // environment variables
}

type CreateEmailAccountRequest struct {
	DomainID    int64  `json:"domain_id" binding:"required"`
	Username    string `json:"username" binding:"required"`          // local part (before @)
	Password    string `json:"password" binding:"required,min=8"`   // mailbox password
	DisplayName string `json:"display_name"`
	Quota       int64  `json:"quota"` // bytes, defaults to 1GB if 0
}

type UpdateEmailPasswordRequest struct {
	Password string `json:"password" binding:"required,min=8"`
}

type CreateEmailAliasRequest struct {
	DomainID    int64  `json:"domain_id" binding:"required"`
	Source      string `json:"source" binding:"required"`
	Destination string `json:"destination" binding:"required"`
}

type DomainDNSResponse struct {
	Domain      string      `json:"domain"`
	Nameservers []string    `json:"nameservers"`
	Records     []DNSRecord `json:"records"`
}

type DNSRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	Purpose string `json:"purpose"`
}
