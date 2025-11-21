package e2eframe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// ResourceDiscoverer discovers orphaned Docker resources created by ene.
type ResourceDiscoverer struct {
	dockerClient *client.Client
}

// DiscoverOptions configures resource discovery behavior.
type DiscoverOptions struct {
	// OlderThan filters resources created before this duration ago.
	// Zero value means no age filtering.
	OlderThan time.Duration

	// IncludeAll includes all matching resources, even if they're in use.
	// By default, only unused/orphaned resources are returned.
	IncludeAll bool

	// Pattern is a custom name pattern to match against.
	// If empty, uses default ene patterns.
	Pattern string
}

// OrphanedNetwork represents a Docker network that appears to be orphaned.
type OrphanedNetwork struct {
	ID               string
	Name             string
	Created          time.Time
	Age              time.Duration
	Containers       int
	Driver           string
	Scope            string
	IsTestcontainers bool
}

// OrphanedContainer represents a Docker container that appears to be orphaned.
type OrphanedContainer struct {
	ID      string
	Name    string
	Created time.Time
	Age     time.Duration
	Status  string
	State   string
	Image   string
	IsEne   bool
}

// NewResourceDiscoverer creates a new resource discoverer using the local Docker daemon.
func NewResourceDiscoverer() (*ResourceDiscoverer, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &ResourceDiscoverer{
		dockerClient: dockerClient,
	}, nil
}

// Close closes the Docker client connection.
func (d *ResourceDiscoverer) Close() error {
	if d.dockerClient != nil {
		return d.dockerClient.Close()
	}
	return nil
}

// DiscoverOrphanedNetworks finds Docker networks that appear to be orphaned.
// Networks are considered orphaned if:
// - They match testcontainers naming patterns
// - They have no containers attached (unless IncludeAll is true)
// - They are older than OlderThan threshold (if specified)
func (d *ResourceDiscoverer) DiscoverOrphanedNetworks(ctx context.Context, opts DiscoverOptions) ([]OrphanedNetwork, error) {
	networks, err := d.dockerClient.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	orphaned := make([]OrphanedNetwork, 0)
	now := time.Now()

	for _, net := range networks {
		// Skip system networks
		if net.Name == "bridge" || net.Name == "host" || net.Name == "none" {
			continue
		}

		// Check if it matches ene/testcontainers patterns
		pattern := opts.Pattern
		if pattern == "" {
			pattern = "testcontainers-" // Default pattern
		}

		isTestcontainers := strings.HasPrefix(net.Name, pattern) ||
			strings.HasPrefix(net.Name, "testcontainers-") ||
			(strings.Contains(net.Name, "-") && len(net.Name) > 10 && len(net.ID) > 0)

		if !isTestcontainers {
			continue
		}

		// Parse creation time
		// net.Created is already a time.Time in the network.Summary struct
		created := net.Created

		age := now.Sub(created)

		// Apply age filter
		if opts.OlderThan > 0 && age < opts.OlderThan {
			continue
		}

		// Use NetworkInspect for accurate container count
		// The NetworkList may not populate Containers map consistently
		containerCount := len(net.Containers)
		if containerCount == 0 {
			// Double-check with Inspect for more accurate data
			inspectData, err := d.dockerClient.NetworkInspect(ctx, net.ID, network.InspectOptions{})
			if err == nil {
				containerCount = len(inspectData.Containers)
			}
		}

		// Skip networks with containers unless IncludeAll is set
		if !opts.IncludeAll && containerCount > 0 {
			continue
		}

		orphaned = append(orphaned, OrphanedNetwork{
			ID:               net.ID,
			Name:             net.Name,
			Created:          created,
			Age:              age,
			Containers:       containerCount,
			Driver:           net.Driver,
			Scope:            net.Scope,
			IsTestcontainers: isTestcontainers,
		})
	}

	return orphaned, nil
}

// DiscoverOrphanedContainers finds Docker containers that appear to be orphaned.
// Containers are considered orphaned if:
// - They match ene/testcontainers naming patterns or labels
// - They are stopped/exited (unless IncludeAll is true)
// - They are older than OlderThan threshold (if specified)
func (d *ResourceDiscoverer) DiscoverOrphanedContainers(ctx context.Context, opts DiscoverOptions) ([]OrphanedContainer, error) {
	// List all containers (including stopped ones)
	listOpts := container.ListOptions{
		All: true,
	}

	// Add filter for ene-related containers if we have a pattern
	if opts.Pattern != "" {
		listOpts.Filters = filters.NewArgs(
			filters.Arg("name", opts.Pattern),
		)
	}

	containers, err := d.dockerClient.ContainerList(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	orphaned := make([]OrphanedContainer, 0)
	now := time.Now()

	for _, cont := range containers {
		// Check if it's an ene/testcontainers container
		isEne := false

		// Check name patterns
		for _, name := range cont.Names {
			// Remove leading slash from container names
			cleanName := strings.TrimPrefix(name, "/")

			if strings.Contains(cleanName, "testcontainers") ||
				strings.Contains(cleanName, "ene-") ||
				strings.HasPrefix(cont.Image, "ene-") {
				isEne = true
				break
			}
		}

		// Check labels
		if cont.Labels != nil {
			if _, ok := cont.Labels["org.testcontainers"]; ok {
				isEne = true
			}
			if _, ok := cont.Labels["ene.test"]; ok {
				isEne = true
			}
		}

		if !isEne {
			continue
		}

		// Parse creation time
		created := time.Unix(cont.Created, 0)
		age := now.Sub(created)

		// Apply age filter
		if opts.OlderThan > 0 && age < opts.OlderThan {
			continue
		}

		// Skip running containers unless IncludeAll is set
		if !opts.IncludeAll && cont.State == "running" {
			continue
		}

		// Get primary name (first one, without leading slash)
		name := "unknown"
		if len(cont.Names) > 0 {
			name = strings.TrimPrefix(cont.Names[0], "/")
		}

		orphaned = append(orphaned, OrphanedContainer{
			ID:      cont.ID,
			Name:    name,
			Created: created,
			Age:     age,
			Status:  cont.Status,
			State:   cont.State,
			Image:   cont.Image,
			IsEne:   isEne,
		})
	}

	return orphaned, nil
}

// DiscoverAll discovers both orphaned networks and containers.
func (d *ResourceDiscoverer) DiscoverAll(ctx context.Context, opts DiscoverOptions) ([]OrphanedNetwork, []OrphanedContainer, error) {
	networks, err := d.DiscoverOrphanedNetworks(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("discover networks: %w", err)
	}

	containers, err := d.DiscoverOrphanedContainers(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("discover containers: %w", err)
	}

	return networks, containers, nil
}
