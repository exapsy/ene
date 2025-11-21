package e2eframe

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewCleanupRegistry(t *testing.T) {
	registry := NewCleanupRegistry()

	if registry == nil {
		t.Fatal("NewCleanupRegistry() returned nil")
	}

	if registry.resources == nil {
		t.Error("registry.resources is nil")
	}

	if len(registry.cleanupOrder) == 0 {
		t.Error("registry.cleanupOrder is empty")
	}

	// Verify default cleanup order
	expectedOrder := []string{"container", "volume", "network", "file"}
	if len(registry.cleanupOrder) != len(expectedOrder) {
		t.Errorf("cleanupOrder length = %v, want %v", len(registry.cleanupOrder), len(expectedOrder))
	}

	for i, expected := range expectedOrder {
		if i >= len(registry.cleanupOrder) {
			break
		}
		if registry.cleanupOrder[i] != expected {
			t.Errorf("cleanupOrder[%d] = %v, want %v", i, registry.cleanupOrder[i], expected)
		}
	}
}

func TestCleanupRegistry_Register(t *testing.T) {
	registry := NewCleanupRegistry()

	mock := &MockCleanable{
		resourceType: "container",
		resourceID:   "test-container",
	}

	registry.Register(mock)

	if count := registry.CountByType("container"); count != 1 {
		t.Errorf("CountByType(container) = %v, want 1", count)
	}

	if total := registry.Count(); total != 1 {
		t.Errorf("Count() = %v, want 1", total)
	}
}

func TestCleanupRegistry_RegisterNil(t *testing.T) {
	registry := NewCleanupRegistry()

	// Registering nil should not panic or add anything
	registry.Register(nil)

	if count := registry.Count(); count != 0 {
		t.Errorf("Count() after Register(nil) = %v, want 0", count)
	}
}

func TestCleanupRegistry_RegisterMultipleTypes(t *testing.T) {
	registry := NewCleanupRegistry()

	registry.Register(&MockCleanable{resourceType: "container", resourceID: "c1"})
	registry.Register(&MockCleanable{resourceType: "container", resourceID: "c2"})
	registry.Register(&MockCleanable{resourceType: "network", resourceID: "n1"})
	registry.Register(&MockCleanable{resourceType: "volume", resourceID: "v1"})

	if count := registry.CountByType("container"); count != 2 {
		t.Errorf("CountByType(container) = %v, want 2", count)
	}

	if count := registry.CountByType("network"); count != 1 {
		t.Errorf("CountByType(network) = %v, want 1", count)
	}

	if count := registry.CountByType("volume"); count != 1 {
		t.Errorf("CountByType(volume) = %v, want 1", count)
	}

	if total := registry.Count(); total != 4 {
		t.Errorf("Count() = %v, want 4", total)
	}
}

func TestCleanupRegistry_CleanupAll_Empty(t *testing.T) {
	registry := NewCleanupRegistry()
	ctx := context.Background()

	err := registry.CleanupAll(ctx)
	if err != nil {
		t.Errorf("CleanupAll() on empty registry error = %v, want nil", err)
	}
}

func TestCleanupRegistry_CleanupAll_Success(t *testing.T) {
	registry := NewCleanupRegistry()

	mock1 := &MockCleanable{resourceType: "container", resourceID: "c1"}
	mock2 := &MockCleanable{resourceType: "network", resourceID: "n1"}

	registry.Register(mock1)
	registry.Register(mock2)

	ctx := context.Background()
	err := registry.CleanupAll(ctx)
	if err != nil {
		t.Errorf("CleanupAll() error = %v, want nil", err)
	}

	// Verify both were cleaned up
	if mock1.cleanupCount != 1 {
		t.Errorf("container cleanup count = %v, want 1", mock1.cleanupCount)
	}

	if mock2.cleanupCount != 1 {
		t.Errorf("network cleanup count = %v, want 1", mock2.cleanupCount)
	}

	// Verify registry is now empty
	if count := registry.Count(); count != 0 {
		t.Errorf("Count() after CleanupAll() = %v, want 0", count)
	}
}

