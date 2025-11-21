package e2eframe

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

// BenchmarkCleanupRegistry_Register measures the cost of registering a resource
func BenchmarkCleanupRegistry_Register(b *testing.B) {
	registry := NewCleanupRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mock := &MockCleanable{
			resourceType: "container",
			resourceID:   fmt.Sprintf("container-%d", i),
		}
		registry.Register(mock)
	}
}

// BenchmarkCleanupRegistry_RegisterConcurrent measures concurrent registration performance
func BenchmarkCleanupRegistry_RegisterConcurrent(b *testing.B) {
	registry := NewCleanupRegistry()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			mock := &MockCleanable{
				resourceType: "container",
				resourceID:   fmt.Sprintf("container-%d", i),
			}
			registry.Register(mock)
			i++
		}
	})
}

// BenchmarkCleanupRegistry_Count measures the cost of counting resources
func BenchmarkCleanupRegistry_Count(b *testing.B) {
	registry := NewCleanupRegistry()

	// Pre-populate with 100 resources
	for i := 0; i < 100; i++ {
		mock := &MockCleanable{
			resourceType: "container",
			resourceID:   fmt.Sprintf("container-%d", i),
		}
		registry.Register(mock)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.Count()
	}
}

// BenchmarkCleanupRegistry_CountByType measures the cost of counting by type
func BenchmarkCleanupRegistry_CountByType(b *testing.B) {
	registry := NewCleanupRegistry()

	// Pre-populate with 100 resources across 4 types
	for i := 0; i < 100; i++ {
		resourceType := []string{"container", "network", "volume", "file"}[i%4]
		mock := &MockCleanable{
			resourceType: resourceType,
			resourceID:   fmt.Sprintf("%s-%d", resourceType, i),
		}
		registry.Register(mock)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.CountByType("container")
	}
}

// BenchmarkCleanupRegistry_ListByType measures the cost of listing by type
func BenchmarkCleanupRegistry_ListByType(b *testing.B) {
	registry := NewCleanupRegistry()

	// Pre-populate with 100 containers
	for i := 0; i < 100; i++ {
		mock := &MockCleanable{
			resourceType: "container",
			resourceID:   fmt.Sprintf("container-%d", i),
		}
		registry.Register(mock)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.ListByType("container")
	}
}

// BenchmarkCleanupRegistry_CleanupAll measures the cost of cleaning up all resources
func BenchmarkCleanupRegistry_CleanupAll(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		registry := NewCleanupRegistry()

		// Register 10 resources of different types
		for j := 0; j < 10; j++ {
			resourceType := []string{"container", "volume", "network", "file"}[j%4]
			mock := &MockCleanable{
				resourceType: resourceType,
				resourceID:   fmt.Sprintf("%s-%d", resourceType, j),
			}
			registry.Register(mock)
		}

		b.StartTimer()
		_ = registry.CleanupAll(ctx)
	}
}

// BenchmarkCleanupRegistry_CleanupAllWithErrors measures cleanup performance when some resources fail
func BenchmarkCleanupRegistry_CleanupAllWithErrors(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		registry := NewCleanupRegistry()

		// Register 10 resources, half of which fail
		for j := 0; j < 10; j++ {
			resourceType := []string{"container", "volume", "network", "file"}[j%4]
			mock := &MockCleanable{
				resourceType: resourceType,
				resourceID:   fmt.Sprintf("%s-%d", resourceType, j),
				cleanupFunc: func(ctx context.Context) error {
					if j%2 == 0 {
						return fmt.Errorf("cleanup failed")
					}
					return nil
				},
			}
			registry.Register(mock)
		}

		b.StartTimer()
		_ = registry.CleanupAll(ctx)
	}
}

// BenchmarkCleanupRegistry_CleanupByType measures the cost of cleaning up by type
func BenchmarkCleanupRegistry_CleanupByType(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		registry := NewCleanupRegistry()

		// Register 25 containers
		for j := 0; j < 25; j++ {
			mock := &MockCleanable{
				resourceType: "container",
				resourceID:   fmt.Sprintf("container-%d", j),
			}
			registry.Register(mock)
		}

		b.StartTimer()
		_ = registry.CleanupByType(ctx, "container")
	}
}

// BenchmarkCleanableNetwork_Metadata measures metadata extraction cost
func BenchmarkCleanableNetwork_Metadata(b *testing.B) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "test-network-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cleanable.Metadata()
	}
}

// BenchmarkCleanableContainer_Metadata measures metadata extraction cost
func BenchmarkCleanableContainer_Metadata(b *testing.B) {
	container := &MockContainer{
		containerID: "test-container-id",
	}

	cleanable := NewCleanableContainer(container, "test-unit")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cleanable.Metadata()
	}
}

// BenchmarkCleanupRegistry_ResourceTypes measures the cost of getting all resource types
func BenchmarkCleanupRegistry_ResourceTypes(b *testing.B) {
	registry := NewCleanupRegistry()

	// Pre-populate with resources of 4 types
	for i := 0; i < 100; i++ {
		resourceType := []string{"container", "network", "volume", "file"}[i%4]
		mock := &MockCleanable{
			resourceType: resourceType,
			resourceID:   fmt.Sprintf("%s-%d", resourceType, i),
		}
		registry.Register(mock)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.ResourceTypes()
	}
}

// BenchmarkCleanupRegistry_Clear measures the cost of clearing the registry
func BenchmarkCleanupRegistry_Clear(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		registry := NewCleanupRegistry()

		// Pre-populate with 100 resources
		for j := 0; j < 100; j++ {
			mock := &MockCleanable{
				resourceType: "container",
				resourceID:   fmt.Sprintf("container-%d", j),
			}
			registry.Register(mock)
		}

		b.StartTimer()
		registry.Clear()
	}
}

// BenchmarkCleanupError_Error measures error formatting cost
func BenchmarkCleanupError_Error(b *testing.B) {
	err := &CleanupError{
		ResourceType: "container",
		ResourceID:   "test-container-id",
		Err:          fmt.Errorf("connection failed"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Error()
	}
}

// BenchmarkNewCleanableNetwork measures the cost of wrapping a network
func BenchmarkNewCleanableNetwork(b *testing.B) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "test-network-id",
		Driver: "bridge",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewCleanableNetwork(network)
	}
}

// BenchmarkNewCleanableContainer measures the cost of wrapping a container
func BenchmarkNewCleanableContainer(b *testing.B) {
	container := &MockContainer{
		containerID: "test-container-id",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewCleanableContainer(container, "test-unit")
	}
}
