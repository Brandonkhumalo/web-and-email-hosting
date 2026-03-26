package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"hosting-platform/control-panel/internal/database"
	"hosting-platform/control-panel/internal/middleware"
	"hosting-platform/control-panel/internal/models"
	"hosting-platform/control-panel/internal/services"
)

// DomainHandler handles domain CRUD and DNS provisioning.
type DomainHandler struct {
	db  *database.DB
	dns *services.DNSService
	ses *services.SESService
	ssl *services.SSLService
}

// NewDomainHandler creates a new DomainHandler with required services.
func NewDomainHandler(db *database.DB, dns *services.DNSService, ses *services.SESService, ssl *services.SSLService) *DomainHandler {
	return &DomainHandler{db: db, dns: dns, ses: ses, ssl: ssl}
}

// Create adds a new domain for the authenticated customer.
// Steps: create Route53 hosted zone → store in DB → return NS records.
func (h *DomainHandler) Create(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)

	var req models.CreateDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Check if domain already exists
	var exists bool
	err := h.db.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM domains WHERE name = $1)", req.Name).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "domain already registered"})
		return
	}

	// Create Route53 hosted zone
	zoneID, nameservers, err := h.dns.CreateHostedZone(ctx, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create DNS zone: " + err.Error()})
		return
	}

	// Insert domain into database
	var domainID int64
	err = h.db.Pool.QueryRow(ctx,
		`INSERT INTO domains (customer_id, name, route53_zone_id)
		 VALUES ($1, $2, $3)
		 RETURNING id`,
		customerID, req.Name, zoneID,
	).Scan(&domainID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Store nameservers
	for _, ns := range nameservers {
		_, err = h.db.Pool.Exec(ctx,
			"INSERT INTO domain_nameservers (domain_id, nameserver) VALUES ($1, $2)",
			domainID, ns,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
	}

	// Request SSL certificate in background via Certbot
	// NOTE: This will only succeed after the customer points their nameservers
	// and DNS propagates. The background goroutine will fail silently if too early.
	go func() {
		bgCtx := context.Background()
		certPath, keyPath, err := h.ssl.RequestCertificate(bgCtx, req.Name)
		if err != nil {
			return
		}
		h.db.Pool.Exec(bgCtx,
			"UPDATE domains SET cert_path = $1, cert_key_path = $2, ssl_status = 'issued' WHERE id = $3",
			certPath, keyPath, domainID,
		)
	}()

	c.JSON(http.StatusCreated, gin.H{
		"id":          domainID,
		"domain":      req.Name,
		"zone_id":     zoneID,
		"nameservers": nameservers,
		"message":     "Domain created. Point your domain's nameservers to the ones listed above.",
	})
}

// List returns all domains for the authenticated customer.
func (h *DomainHandler) List(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	ctx := c.Request.Context()

	rows, err := h.db.Pool.Query(ctx,
		`SELECT id, customer_id, name, route53_zone_id, ns_verified,
		        ssl_status, cert_path, cert_key_path,
		        email_enabled, ses_verified,
		        active, created_at, updated_at
		 FROM domains WHERE customer_id = $1 ORDER BY created_at DESC`,
		customerID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	var domains []models.Domain
	for rows.Next() {
		var d models.Domain
		err := rows.Scan(
			&d.ID, &d.CustomerID, &d.Name, &d.Route53ZoneID, &d.NSVerified,
			&d.SSLStatus, &d.CertPath, &d.CertKeyPath,
			&d.EmailEnabled, &d.SESVerified,
			&d.Active, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan error"})
			return
		}
		domains = append(domains, d)
	}

	c.JSON(http.StatusOK, domains)
}

// GetNameservers returns the NS records for a specific domain.
func (h *DomainHandler) GetNameservers(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
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
		"SELECT nameserver FROM domain_nameservers WHERE domain_id = $1",
		domainID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	var nameservers []string
	for rows.Next() {
		var ns string
		if err := rows.Scan(&ns); err == nil {
			nameservers = append(nameservers, ns)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"domain":      domainName,
		"nameservers": nameservers,
		"instructions": "Set these as your domain's nameservers at your registrar (GoDaddy, Namecheap, etc.)",
	})
}

// Verify checks if the customer has pointed their nameservers correctly.
func (h *DomainHandler) Verify(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	ctx := c.Request.Context()

	var domain models.Domain
	err = h.db.Pool.QueryRow(ctx,
		`SELECT id, name, route53_zone_id, ns_verified
		 FROM domains WHERE id = $1 AND customer_id = $2`,
		domainID, customerID,
	).Scan(&domain.ID, &domain.Name, &domain.Route53ZoneID, &domain.NSVerified)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	if domain.NSVerified {
		c.JSON(http.StatusOK, gin.H{"verified": true, "message": "Domain already verified"})
		return
	}

	verified, err := h.dns.VerifyNameservers(ctx, domain.Name, domain.Route53ZoneID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "verification failed: " + err.Error()})
		return
	}

	if verified {
		h.db.Pool.Exec(ctx, "UPDATE domains SET ns_verified = TRUE WHERE id = $1", domainID)
		c.JSON(http.StatusOK, gin.H{"verified": true, "message": "Nameservers verified! Domain is now active."})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"verified": false,
			"message":  "Nameservers not yet propagated. This can take up to 48 hours.",
		})
	}
}

// Delete removes a domain and cleans up all associated resources.
func (h *DomainHandler) Delete(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	domainID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	ctx := c.Request.Context()

	var zoneID, domainName string
	err = h.db.Pool.QueryRow(ctx,
		"SELECT route53_zone_id, name FROM domains WHERE id = $1 AND customer_id = $2",
		domainID, customerID,
	).Scan(&zoneID, &domainName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	// Delete Route53 hosted zone
	if zoneID != "" {
		if err := h.dns.DeleteHostedZone(ctx, zoneID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete DNS zone"})
			return
		}
	}

	// Delete SSL certificate
	h.ssl.DeleteCertificate(ctx, domainName)

	// Database CASCADE will delete domain_nameservers, sites, email_accounts, email_aliases
	_, err = h.db.Pool.Exec(ctx, "DELETE FROM domains WHERE id = $1", domainID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "domain deleted"})
}
