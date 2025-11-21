package e2eframe

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
)

// CleanableNetwork wraps a Docker network created by testcontainers.
// It implements the Cleanable interface to allow unified cleanup through
// the CleanupRegistry.
type CleanableNetwork struct {
	network *testcontainers.DockerNetwork
	name    string
}

// NewCleanableNetwork creates a cleanable wrapper for a Docker network.
func NewCleanableNetwork(network *testcontainers.DockerNetwork) *CleanableNetwork {
	if network == nil {
		return nil
	}

	return &CleanableNetwork{
		network: network,
		name:    network.Name,
	}
}

// Cleanup removes the Docker network.
// Uses ForceCleanupNetwork to handle edge cases like stuck containers.
func (n *CleanableNetwork) Cleanup(ctx context.Context) error {
	if n.network == nil {
		return nil
	}

	if err := ForceCleanupNetwork(ctx, n.network); err != nil {
		return fmt.Errorf("cleanup network %s: %w", n.name, err)
	}

	return nil
}

// ResourceType returns "network" to identify this as a network resource.
func (n *CleanableNetwork) ResourceType() string {
	return "network"
}

// ResourceID returns the network name for identification.
func (n *CleanableNetwork) ResourceID() string {
	return n.name
}

// Metadata returns additional information about the network.
func (n *CleanableNetwork) Metadata() map[string]string {
	metadata := map[string]string{
		"name": n.name,
	}

	if n.network != nil {
		metadata["id"] = n.network.ID
		metadata["driver"] = n.network.Driver
	}

	return metadata
}
