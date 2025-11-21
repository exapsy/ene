package e2eframe

import (
	"context"
	"fmt"
)

// Cleanable represents any resource that requires cleanup.
// Resources implementing this interface can be registered in a CleanupRegistry
// and will be automatically cleaned up in the correct order.
//
// Example implementations:
//   - CleanableNetwork (wraps Docker networks)
//   - CleanableContainer (wraps Docker containers)
//   - CleanableVolume (wraps Docker volumes)
//   - CleanableFile (wraps temporary files)
type Cleanable interface {
	// Cleanup removes the resource and releases any associated system resources.
	// Implementations should be idempotent - calling Cleanup multiple times
	// should not cause errors if the resource is already cleaned up.
	//
	// The context may be used for timeout control and cancellation.
	Cleanup(ctx context.Context) error

	// ResourceType returns a string identifying the category of this resource.
	// This is used for grouping resources and controlling cleanup order.
	//
	// Standard types: "network", "container", "volume", "file"
	ResourceType() string

	// ResourceID returns a unique identifier for this specific resource.
	// This should be human-readable for logging and debugging.
	//
	// Examples:
	//   - "testcontainers-abc123" (network name)
	//   - "postgres-main" (container name)
	//   - "/tmp/test-data-xyz" (file path)
	ResourceID() string

	// Metadata returns additional information about the resource.
	// This can be used for filtering, reporting, or debugging.
	//
	// Common metadata keys:
	//   - "name": human-readable name
	//   - "id": Docker ID or system identifier
	//   - "created_at": timestamp when created
	//   - "suite": test suite that created this resource
	//   - "unit": unit that owns this resource
	Metadata() map[string]string
}

// CleanupError represents an error that occurred during cleanup.
// It includes information about which resource failed and the underlying error.
type CleanupError struct {
	ResourceType string
	ResourceID   string
	Err          error
}

func (e *CleanupError) Error() string {
	return fmt.Sprintf("failed to cleanup %s %s: %v", e.ResourceType, e.ResourceID, e.Err)
}

func (e *CleanupError) Unwrap() error {
	return e.Err
}
