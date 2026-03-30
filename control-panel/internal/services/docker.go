package services

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

	"tishanyq-hosting/control-panel/internal/config"
)

// DockerService manages customer backend containers using the Docker CLI.
// Each customer backend runs as a Docker container with an allocated host port.
// Nginx reverse-proxies traffic from port 80/443 to the container's host port.
type DockerService struct {
	network        string         // Docker network name (e.g., "customer-net")
	portRangeStart int            // Start of port allocation range (e.g., 10000)
	portRangeEnd   int            // End of port allocation range (e.g., 10999)
	usedPorts      map[int]bool   // Tracks which ports are in use
	mu             sync.Mutex     // Protects usedPorts map
}

// NewDockerService creates a new DockerService from config.
func NewDockerService(cfg *config.Config) *DockerService {
	return &DockerService{
		network:        cfg.DockerNetwork,
		portRangeStart: cfg.PortRangeStart,
		portRangeEnd:   cfg.PortRangeEnd,
		usedPorts:      make(map[int]bool),
	}
}

// LoadUsedPorts syncs the in-memory port map from the database.
// Call this once at startup with all host_port values from the sites table.
func (s *DockerService) LoadUsedPorts(ports []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range ports {
		if p > 0 {
			s.usedPorts[p] = true
		}
	}
	log.Printf("Docker: loaded %d used ports", len(s.usedPorts))
}

// RunContainer starts a Docker container for a customer backend.
// Returns the allocated host port.
//
// The container runs with:
//   - --restart unless-stopped (Docker restarts it on crash or EC2 reboot)
//   - --memory and --cpus resource limits
//   - Port mapping from allocated host port to container port
func (s *DockerService) RunContainer(ctx context.Context, name, image string, containerPort int, envVars map[string]string) (hostPort int, err error) {
	// Allocate a unique host port
	hostPort, err = s.AllocatePort()
	if err != nil {
		return 0, err
	}

	// Build docker run command
	// docker run -d --name {name} --network {network}
	//   -p {hostPort}:{containerPort}
	//   --restart unless-stopped
	//   --memory 512m --cpus 0.5
	//   -e KEY=VALUE ...
	//   {image}
	args := []string{
		"run", "-d",
		"--name", name,
		"--network", s.network,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort),
		"--restart", "unless-stopped",
		"--memory", "512m",
		"--cpus", "0.5",
	}

	// Add environment variables
	for k, v := range envVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, image)

	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.ReleasePort(hostPort)
		return 0, fmt.Errorf("docker run failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	log.Printf("Docker: started container %s (image=%s, port=%d->%d)", name, image, hostPort, containerPort)
	return hostPort, nil
}

// StopAndRemoveContainer stops and removes a Docker container by name.
func (s *DockerService) StopAndRemoveContainer(ctx context.Context, name string) error {
	// Stop the container (10 second grace period)
	stopCmd := exec.CommandContext(ctx, "docker", "stop", name)
	if output, err := stopCmd.CombinedOutput(); err != nil {
		// Container might already be stopped — try to remove anyway
		log.Printf("Docker: stop %s warning: %s", name, strings.TrimSpace(string(output)))
	}

	// Remove the container
	rmCmd := exec.CommandContext(ctx, "docker", "rm", name)
	if output, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm %s failed: %s: %w", name, strings.TrimSpace(string(output)), err)
	}

	log.Printf("Docker: stopped and removed container %s", name)
	return nil
}

// ContainerStatus checks if a container is running.
// Returns: "running", "exited", "paused", "restarting", or "not_found".
func (s *DockerService) ContainerStatus(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Status}}", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "not_found", nil
	}
	return strings.TrimSpace(string(output)), nil
}

// AllocatePort finds the next available port in the configured range.
// Thread-safe via mutex.
func (s *DockerService) AllocatePort() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for port := s.portRangeStart; port <= s.portRangeEnd; port++ {
		if !s.usedPorts[port] {
			s.usedPorts[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", s.portRangeStart, s.portRangeEnd)
}

// ReleasePort marks a port as available for reuse.
func (s *DockerService) ReleasePort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.usedPorts, port)
}