func TestCleanupRegistry_CleanupAll_RespectOrder(t *testing.T) {
	registry := NewCleanupRegistry()

	var cleanupOrder []string
	var mu sync.Mutex

	// Create mocks that record cleanup order
	container := &MockCleanable{
		resourceType: "container",
		resourceID:   "c1",
		cleanupFunc: func(ctx context.Context) error {
			mu.Lock()
			cleanupOrder = append(cleanupOrder, "container")
			mu.Unlock()
			return nil
		},
	}

	network := &MockCleanable{
		resourceType: "network",
		resourceID:   "n1",
		cleanupFunc: func(ctx context.Context) error {
			mu.Lock()
			cleanupOrder = append(cleanupOrder, "network")
			mu.Unlock()
			return nil
		},
	}

	volume := &MockCleanable{
		resourceType: "volume",
		resourceID:   "v1",
		cleanupFunc: func(ctx context.Context) error {
			mu.Lock()
			cleanupOrder = append(cleanupOrder, "volume")
			mu.Unlock()
			return nil
		},
	}

	file := &MockCleanable{
		resourceType: "file",
		resourceID:   "f1",
		cleanupFunc: func(ctx context.Context) error {
			mu.Lock()
			cleanupOrder = append(cleanupOrder, "file")
			mu.Unlock()
			return nil
		},
	}

	// Register in reverse order to test that cleanup order is enforced
	registry.Register(file)
	registry.Register(network)
	registry.Register(volume)
	registry.Register(container)

	ctx := context.Background()
	err := registry.CleanupAll(ctx)
	if err != nil {
		t.Errorf("CleanupAll() error = %v, want nil", err)
	}

	// Verify cleanup happened in the correct order
	expectedOrder := []string{"container", "volume", "network", "file"}
	if len(cleanupOrder) != len(expectedOrder) {
		t.Fatalf("cleanup order length = %v, want %v", len(cleanupOrder), len(expectedOrder))
	}

	for i, expected := range expectedOrder {
		if cleanupOrder[i] != expected {
			t.Errorf("cleanupOrder[%d] = %v, want %v", i, cleanupOrder[i], expected)
		}
	}
}

func TestCleanupRegistry_CleanupAll_WithErrors(t *testing.T) {
	registry := NewCleanupRegistry()

	failingMock := &MockCleanable{
		resourceType: "container",
		resourceID:   "failing-container",
		cleanupFunc: func(ctx context.Context) error {
			return errors.New("cleanup failed")
		},
	}

	successMock := &MockCleanable{
		resourceType: "container",
		resourceID:   "success-container",
	}

	registry.Register(failingMock)
	registry.Register(successMock)

	ctx := context.Background()
	err := registry.CleanupAll(ctx)

	// Should return an error
	if err == nil {
		t.Fatal("CleanupAll() with failing resource error = nil, want error")
	}

	// But should still attempt all cleanups
	if successMock.cleanupCount != 1 {
		t.Errorf("success mock cleanup count = %v, want 1", successMock.cleanupCount)
	}

	if failingMock.cleanupCount != 1 {
		t.Errorf("failing mock cleanup count = %v, want 1", failingMock.cleanupCount)
	}

	// Registry should be cleared even with errors
	if count := registry.Count(); count != 0 {
		t.Errorf("Count() after CleanupAll() with errors = %v, want 0", count)
	}
}

