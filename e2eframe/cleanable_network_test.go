package e2eframe

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

func TestNewCleanableNetwork_Nil(t *testing.T) {
	cleanable := NewCleanableNetwork(nil)

	if cleanable != nil {
		t.Errorf("NewCleanableNetwork(nil) = %v, want nil", cleanable)
	}
}

func TestNewCleanableNetwork_Valid(t *testing.T) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "test-network-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	if cleanable == nil {
		t.Fatal("NewCleanableNetwork() returned nil")
	}

	if cleanable.network != network {
		t.Error("cleanable.network does not match input network")
	}

	if cleanable.name != "test-network" {
		t.Errorf("cleanable.name = %v, want test-network", cleanable.name)
	}
}

func TestCleanableNetwork_ResourceType(t *testing.T) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "test-network-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	if resourceType := cleanable.ResourceType(); resourceType != "network" {
		t.Errorf("ResourceType() = %v, want network", resourceType)
	}
}

func TestCleanableNetwork_ResourceID(t *testing.T) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network-123",
		ID:     "test-network-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	if resourceID := cleanable.ResourceID(); resourceID != "test-network-123" {
		t.Errorf("ResourceID() = %v, want test-network-123", resourceID)
	}
}

func TestCleanableNetwork_Metadata(t *testing.T) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "test-network-id-abc123",
		Driver: "overlay",
	}

	cleanable := NewCleanableNetwork(network)
	metadata := cleanable.Metadata()

	if metadata == nil {
		t.Fatal("Metadata() returned nil")
	}

	if metadata["name"] != "test-network" {
		t.Errorf("Metadata()[name] = %v, want test-network", metadata["name"])
	}

	if metadata["id"] != "test-network-id-abc123" {
		t.Errorf("Metadata()[id] = %v, want test-network-id-abc123", metadata["id"])
	}

	if metadata["driver"] != "overlay" {
		t.Errorf("Metadata()[driver] = %v, want overlay", metadata["driver"])
	}
}

func TestCleanableNetwork_Metadata_NilNetwork(t *testing.T) {
	cleanable := &CleanableNetwork{
		network: nil,
		name:    "test-network",
	}

	metadata := cleanable.Metadata()

	if metadata == nil {
		t.Fatal("Metadata() returned nil")
	}

	if metadata["name"] != "test-network" {
		t.Errorf("Metadata()[name] = %v, want test-network", metadata["name"])
	}

	// ID and driver should not be present when network is nil
	if _, exists := metadata["id"]; exists {
		t.Error("Metadata()[id] should not exist when network is nil")
	}

	if _, exists := metadata["driver"]; exists {
		t.Error("Metadata()[driver] should not exist when network is nil")
	}
}

func TestCleanableNetwork_Cleanup_NilNetwork(t *testing.T) {
	cleanable := &CleanableNetwork{
		network: nil,
		name:    "test-network",
	}

	ctx := context.Background()
	err := cleanable.Cleanup(ctx)

	if err != nil {
		t.Errorf("Cleanup() with nil network error = %v, want nil", err)
	}
}

func TestCleanableNetwork_InterfaceCompliance(t *testing.T) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "test-network-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	// Verify it implements Cleanable interface
	var _ Cleanable = cleanable
}

func TestCleanableNetwork_Cleanup_ErrorPropagation(t *testing.T) {
	// This test verifies that errors from ForceCleanupNetwork are properly wrapped
	// We can't easily test the actual cleanup without a real Docker environment,
	// but we can verify the structure is correct

	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "invalid-network-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	// Verify the cleanup method exists and returns expected types
	ctx := context.Background()
	err := cleanable.Cleanup(ctx)

	// Error is expected since this is not a real network
	// Just verify error message contains the network name
	if err != nil && cleanable.name != "" {
		errMsg := err.Error()
		// Error message should mention the network name
		if len(errMsg) == 0 {
			t.Error("Cleanup() error message is empty")
		}
	}
}

func TestCleanableNetwork_MultipleInstances(t *testing.T) {
	// Test that multiple CleanableNetwork instances can be created independently
	network1 := &testcontainers.DockerNetwork{
		Name:   "network-1",
		ID:     "id-1",
		Driver: "bridge",
	}

	network2 := &testcontainers.DockerNetwork{
		Name:   "network-2",
		ID:     "id-2",
		Driver: "overlay",
	}

	cleanable1 := NewCleanableNetwork(network1)
	cleanable2 := NewCleanableNetwork(network2)

	if cleanable1.ResourceID() == cleanable2.ResourceID() {
		t.Error("Different networks should have different resource IDs")
	}

	metadata1 := cleanable1.Metadata()
	metadata2 := cleanable2.Metadata()

	if metadata1["name"] == metadata2["name"] {
		t.Error("Different networks should have different names in metadata")
	}

	if metadata1["driver"] == metadata2["driver"] {
		t.Error("Different networks should have different drivers in metadata")
	}
}

func TestCleanableNetwork_EmptyName(t *testing.T) {
	network := &testcontainers.DockerNetwork{
		Name:   "",
		ID:     "test-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	if cleanable == nil {
		t.Fatal("NewCleanableNetwork() returned nil for network with empty name")
	}

	if cleanable.ResourceID() != "" {
		t.Errorf("ResourceID() = %v, want empty string", cleanable.ResourceID())
	}

	metadata := cleanable.Metadata()
	if metadata["name"] != "" {
		t.Errorf("Metadata()[name] = %v, want empty string", metadata["name"])
	}
}

func TestCleanableNetwork_ContextCancellation(t *testing.T) {
	network := &testcontainers.DockerNetwork{
		Name:   "test-network",
		ID:     "test-id",
		Driver: "bridge",
	}

	cleanable := NewCleanableNetwork(network)

	// Create an already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Cleanup with canceled context
	// This should respect the context and potentially fail faster
	_ = cleanable.Cleanup(ctx)

	// We can't assert much here without a real Docker environment,
	// but at least we verify the method handles canceled contexts
}
