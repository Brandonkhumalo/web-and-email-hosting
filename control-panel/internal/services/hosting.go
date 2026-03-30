package services

import (
	"context"
	"fmt"
	"log"

	"tishanyq-hosting/control-panel/internal/config"
)

// HostingService orchestrates static site and backend site provisioning.
// Static sites: creates directory + Nginx config to serve files.
// Backend sites: runs Docker container + Nginx reverse proxy config.
type HostingService struct {
	nginx  *NginxService
	docker *DockerService
	ec2IP  string // Elastic IP of this EC2 instance
}

// StaticSiteResult contains the info stored in the DB after creating a static site.
type StaticSiteResult struct {
	SiteRoot        string // /var/www/example.com
	NginxConfigPath string // /etc/nginx/sites-available/example.com.conf
}

// BackendSiteResult contains the info stored in the DB after creating a backend site.
type BackendSiteResult struct {
	ContainerName   string // e.g., "customer-123-api"
	HostPort        int    // e.g., 10001
	NginxConfigPath string
}

// BackendSiteConfig holds parameters for creating a backend site.
type BackendSiteConfig struct {
	Hostname      string            // Full hostname (e.g., api.example.com)
	ContainerName string            // Unique container name
	Image         string            // Docker image (e.g., myrepo/myapp:v1)
	ContainerPort int               // Port the app listens on inside the container
	EnvVars       map[string]string // Environment variables for the container
}

// NewHostingService creates a new hosting service.
func NewHostingService(cfg *config.Config, nginx *NginxService, docker *DockerService) *HostingService {
	return &HostingService{
		nginx:  nginx,
		docker: docker,
		ec2IP:  cfg.EC2PublicIP,
	}
}

// EC2PublicIP returns the Elastic IP for creating DNS A records.
func (s *HostingService) EC2PublicIP() string {
	return s.ec2IP
}

// CreateStaticSite creates a static website hosted via Nginx.
// 1. Creates /var/www/{hostname}/ with a default index.html
// 2. Generates Nginx server block to serve the directory
// 3. Reloads Nginx
//
// After this, the customer uploads their files to /var/www/{hostname}/
// and obtains SSL via Certbot.
func (s *HostingService) CreateStaticSite(ctx context.Context, hostname string) (*StaticSiteResult, error) {
	configPath, siteRoot, err := s.nginx.CreateStaticSiteConfig(hostname)
	if err != nil {
		return nil, fmt.Errorf("create static site %s: %w", hostname, err)
	}

	log.Printf("Hosting: created static site %s (root=%s)", hostname, siteRoot)

	return &StaticSiteResult{
		SiteRoot:        siteRoot,
		NginxConfigPath: configPath,
	}, nil
}

// CreateBackendSite creates a backend application hosted in a Docker container.
// 1. Runs a Docker container with the specified image
// 2. Generates Nginx reverse proxy config pointing to the container port
// 3. Reloads Nginx
//
// After this, the site is accessible via HTTP. SSL is obtained separately via Certbot.
func (s *HostingService) CreateBackendSite(ctx context.Context, cfg BackendSiteConfig) (*BackendSiteResult, error) {
	// Start the Docker container
	hostPort, err := s.docker.RunContainer(ctx, cfg.ContainerName, cfg.Image, cfg.ContainerPort, cfg.EnvVars)
	if err != nil {
		return nil, fmt.Errorf("run container for %s: %w", cfg.Hostname, err)
	}

	// Create Nginx reverse proxy config
	configPath, err := s.nginx.CreateProxySiteConfig(cfg.Hostname, hostPort)
	if err != nil {
		// Rollback: stop the container
		s.docker.StopAndRemoveContainer(ctx, cfg.ContainerName)
		s.docker.ReleasePort(hostPort)
		return nil, fmt.Errorf("create proxy config for %s: %w", cfg.Hostname, err)
	}

	log.Printf("Hosting: created backend site %s (container=%s, port=%d)", cfg.Hostname, cfg.ContainerName, hostPort)

	return &BackendSiteResult{
		ContainerName:   cfg.ContainerName,
		HostPort:        hostPort,
		NginxConfigPath: configPath,
	}, nil
}

// DeleteStaticSite removes a static site's Nginx config and files.
func (s *HostingService) DeleteStaticSite(ctx context.Context, hostname string) error {
	if err := s.nginx.DeleteSiteConfig(hostname); err != nil {
		log.Printf("Warning: failed to delete nginx config for %s: %v", hostname, err)
	}
	if err := s.nginx.DeleteSiteRoot(hostname); err != nil {
		log.Printf("Warning: failed to delete site root for %s: %v", hostname, err)
	}
	log.Printf("Hosting: deleted static site %s", hostname)
	return nil
}

// DeleteBackendSite stops a Docker container and removes its Nginx config.
func (s *HostingService) DeleteBackendSite(ctx context.Context, containerName, hostname string, hostPort int) error {
	if err := s.docker.StopAndRemoveContainer(ctx, containerName); err != nil {
		log.Printf("Warning: failed to stop container %s: %v", containerName, err)
	}
	s.docker.ReleasePort(hostPort)

	if err := s.nginx.DeleteSiteConfig(hostname); err != nil {
		log.Printf("Warning: failed to delete nginx config for %s: %v", hostname, err)
	}

	log.Printf("Hosting: deleted backend site %s (container=%s)", hostname, containerName)
	return nil
}
