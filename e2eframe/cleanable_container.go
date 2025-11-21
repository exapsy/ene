package e2eframe

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
)

// CleanableContainer wraps a Docker container created by testcontainers.
// It implements the Cleanable interface to allow unified cleanup through
// the CleanupRegistry.
type CleanableContainer struct {
	container testcontainers.Container
	unitName  string
}

// NewCleanableContainer creates a cleanable wrapper for a Docker container.
// The unitName is used for identification and logging (e.g., "postgres-main", "http-app").
func NewCleanableContainer(container testcontainers.Container, unitName string) *CleanableContainer {
	if container == nil {
		return nil
	}

	return &CleanableContainer{
		container: container,
		unitName:  unitName,
	}
}

// Cleanup terminates and removes the Docker container.
// Uses Terminate() instead of Stop() to ensure the container is fully removed,
// not just stopped. This is critical for proper network cleanup.
func (c *CleanableContainer) Cleanup(ctx context.Context) error {
	if c.container == nil {
		return nil
	}

	if err := c.container.Terminate(ctx); err != nil {
		return fmt.Errorf("cleanup container %s: %w", c.unitName, err)
	}

	return nil
}

// ResourceType returns "container" to identify this as a container resource.
func (c *CleanableContainer) ResourceType() string {
	return "container"
}

// ResourceID returns the unit name for identification.
func (c *CleanableContainer) ResourceID() string {
	return c.unitName
}

// Metadata returns additional information about the container.
func (c *CleanableContainer) Metadata() map[string]string {
	metadata := map[string]string{
		"unit": c.unitName,
	}

	if c.container != nil {
		// Get container ID if available
		if containerID := c.container.GetContainerID(); containerID != "" {
			metadata["id"] = containerID
		}
	}

	return metadata
}
