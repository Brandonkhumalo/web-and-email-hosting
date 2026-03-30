package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"tishanyq-hosting/control-panel/internal/database"
	"tishanyq-hosting/control-panel/internal/middleware"
	"tishanyq-hosting/control-panel/internal/models"
	"tishanyq-hosting/control-panel/internal/services"
)

// EmailHandler handles email account and alias CRUD.
// Creates real mailboxes that users can log into via IMAP (phone) and Roundcube (webmail).
type EmailHandler struct {
	db   *database.DB
	dns  *services.DNSService
	ses  *services.SESService
	mail *services.MailService
}

// NewEmailHandler creates a new EmailHandler.
func NewEmailHandler(db *database.DB, dns *services.DNSService, ses *services.SESService, mail *services.MailService) *EmailHandler {
	return &EmailHandler{db: db, dns: dns, ses: ses, mail: mail}
}

// CreateAccount creates a new email mailbox.
// After creation, the user can immediately configure their phone or log into webmail.
func (h *EmailHandler) CreateAccount(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)

	var req models.CreateEmailAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Quota == 0 {
		req.Quota = 1073741824 // 1GB default
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Verify domain belongs to this customer
	var domainName string
	var domainRoute53ZoneID string
	var emailEnabled bool
	err := h.db.Pool.QueryRow(ctx,
		`SELECT name, route53_zone_id, email_enabled
		 FROM domains WHERE id = $1 AND customer_id = $2`,
		req.DomainID, customerID,
	).Scan(&domainName, &domainRoute53ZoneID, &emailEnabled)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	email := req.Username + "@" + domainName
	maildir := h.mail.MaildirPath(email)

	// If this is the first email on this domain, set up DNS + SES
	if !emailEnabled {
		if err := h.setupDomainEmail(ctx, req.DomainID, domainName, domainRoute53ZoneID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "email setup failed: " + err.Error()})
			return
		}
	}

	// Hash the password (SHA512-CRYPT for Dovecot)
	hashedPassword, err := h.mail.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hashing failed"})
		return
	}

	// Insert email account into the database
	var accountID int64
	err = h.db.Pool.QueryRow(ctx,
		`INSERT INTO email_accounts (domain_id, email, display_name, password, maildir, quota, mail_enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, TRUE)
		 RETURNING id`,
		req.DomainID, email, req.DisplayName, hashedPassword, maildir, req.Quota,
	).Scan(&accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create email account (address may already exist)",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":             accountID,
		"email":          email,
		"quota":          req.Quota,
		"phone_settings": h.mail.PhoneSettings(email),
		"webmail":        fmt.Sprintf("https://webmail.%s", domainName),
		"message":        fmt.Sprintf("Email account %s created. Use the phone_settings to configure your device, or log in at webmail.", email),
	})
}

// setupDomainEmail enables email for a domain by creating DNS records and verifying with SES.
func (h *EmailHandler) setupDomainEmail(ctx context.Context, domainID int64, domainName, zoneID string) error {
	mailHost := h.mail.ServerHost()

	// Create MX record pointing to the mail server
	if err := h.dns.CreateMXRecord(ctx, zoneID, domainName, mailHost); err != nil {
		return fmt.Errorf("MX record: %w", err)
	}

	// Create SPF record (authorize this server + SES)
	spf := fmt.Sprintf("v=spf1 ip4:%s include:amazonses.com -all", h.mail.EC2PublicIP())
	if err := h.dns.CreateTXTRecord(ctx, zoneID, domainName, spf); err != nil {
		return fmt.Errorf("SPF record: %w", err)
	}

	// Create DMARC record
	dmarc := fmt.Sprintf("v=DMARC1; p=quarantine; rua=mailto:postmaster@%s", domainName)
	if err := h.dns.CreateTXTRecord(ctx, zoneID, "_dmarc."+domainName, dmarc); err != nil {
		return fmt.Errorf("DMARC record: %w", err)
	}

	// Create auto-discover SRV records (helps phone email apps auto-configure)
	if err := h.dns.CreateSRVRecord(ctx, zoneID, "_imaps._tcp."+domainName, mailHost, 993); err != nil {
		return fmt.Errorf("IMAP SRV record: %w", err)
	}
	if err := h.dns.CreateSRVRecord(ctx, zoneID, "_submission._tcp."+domainName, mailHost, 587); err != nil {
		return fmt.Errorf("Submission SRV record: %w", err)
	}

	// Verify domain with SES for outbound sending
	if err := h.ses.VerifyDomain(ctx, domainName, zoneID); err != nil {
		return fmt.Errorf("SES verification: %w", err)
	}

	// Mark domain as email-enabled
	_, err := h.db.Pool.Exec(ctx,
		"UPDATE domains SET email_enabled = TRUE WHERE id = $1", domainID)
	return err
}

