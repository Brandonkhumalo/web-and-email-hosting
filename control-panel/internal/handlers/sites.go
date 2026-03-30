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

// SiteHandler handles static and backend site provisioning.
type SiteHandler struct {
	db      *database.DB
	hosting *services.HostingService
	dns     *services.DNSService
	ssl     *services.SSLService
}

// NewSiteHandler creates a new SiteHandler.
func NewSiteHandler(db *database.DB, hosting *services.HostingService, dns *services.DNSService, ssl *services.SSLService) *SiteHandler {
	return &SiteHandler{db: db, hosting: hosting, dns: dns, ssl: ssl}
}

// CreateStatic provisions a static site served by Nginx from disk.
func (h *SiteHandler) CreateStatic(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)

	var req models.CreateStaticSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Subdomain == "" {
		req.Subdomain = "@"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// Get domain info (verify ownership)
	var domain models.Domain
	err := h.db.Pool.QueryRow(ctx,
		`SELECT id, name, route53_zone_id
		 FROM domains WHERE id = $1 AND customer_id = $2`,
		req.DomainID, customerID,
	).Scan(&domain.ID, &domain.Name, &domain.Route53ZoneID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	// Build full hostname
	hostname := domain.Name
	if req.Subdomain != "@" {
		hostname = req.Subdomain + "." + domain.Name
	}

	// Create site directory + Nginx config
	result, err := h.hosting.CreateStaticSite(ctx, hostname)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provisioning failed: " + err.Error()})
		return
	}

	// Create DNS A record pointing to EC2
	err = h.dns.CreateARecord(ctx, domain.Route53ZoneID, hostname, h.hosting.EC2PublicIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DNS record failed: " + err.Error()})
		return
	}

	// Save to database
	var siteID int64
	err = h.db.Pool.QueryRow(ctx,
		`INSERT INTO sites (customer_id, domain_id, type, subdomain,
		                     nginx_config_path, site_root)
		 VALUES ($1, $2, 'static', $3, $4, $5)
		 RETURNING id`,
		customerID, req.DomainID, req.Subdomain,
		result.NginxConfigPath, result.SiteRoot,
	).Scan(&siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Request SSL certificate in background (requires DNS to be propagated)
	go func() {
		bgCtx := context.Background()
		certPath, keyPath, err := h.ssl.RequestCertificate(bgCtx, hostname)
		if err != nil {
			return
		}
		h.db.Pool.Exec(bgCtx,
			"UPDATE domains SET cert_path = $1, cert_key_path = $2, ssl_status = 'issued' WHERE id = $3",
			certPath, keyPath, req.DomainID,
		)
	}()

	c.JSON(http.StatusCreated, gin.H{
		"id":        siteID,
		"type":      "static",
		"hostname":  hostname,
		"site_root": result.SiteRoot,
		"message":   "Static site created. Upload your files to " + result.SiteRoot,
	})
}

// CreateBackend provisions a Docker container + Nginx reverse proxy for a backend app.
func (h *SiteHandler) CreateBackend(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)

	var req models.CreateBackendSiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	if req.Subdomain == "" {
		req.Subdomain = "api"
	}
	if req.ContainerPort == 0 {
		req.ContainerPort = 8080
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	// Get domain info
	var domain models.Domain
	err := h.db.Pool.QueryRow(ctx,
		`SELECT id, name, route53_zone_id
		 FROM domains WHERE id = $1 AND customer_id = $2`,
		req.DomainID, customerID,
	).Scan(&domain.ID, &domain.Name, &domain.Route53ZoneID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	// Build full hostname
	hostname := domain.Name
	if req.Subdomain != "@" {
		hostname = req.Subdomain + "." + domain.Name
	}

	// Generate unique container name
	containerName := fmt.Sprintf("customer-%d-%s", customerID, sanitize(hostname))

	// Run Docker container + create Nginx proxy config
	result, err := h.hosting.CreateBackendSite(ctx, services.BackendSiteConfig{
		Hostname:      hostname,
		ContainerName: containerName,
		Image:         req.Image,
		ContainerPort: req.ContainerPort,
		EnvVars:       req.EnvVars,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provisioning failed: " + err.Error()})
		return
	}

	// Create DNS A record pointing to EC2
	err = h.dns.CreateARecord(ctx, domain.Route53ZoneID, hostname, h.hosting.EC2PublicIP())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DNS record failed: " + err.Error()})
		return
	}

	// Save to database
	var siteID int64
	err = h.db.Pool.QueryRow(ctx,
		`INSERT INTO sites (customer_id, domain_id, type, subdomain,
		                     nginx_config_path, container_name, host_port,
		                     container_port, docker_image)
		 VALUES ($1, $2, 'backend', $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		customerID, req.DomainID, req.Subdomain,
		result.NginxConfigPath, result.ContainerName, result.HostPort,
		req.ContainerPort, req.Image,
	).Scan(&siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Request SSL in background
	go func() {
		bgCtx := context.Background()
		certPath, keyPath, err := h.ssl.RequestCertificate(bgCtx, hostname)
		if err != nil {
			return
		}
		h.db.Pool.Exec(bgCtx,
			"UPDATE domains SET cert_path = $1, cert_key_path = $2, ssl_status = 'issued' WHERE id = $3",
			certPath, keyPath, req.DomainID,
		)
	}()

	c.JSON(http.StatusCreated, gin.H{
		"id":       siteID,
		"type":     "backend",
		"hostname": hostname,
		"message":  "Backend service created and running.",
	})
}

// List returns all sites for the authenticated customer.
func (h *SiteHandler) List(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	ctx := c.Request.Context()

	rows, err := h.db.Pool.Query(ctx,
		`SELECT s.id, s.customer_id, s.domain_id, s.type, s.subdomain,
		        s.active, s.created_at, s.updated_at,
		        s.nginx_config_path, s.site_root,
		        s.container_name, s.host_port, s.container_port, s.docker_image,
		        d.name as domain_name
		 FROM sites s
		 JOIN domains d ON d.id = s.domain_id
		 WHERE s.customer_id = $1
		 ORDER BY s.created_at DESC`,
		customerID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	type SiteWithDomain struct {
		models.Site
		DomainName string `json:"domain_name"`
		Hostname   string `json:"hostname"`
	}

	var sites []SiteWithDomain
	for rows.Next() {
		var s SiteWithDomain
		err := rows.Scan(
			&s.ID, &s.CustomerID, &s.DomainID, &s.Type, &s.Subdomain,
			&s.Active, &s.CreatedAt, &s.UpdatedAt,
			&s.NginxConfigPath, &s.SiteRoot,
			&s.ContainerName, &s.HostPort, &s.ContainerPort, &s.DockerImage,
			&s.DomainName,
		)
		if err != nil {
			continue
		}
		if s.Subdomain == "@" {
			s.Hostname = s.DomainName
		} else {
			s.Hostname = s.Subdomain + "." + s.DomainName
		}
		sites = append(sites, s)
	}

	c.JSON(http.StatusOK, sites)
}

// Delete tears down a site (Nginx config + Docker container or files).
func (h *SiteHandler) Delete(c *gin.Context) {
	customerID := middleware.GetCustomerID(c)
	siteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// Get site info + domain name for hostname construction
	var site models.Site
	var domainName string
	err = h.db.Pool.QueryRow(ctx,
		`SELECT s.id, s.type, s.subdomain, s.container_name, s.host_port,
		        d.name
		 FROM sites s
		 JOIN domains d ON d.id = s.domain_id
		 WHERE s.id = $1 AND s.customer_id = $2`,
		siteID, customerID,
	).Scan(&site.ID, &site.Type, &site.Subdomain, &site.ContainerName, &site.HostPort,
		&domainName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
		return
	}

	// Build hostname for Nginx config lookup
	hostname := domainName
	if site.Subdomain != "@" {
		hostname = site.Subdomain + "." + domainName
	}

	// Tear down based on type
	if site.Type == models.SiteTypeStatic {
		h.hosting.DeleteStaticSite(ctx, hostname)
	} else {
		h.hosting.DeleteBackendSite(ctx, site.ContainerName, hostname, site.HostPort)
	}

	// Delete SSL cert
	h.ssl.DeleteCertificate(ctx, hostname)

	_, err = h.db.Pool.Exec(ctx, "DELETE FROM sites WHERE id = $1", siteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "site deleted"})
}

// sanitize replaces dots with dashes for safe naming.
func sanitize(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			result[i] = '-'
		} else {
			result[i] = s[i]
		}
	}
	return string(result)
}
