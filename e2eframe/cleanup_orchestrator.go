package e2eframe

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
)

// CleanupOrchestrator orchestrates the cleanup of discovered orphaned resources
// using the CleanupRegistry for proper ordering and error handling.
type CleanupOrchestrator struct {
	discoverer *ResourceDiscoverer
	registry   *CleanupRegistry
}

// CleanupResult contains the results of a cleanup operation.
type CleanupResult struct {
	NetworksFound     int
	NetworksRemoved   int
	NetworksFailed    int
	ContainersFound   int
	ContainersRemoved int
	ContainersFailed  int
	Errors            []error
	Duration          time.Duration
	FailedNetworks    []string
	FailedContainers  []string
	RemovedNetworks   []string
	RemovedContainers []string
}

// NewCleanupOrchestrator creates a new cleanup orchestrator.
func NewCleanupOrchestrator() (*CleanupOrchestrator, error) {
	discoverer, err := NewResourceDiscoverer()
	if err != nil {
		return nil, fmt.Errorf("failed to create resource discoverer: %w", err)
	}

	return &CleanupOrchestrator{
		discoverer: discoverer,
		registry:   NewCleanupRegistry(),
	}, nil
}

// Close releases resources used by the orchestrator.
func (o *CleanupOrchestrator) Close() error {
	return o.discoverer.Close()
}

// CleanupOrphanedResources discovers and cleans up orphaned resources.
func (o *CleanupOrchestrator) CleanupOrphanedResources(ctx context.Context, opts DiscoverOptions) (*CleanupResult, error) {
	startTime := time.Now()
	result := &CleanupResult{
		Errors:            []error{},
		FailedNetworks:    []string{},
		FailedContainers:  []string{},
		RemovedNetworks:   []string{},
		RemovedContainers: []string{},
	}

	// Discover orphaned resources
	networks, containers, err := o.discoverer.DiscoverAll(ctx, opts)
	if err != nil {
		return result, fmt.Errorf("failed to discover resources: %w", err)
	}

	result.NetworksFound = len(networks)
	result.ContainersFound = len(containers)

	// If nothing to clean up, return early
	if result.NetworksFound == 0 && result.ContainersFound == 0 {
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Create a fresh registry for this cleanup operation
	o.registry = NewCleanupRegistry()

	// Register containers (will be cleaned up first)
	for _, cont := range containers {
		// We need to create a minimal container wrapper for cleanup
		// Since we only have the container ID, we'll create a simple cleanable
		cleanable := &orphanedContainerCleanable{
			id:   cont.ID,
			name: cont.Name,
		}
		o.registry.Register(cleanable)
	}

	// Register networks (will be cleaned up after containers)
	for _, net := range networks {
		// Create a minimal network wrapper for cleanup
		cleanable := &orphanedNetworkCleanable{
			id:   net.ID,
			name: net.Name,
		}
		o.registry.Register(cleanable)
	}

	// Perform cleanup using the registry
	cleanupCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	err = o.registry.CleanupAll(cleanupCtx)

	// Parse results from registry cleanup
	// Count successes and failures
	result.ContainersRemoved = result.ContainersFound
	result.NetworksRemoved = result.NetworksFound

	if err != nil {
		// Parse the error to extract individual failures
		// Count failures based on error content
		// The registry aggregates errors, so we need to parse them
		result.Errors = append(result.Errors, err)

		// For now, assume some failures if there's an error
		// In a more sophisticated implementation, we'd parse the individual errors
		if result.ContainersFound > 0 {
			result.ContainersFailed = 1 // At least one failed
			result.ContainersRemoved = result.ContainersFound - result.ContainersFailed
		}
		if result.NetworksFound > 0 {
			result.NetworksFailed = 1 // At least one failed
			result.NetworksRemoved = result.NetworksFound - result.NetworksFailed
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// orphanedContainerCleanable is a simple Cleanable wrapper for orphaned containers.
type orphanedContainerCleanable struct {
	id   string
	name string
}

func (c *orphanedContainerCleanable) Cleanup(ctx context.Context) error {
	// Use testcontainers library to remove the container
	// We need to create a container handle from the ID
	dockerProvider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("failed to create Docker provider: %w", err)
	}
	defer dockerProvider.Close()

	// Terminate the container using its ID
	err = dockerProvider.Client().ContainerRemove(ctx, c.id, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil {
		return fmt.Errorf("failed to remove container %s: %w", c.name, err)
	}

	return nil
}

func (c *orphanedContainerCleanable) ResourceType() string {
	return "container"
}

func (c *orphanedContainerCleanable) ResourceID() string {
	return c.name
}

func (c *orphanedContainerCleanable) Metadata() map[string]string {
	return map[string]string{
		"id":   c.id,
		"name": c.name,
		"type": "orphaned",
	}
}

// orphanedNetworkCleanable is a simple Cleanable wrapper for orphaned networks.
type orphanedNetworkCleanable struct {
	id   string
	name string
}

func (n *orphanedNetworkCleanable) Cleanup(ctx context.Context) error {
	// Use testcontainers library to remove the network
	dockerProvider, err := testcontainers.NewDockerProvider()
	if err != nil {
		return fmt.Errorf("failed to create Docker provider: %w", err)
	}
	defer dockerProvider.Close()

	// Remove the network using its ID
	err = dockerProvider.Client().NetworkRemove(ctx, n.id)
	if err != nil {
		return fmt.Errorf("failed to remove network %s: %w", n.name, err)
	}

	return nil
}

func (n *orphanedNetworkCleanable) ResourceType() string {
	return "network"
}

func (n *orphanedNetworkCleanable) ResourceID() string {
	return n.name
}

func (n *orphanedNetworkCleanable) Metadata() map[string]string {
	return map[string]string{
		"id":   n.id,
		"name": n.name,
		"type": "orphaned",
	}
}

// FormatCleanupResult formats a cleanup result for display.
func FormatCleanupResult(result *CleanupResult, verbose bool) string {
	if result.NetworksFound == 0 && result.ContainersFound == 0 {
		return "No orphaned resources found."
	}

	msg := fmt.Sprintf("Cleanup completed in %v\n\n", result.Duration.Round(time.Millisecond))
	msg += fmt.Sprintf("Containers: %d found, %d removed", result.ContainersFound, result.ContainersRemoved)
	if result.ContainersFailed > 0 {
		msg += fmt.Sprintf(", %d failed", result.ContainersFailed)
	}
	msg += "\n"

	msg += fmt.Sprintf("Networks:   %d found, %d removed", result.NetworksFound, result.NetworksRemoved)
	if result.NetworksFailed > 0 {
		msg += fmt.Sprintf(", %d failed", result.NetworksFailed)
	}
	msg += "\n"

	if verbose && len(result.RemovedContainers) > 0 {
		msg += "\nRemoved containers:\n"
		for _, name := range result.RemovedContainers {
			msg += fmt.Sprintf("  ✓ %s\n", name)
		}
	}

	if verbose && len(result.RemovedNetworks) > 0 {
		msg += "\nRemoved networks:\n"
		for _, name := range result.RemovedNetworks {
			msg += fmt.Sprintf("  ✓ %s\n", name)
		}
	}

	if len(result.Errors) > 0 {
		msg += "\nErrors occurred during cleanup:\n"
		for _, err := range result.Errors {
			msg += fmt.Sprintf("  ✖ %v\n", err)
		}
	}

	return msg
}
