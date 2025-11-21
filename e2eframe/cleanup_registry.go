package e2eframe

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// CleanupRegistry maintains a collection of cleanable resources and handles
// their cleanup in the correct order to respect dependencies.
//
// Resources are grouped by type, and cleanup happens in a predefined order:
//  1. containers (must be removed before networks)
//  2. volumes (can be removed after containers stop)
//  3. networks (can be removed after containers detach)
//  4. files (cleaned up last)
//
// Thread-safe: Multiple goroutines can safely register resources concurrently.
type CleanupRegistry struct {
	resources map[string][]Cleanable
	mu        sync.RWMutex

	// cleanupOrder defines the order in which resource types are cleaned up.
	// Resources not in this list are cleaned up after these, in arbitrary order.
	cleanupOrder []string
}

// NewCleanupRegistry creates a new cleanup registry with default settings.
func NewCleanupRegistry() *CleanupRegistry {
	return &CleanupRegistry{
		resources: make(map[string][]Cleanable),
		cleanupOrder: []string{
			"container", // Must be first - detaches from networks
			"volume",    // After containers stop using them
			"network",   // After containers are removed
			"file",      // Last - logs and temp files
		},
	}
}

// Register adds a cleanable resource to the registry.
// Resources are grouped by their ResourceType for ordered cleanup.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func (r *CleanupRegistry) Register(c Cleanable) {
	if c == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	resourceType := c.ResourceType()
	r.resources[resourceType] = append(r.resources[resourceType], c)
}

// CleanupAll cleans up all registered resources in the correct order.
// Returns a multi-error containing all cleanup failures, or nil if all succeeded.
//
// Cleanup continues even if individual resources fail - all resources are attempted.
// The context is passed to each Cleanable's Cleanup method for timeout control.
func (r *CleanupRegistry) CleanupAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var allErrors []error

	// First, cleanup resources in the defined order
	for _, resourceType := range r.cleanupOrder {
		if resources, ok := r.resources[resourceType]; ok {
			if err := r.cleanupResourceList(ctx, resourceType, resources); err != nil {
				allErrors = append(allErrors, err)
			}
			// Remove from map so we don't clean them up again
			delete(r.resources, resourceType)
		}
	}

	// Then cleanup any remaining resource types not in the order list
	for resourceType, resources := range r.resources {
		if err := r.cleanupResourceList(ctx, resourceType, resources); err != nil {
			allErrors = append(allErrors, err)
		}
	}

	// Clear all resources
	r.resources = make(map[string][]Cleanable)

	if len(allErrors) > 0 {
		return errors.Join(allErrors...)
	}
	return nil
}

// cleanupResourceList cleans up all resources in the given list.
// Returns an error containing all failures for this resource type.
func (r *CleanupRegistry) cleanupResourceList(ctx context.Context, resourceType string, resources []Cleanable) error {
	if len(resources) == 0 {
		return nil
	}

	var errs []error
	for _, resource := range resources {
		if err := resource.Cleanup(ctx); err != nil {
			errs = append(errs, &CleanupError{
				ResourceType: resourceType,
				ResourceID:   resource.ResourceID(),
				Err:          err,
			})
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to cleanup %d %s(s): %w",
			len(errs), resourceType, errors.Join(errs...))
	}
	return nil
}

// CleanupByType cleans up all resources of a specific type.
// Returns an error if any cleanup fails, but continues attempting all resources.
func (r *CleanupRegistry) CleanupByType(ctx context.Context, resourceType string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	resources, ok := r.resources[resourceType]
	if !ok || len(resources) == 0 {
		return nil // No resources of this type
	}

	err := r.cleanupResourceList(ctx, resourceType, resources)

	// Remove these resources from the registry
	delete(r.resources, resourceType)

	return err
}

// ListByType returns all registered resources of a specific type.
// Returns a copy of the slice to prevent external modification.
func (r *CleanupRegistry) ListByType(resourceType string) []Cleanable {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resources, ok := r.resources[resourceType]
	if !ok {
		return nil
	}

	// Return a copy to prevent external modification
	result := make([]Cleanable, len(resources))
	copy(result, resources)
	return result
}

// ResourceTypes returns all resource types currently registered.
// Returns a new slice containing the types in arbitrary order.
func (r *CleanupRegistry) ResourceTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.resources))
	for resourceType := range r.resources {
		types = append(types, resourceType)
	}
	return types
}

// Count returns the total number of registered resources.
func (r *CleanupRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := 0
	for _, resources := range r.resources {
		total += len(resources)
	}
	return total
}

// CountByType returns the number of registered resources of a specific type.
func (r *CleanupRegistry) CountByType(resourceType string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resources, ok := r.resources[resourceType]
	if !ok {
		return 0
	}
	return len(resources)
}

// Clear removes all registered resources without cleaning them up.
// This should only be used in testing or error recovery scenarios.
func (r *CleanupRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.resources = make(map[string][]Cleanable)
}