func TestCleanupRegistry_CleanupByType(t *testing.T) {
	registry := NewCleanupRegistry()

	container := &MockCleanable{resourceType: "container", resourceID: "c1"}
	network := &MockCleanable{resourceType: "network", resourceID: "n1"}

	registry.Register(container)
	registry.Register(network)

	ctx := context.Background()

	// Cleanup only containers
	err := registry.CleanupByType(ctx, "container")
	if err != nil {
		t.Errorf("CleanupByType(container) error = %v, want nil", err)
	}

	// Container should be cleaned up
	if container.cleanupCount != 1 {
		t.Errorf("container cleanup count = %v, want 1", container.cleanupCount)
	}

	// Network should not be cleaned up yet
	if network.cleanupCount != 0 {
		t.Errorf("network cleanup count = %v, want 0", network.cleanupCount)
	}

	// Only network should remain
	if count := registry.CountByType("container"); count != 0 {
		t.Errorf("CountByType(container) after cleanup = %v, want 0", count)
	}

	if count := registry.CountByType("network"); count != 1 {
		t.Errorf("CountByType(network) = %v, want 1", count)
	}
}

func TestCleanupRegistry_CleanupByType_NonExistent(t *testing.T) {
	registry := NewCleanupRegistry()
	ctx := context.Background()

	// Cleanup non-existent type should not error
	err := registry.CleanupByType(ctx, "nonexistent")
	if err != nil {
		t.Errorf("CleanupByType(nonexistent) error = %v, want nil", err)
	}
}

func TestCleanupRegistry_ListByType(t *testing.T) {
	registry := NewCleanupRegistry()

	mock1 := &MockCleanable{resourceType: "container", resourceID: "c1"}
	mock2 := &MockCleanable{resourceType: "container", resourceID: "c2"}
	mock3 := &MockCleanable{resourceType: "network", resourceID: "n1"}

	registry.Register(mock1)
	registry.Register(mock2)
	registry.Register(mock3)

	containers := registry.ListByType("container")
	if len(containers) != 2 {
		t.Errorf("ListByType(container) length = %v, want 2", len(containers))
	}

	networks := registry.ListByType("network")
	if len(networks) != 1 {
		t.Errorf("ListByType(network) length = %v, want 1", len(networks))
	}

	// Non-existent type should return nil
	nonExistent := registry.ListByType("nonexistent")
	if nonExistent != nil {
		t.Errorf("ListByType(nonexistent) = %v, want nil", nonExistent)
	}
}

func TestCleanupRegistry_ListByType_ReturnsCopy(t *testing.T) {
	registry := NewCleanupRegistry()

	mock := &MockCleanable{resourceType: "container", resourceID: "c1"}
	registry.Register(mock)

	// Get the list
	list1 := registry.ListByType("container")
	list2 := registry.ListByType("container")

	// Modify one list
	list1[0] = nil

	// Other list should be unaffected
	if list2[0] == nil {
		t.Error("ListByType() returns shared slice, want independent copy")
	}
}

func TestCleanupRegistry_ResourceTypes(t *testing.T) {
	registry := NewCleanupRegistry()

	registry.Register(&MockCleanable{resourceType: "container", resourceID: "c1"})
	registry.Register(&MockCleanable{resourceType: "network", resourceID: "n1"})
	registry.Register(&MockCleanable{resourceType: "volume", resourceID: "v1"})

	types := registry.ResourceTypes()
	if len(types) != 3 {
		t.Errorf("ResourceTypes() length = %v, want 3", len(types))
	}

	// Check that all expected types are present
	typeSet := make(map[string]bool)
	for _, typ := range types {
		typeSet[typ] = true
	}

	for _, expected := range []string{"container", "network", "volume"} {
		if !typeSet[expected] {
			t.Errorf("ResourceTypes() missing %v", expected)
		}
	}
}

func TestCleanupRegistry_Clear(t *testing.T) {
	registry := NewCleanupRegistry()

	mock := &MockCleanable{resourceType: "container", resourceID: "c1"}
	registry.Register(mock)

	if count := registry.Count(); count != 1 {
		t.Fatalf("Count() before Clear() = %v, want 1", count)
	}

	registry.Clear()

	if count := registry.Count(); count != 0 {
		t.Errorf("Count() after Clear() = %v, want 0", count)
	}

	// Cleanup should not have been called
	if mock.cleanupCount != 0 {
		t.Errorf("cleanup count after Clear() = %v, want 0", mock.cleanupCount)
	}
}

