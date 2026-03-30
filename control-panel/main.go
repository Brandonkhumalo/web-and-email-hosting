package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"tishanyq-hosting/control-panel/internal/config"
	"tishanyq-hosting/control-panel/internal/database"
	"tishanyq-hosting/control-panel/internal/handlers"
	"tishanyq-hosting/control-panel/internal/middleware"
	"tishanyq-hosting/control-panel/internal/models"
	"tishanyq-hosting/control-panel/internal/services"
)

func main() {
	// Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to the database (single PostgreSQL on same EC2)
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations (creates tables if they don't exist)
	if err := db.RunMigrations(context.Background()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Load AWS config (for Route53 + SES)
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Initialize services
	dnsService := services.NewDNSService(awsCfg)
	sesService := services.NewSESService(awsCfg, cfg, dnsService)
	sslService := services.NewSSLService(cfg.CertbotEmail)
	nginxService := services.NewNginxService(cfg)
	dockerService := services.NewDockerService(cfg)
	hostingService := services.NewHostingService(cfg, nginxService, dockerService)
	mailService := services.NewMailService(cfg)

	// Sync Docker port allocations from database
	loadUsedPorts(db, dockerService)

	// Initialize handlers
	domainHandler := handlers.NewDomainHandler(db, dnsService, sesService, sslService)
	siteHandler := handlers.NewSiteHandler(db, hostingService, dnsService, sslService)
	emailHandler := handlers.NewEmailHandler(db, dnsService, sesService, mailService)

	// Set up Gin router
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// Health check (no auth)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public routes (no auth)
	public := router.Group("/api")
	{
		public.POST("/register", registerHandler(db, cfg))
		public.POST("/login", loginHandler(db, cfg))
	}

	// Authenticated routes
	api := router.Group("/api")
	api.Use(middleware.AuthRequired(cfg.JWTSecret))
	{
		// Domains
		api.POST("/domains", domainHandler.Create)
		api.GET("/domains", domainHandler.List)
		api.GET("/domains/:id/nameservers", domainHandler.GetNameservers)
		api.POST("/domains/:id/verify", domainHandler.Verify)
		api.DELETE("/domains/:id", domainHandler.Delete)

		// Sites
		api.POST("/sites/static", siteHandler.CreateStatic)
		api.POST("/sites/backend", siteHandler.CreateBackend)
		api.GET("/sites", siteHandler.List)
		api.DELETE("/sites/:id", siteHandler.Delete)

		// Email (full mailbox hosting via Postfix + Dovecot)
		api.POST("/email/accounts", emailHandler.CreateAccount)
		api.GET("/email/domains/:domain_id/accounts", emailHandler.ListAccounts)
		api.PUT("/email/accounts/:id/password", emailHandler.UpdatePassword)
		api.DELETE("/email/accounts/:id", emailHandler.DeleteAccount)
		api.POST("/email/aliases", emailHandler.CreateAlias)
		api.GET("/email/domains/:domain_id/aliases", emailHandler.ListAliases)
		api.DELETE("/email/aliases/:id", emailHandler.DeleteAlias)
	}

	// Start server with graceful shutdown
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Control panel API starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server stopped")
}

// loadUsedPorts reads all host_port values from the sites table
// and registers them with the DockerService so port allocation
// doesn't assign already-in-use ports.
func loadUsedPorts(db *database.DB, docker *services.DockerService) {
	rows, err := db.Pool.Query(context.Background(),
		"SELECT host_port FROM sites WHERE host_port > 0")
	if err != nil {
		log.Printf("Warning: failed to load used ports: %v", err)
		return
	}
	defer rows.Close()

	var ports []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err == nil {
			ports = append(ports, p)
		}
	}
	docker.LoadUsedPorts(ports)
}

// registerHandler creates a new customer account.
func registerHandler(db *database.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.CreateCustomerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		var customer models.Customer
		err = db.Pool.QueryRow(c.Request.Context(),
			`INSERT INTO customers (email, password, name, company)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id, email, name, company, active, created_at, updated_at`,
			req.Email, string(hashedPassword), req.Name, req.Company,
		).Scan(&customer.ID, &customer.Email, &customer.Name, &customer.Company,
			&customer.Active, &customer.CreatedAt, &customer.UpdatedAt)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}

		token, expiresAt, err := middleware.GenerateToken(
			customer.ID, customer.Email, cfg.JWTSecret, cfg.JWTExpireHours)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		c.JSON(http.StatusCreated, models.LoginResponse{
			Token:     token,
			ExpiresAt: expiresAt,
			Customer:  customer,
		})
	}
}

// loginHandler authenticates a customer and returns a JWT.
func loginHandler(db *database.DB, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var customer models.Customer
		err := db.Pool.QueryRow(c.Request.Context(),
			`SELECT id, email, password, name, company, active, created_at, updated_at
			 FROM customers WHERE email = $1`,
			req.Email,
		).Scan(&customer.ID, &customer.Email, &customer.Password, &customer.Name,
			&customer.Company, &customer.Active, &customer.CreatedAt, &customer.UpdatedAt)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		if !customer.Active {
			c.JSON(http.StatusForbidden, gin.H{"error": "account disabled"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(customer.Password), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		token, expiresAt, err := middleware.GenerateToken(
			customer.ID, customer.Email, cfg.JWTSecret, cfg.JWTExpireHours)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		customer.Password = ""

		c.JSON(http.StatusOK, models.LoginResponse{
			Token:     token,
			ExpiresAt: expiresAt,
			Customer:  customer,
		})
	}
}