// ListAccounts returns all email accounts for a domain.
func (h *EmailHandler) ListAccounts(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	domainID, err := strconv.ParseInt(c.Param("domain_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	ctx := c.Request.Context()

	// Verify domain ownership
	var domainName string
	err = h.db.Pool.QueryRow(ctx,
		"SELECT name FROM domains WHERE id = $1 AND customer_id = $2",
		domainID, customerID,
	).Scan(&domainName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	rows, err := h.db.Pool.Query(ctx,
		`SELECT id, domain_id, email, display_name, maildir, quota,
		        mail_enabled, ses_verified, active, created_at, updated_at
		 FROM email_accounts
		 WHERE domain_id = $1
		 ORDER BY email`,
		domainID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	var accounts []models.EmailAccount
	for rows.Next() {
		var a models.EmailAccount
		if err := rows.Scan(&a.ID, &a.DomainID, &a.Email, &a.DisplayName,
			&a.Maildir, &a.Quota, &a.MailEnabled,
			&a.SESVerified, &a.Active, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		accounts = append(accounts, a)
	}

	c.JSON(http.StatusOK, accounts)
}

// UpdatePassword changes an email account's password.
func (h *EmailHandler) UpdatePassword(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}

	var req models.UpdateEmailPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	// Verify ownership: email_accounts → domains → customer
	var email string
	err = h.db.Pool.QueryRow(ctx,
		`SELECT ea.email FROM email_accounts ea
		 JOIN domains d ON d.id = ea.domain_id
		 WHERE ea.id = $1 AND d.customer_id = $2`,
		accountID, customerID,
	).Scan(&email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "email account not found"})
		return
	}

	hashedPassword, err := h.mail.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hashing failed"})
		return
	}

	_, err = h.db.Pool.Exec(ctx,
		"UPDATE email_accounts SET password = $1 WHERE id = $2",
		hashedPassword, accountID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password updated for " + email})
}

// DeleteAccount removes an email account.
func (h *EmailHandler) DeleteAccount(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}

	ctx := c.Request.Context()

	var email string
	err = h.db.Pool.QueryRow(ctx,
		`SELECT ea.email FROM email_accounts ea
		 JOIN domains d ON d.id = ea.domain_id
		 WHERE ea.id = $1 AND d.customer_id = $2`,
		accountID, customerID,
	).Scan(&email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "email account not found"})
		return
	}

	_, err = h.db.Pool.Exec(ctx,
		"DELETE FROM email_accounts WHERE id = $1", accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "email account " + email + " deleted"})
}

// CreateAlias creates a forwarding rule.
func (h *EmailHandler) CreateAlias(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)

	var req models.CreateEmailAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	var domainName string
	err := h.db.Pool.QueryRow(ctx,
		"SELECT name FROM domains WHERE id = $1 AND customer_id = $2",
		req.DomainID, customerID,
	).Scan(&domainName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	// Build full source address if only local part was given
	source := req.Source
	if len(source) > 0 && source[0] != '*' && !containsAt(source) {
		source = source + "@" + domainName
	}

	var aliasID int64
	err = h.db.Pool.QueryRow(ctx,
		`INSERT INTO email_aliases (domain_id, source, destination)
		 VALUES ($1, $2, $3) RETURNING id`,
		req.DomainID, source, req.Destination,
	).Scan(&aliasID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          aliasID,
		"source":      source,
		"destination": req.Destination,
		"message":     fmt.Sprintf("Emails to %s will forward to %s", source, req.Destination),
	})
}

// ListAliases returns all aliases for a domain.
func (h *EmailHandler) ListAliases(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	domainID, err := strconv.ParseInt(c.Param("domain_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	ctx := c.Request.Context()

	var domainName string
	err = h.db.Pool.QueryRow(ctx,
		"SELECT name FROM domains WHERE id = $1 AND customer_id = $2",
		domainID, customerID,
	).Scan(&domainName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	rows, err := h.db.Pool.Query(ctx,
		`SELECT id, domain_id, source, destination, active, created_at
		 FROM email_aliases
		 WHERE domain_id = $1
		 ORDER BY source`,
		domainID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	var aliases []models.EmailAlias
	for rows.Next() {
		var a models.EmailAlias
		if err := rows.Scan(&a.ID, &a.DomainID, &a.Source, &a.Destination,
			&a.Active, &a.CreatedAt); err != nil {
			continue
		}
		aliases = append(aliases, a)
	}

	c.JSON(http.StatusOK, aliases)
}

// DeleteAlias removes a forwarding rule.
func (h *EmailHandler) DeleteAlias(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	aliasID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alias id"})
		return
	}

	ctx := c.Request.Context()

	var source string
	err = h.db.Pool.QueryRow(ctx,
		`SELECT ea.source FROM email_aliases ea
		 JOIN domains d ON d.id = ea.domain_id
		 WHERE ea.id = $1 AND d.customer_id = $2`,
		aliasID, customerID,
	).Scan(&source)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "alias not found"})
		return
	}

	_, err = h.db.Pool.Exec(ctx,
		"DELETE FROM email_aliases WHERE id = $1", aliasID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "alias deleted"})
}

// containsAt checks if a string contains the @ character.
func containsAt(s string) bool {
	for _, c := range s {
		if c == '@' {
			return true
		}
	}
	return false
}