func TestCleanupRegistry_ThreadSafety(t *testing.T) {
	registry := NewCleanupRegistry()
	ctx := context.Background()

	const numGoroutines = 10
	const resourcesPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrently register resources
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < resourcesPerGoroutine; j++ {
				mock := &MockCleanable{
					resourceType: "container",
					resourceID:   fmt.Sprintf("c-%d-%d", id, j),
				}
				registry.Register(mock)
			}
		}(i)
	}

	wg.Wait()

	expectedCount := numGoroutines * resourcesPerGoroutine
	if count := registry.Count(); count != expectedCount {
		t.Errorf("Count() after concurrent Register() = %v, want %v", count, expectedCount)
	}

	// Cleanup all concurrently registered resources
	err := registry.CleanupAll(ctx)
	if err != nil {
		t.Errorf("CleanupAll() after concurrent registration error = %v, want nil", err)
	}

	if count := registry.Count(); count != 0 {
		t.Errorf("Count() after CleanupAll() = %v, want 0", count)
	}
}

func TestCleanupRegistry_ConcurrentReads(t *testing.T) {
	registry := NewCleanupRegistry()

	// Register some initial resources
	for i := 0; i < 10; i++ {
		registry.Register(&MockCleanable{
			resourceType: "container",
			resourceID:   fmt.Sprintf("c-%d", i),
		})
	}

	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrently read from registry
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = registry.Count()
				_ = registry.CountByType("container")
				_ = registry.ResourceTypes()
				_ = registry.ListByType("container")
			}
		}()
	}

	wg.Wait()

	// Verify data is still consistent
	if count := registry.Count(); count != 10 {
		t.Errorf("Count() after concurrent reads = %v, want 10", count)
	}
}

func TestCleanupRegistry_ContextTimeout(t *testing.T) {
	registry := NewCleanupRegistry()

	mock := &MockCleanable{
		resourceType: "container",
		resourceID:   "slow-container",
		cleanupFunc: func(ctx context.Context) error {
			select {
			case <-time.After(100 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	registry.Register(mock)

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := registry.CleanupAll(ctx)
	if err == nil {
		t.Fatal("CleanupAll() with timeout error = nil, want error")
	}

	// Should contain context deadline exceeded error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("CleanupAll() error does not contain context.DeadlineExceeded: %v", err)
	}
}

func TestCleanupRegistry_UnorderedTypes(t *testing.T) {
	registry := NewCleanupRegistry()

	var cleanupOrder []string
	var mu sync.Mutex

	// Register a resource type not in the default order
	customType := &MockCleanable{
		resourceType: "custom",
		resourceID:   "custom-1",
		cleanupFunc: func(ctx context.Context) error {
			mu.Lock()
			cleanupOrder = append(cleanupOrder, "custom")
			mu.Unlock()
			return nil
		},
	}

	file := &MockCleanable{
		resourceType: "file",
		resourceID:   "f1",
		cleanupFunc: func(ctx context.Context) error {
			mu.Lock()
			cleanupOrder = append(cleanupOrder, "file")
			mu.Unlock()
			return nil
		},
	}

	registry.Register(customType)
	registry.Register(file)

	ctx := context.Background()
	err := registry.CleanupAll(ctx)
	if err != nil {
		t.Errorf("CleanupAll() error = %v, want nil", err)
	}

	// File should be cleaned up first (it's in the order)
	// Custom should be cleaned up after (not in the order)
	if len(cleanupOrder) != 2 {
		t.Fatalf("cleanup order length = %v, want 2", len(cleanupOrder))
	}

	if cleanupOrder[0] != "file" {
		t.Errorf("cleanupOrder[0] = %v, want file", cleanupOrder[0])
	}

	if cleanupOrder[1] != "custom" {
		t.Errorf("cleanupOrder[1] = %v, want custom", cleanupOrder[1])
	}
}
