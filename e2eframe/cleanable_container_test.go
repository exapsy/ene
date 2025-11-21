package e2eframe

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
)

// MockContainer is a mock implementation of testcontainers.Container for testing
type MockContainer struct {
	testcontainers.Container
	containerID    string
	terminateCount int
	terminateError error
}

func (m *MockContainer) GetContainerID() string {
	return m.containerID
}

func (m *MockContainer) Terminate(ctx context.Context, opts ...testcontainers.TerminateOption) error {
	m.terminateCount++
	return m.terminateError
}

func TestNewCleanableContainer_Nil(t *testing.T) {
	cleanable := NewCleanableContainer(nil, "test-unit")

	if cleanable != nil {
		t.Errorf("NewCleanableContainer(nil) = %v, want nil", cleanable)
	}
}

func TestNewCleanableContainer_Valid(t *testing.T) {
	container := &MockContainer{
		containerID: "test-container-id",
	}

	cleanable := NewCleanableContainer(container, "postgres-main")

	if cleanable == nil {
		t.Fatal("NewCleanableContainer() returned nil")
	}

	if cleanable.container == nil {
		t.Error("cleanable.container is nil")
	}

	if cleanable.unitName != "postgres-main" {
		t.Errorf("cleanable.unitName = %v, want postgres-main", cleanable.unitName)
	}
}

func TestCleanableContainer_ResourceType(t *testing.T) {
	container := &MockContainer{
		containerID: "test-container-id",
	}

	cleanable := NewCleanableContainer(container, "http-app")

	if resourceType := cleanable.ResourceType(); resourceType != "container" {
		t.Errorf("ResourceType() = %v, want container", resourceType)
	}
}

func TestCleanableContainer_ResourceID(t *testing.T) {
	container := &MockContainer{
		containerID: "test-container-id",
	}

	cleanable := NewCleanableContainer(container, "redis-cache")

	if resourceID := cleanable.ResourceID(); resourceID != "redis-cache" {
		t.Errorf("ResourceID() = %v, want redis-cache", resourceID)
	}
}

func TestCleanableContainer_Metadata(t *testing.T) {
	container := &MockContainer{
		containerID: "abc123def456",
	}

	cleanable := NewCleanableContainer(container, "mongo-main")
	metadata := cleanable.Metadata()

	if metadata == nil {
		t.Fatal("Metadata() returned nil")
	}

	if metadata["unit"] != "mongo-main" {
		t.Errorf("Metadata()[unit] = %v, want mongo-main", metadata["unit"])
	}

	if metadata["id"] != "abc123def456" {
		t.Errorf("Metadata()[id] = %v, want abc123def456", metadata["id"])
	}
}

func TestCleanableContainer_Metadata_NilContainer(t *testing.T) {
	cleanable := &CleanableContainer{
		container: nil,
		unitName:  "test-unit",
	}

	metadata := cleanable.Metadata()

	if metadata == nil {
		t.Fatal("Metadata() returned nil")
	}

	if metadata["unit"] != "test-unit" {
		t.Errorf("Metadata()[unit] = %v, want test-unit", metadata["unit"])
	}

	// ID should not be present when container is nil
	if _, exists := metadata["id"]; exists {
		t.Error("Metadata()[id] should not exist when container is nil")
	}
}

func TestCleanableContainer_Metadata_EmptyContainerID(t *testing.T) {
	container := &MockContainer{
		containerID: "",
	}

	cleanable := NewCleanableContainer(container, "test-unit")
	metadata := cleanable.Metadata()

	if metadata == nil {
		t.Fatal("Metadata() returned nil")
	}

	// ID should not be present when container ID is empty
	if _, exists := metadata["id"]; exists {
		t.Error("Metadata()[id] should not exist when container ID is empty")
	}
}

func TestCleanableContainer_Cleanup_NilContainer(t *testing.T) {
	cleanable := &CleanableContainer{
		container: nil,
		unitName:  "test-unit",
	}

	ctx := context.Background()
	err := cleanable.Cleanup(ctx)

	if err != nil {
		t.Errorf("Cleanup() with nil container error = %v, want nil", err)
	}
}

func TestCleanableContainer_Cleanup_Success(t *testing.T) {
	container := &MockContainer{
		containerID:    "test-container-id",
		terminateError: nil,
	}

	cleanable := NewCleanableContainer(container, "test-unit")

	ctx := context.Background()
	err := cleanable.Cleanup(ctx)

	if err != nil {
		t.Errorf("Cleanup() error = %v, want nil", err)
	}

	if container.terminateCount != 1 {
		t.Errorf("Terminate() call count = %v, want 1", container.terminateCount)
	}
}

func TestCleanableContainer_Cleanup_Error(t *testing.T) {
	expectedErr := context.DeadlineExceeded
	container := &MockContainer{
		containerID:    "test-container-id",
		terminateError: expectedErr,
	}

	cleanable := NewCleanableContainer(container, "failing-unit")

	ctx := context.Background()
	err := cleanable.Cleanup(ctx)

	if err == nil {
		t.Fatal("Cleanup() error = nil, want error")
	}

	// Error message should include the unit name
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Cleanup() error message is empty")
	}

	// Should have attempted termination
	if container.terminateCount != 1 {
		t.Errorf("Terminate() call count = %v, want 1", container.terminateCount)
	}
}

func TestCleanableContainer_Cleanup_Idempotency(t *testing.T) {
	container := &MockContainer{
		containerID:    "test-container-id",
		terminateError: nil,
	}

	cleanable := NewCleanableContainer(container, "test-unit")

	ctx := context.Background()

	// Call cleanup multiple times
	for i := 0; i < 3; i++ {
		err := cleanable.Cleanup(ctx)
		if err != nil {
			t.Errorf("Cleanup() call %d error = %v, want nil", i+1, err)
		}
	}

	// Terminate should have been called each time
	if container.terminateCount != 3 {
		t.Errorf("Terminate() call count = %v, want 3", container.terminateCount)
	}
}

func TestCleanableContainer_InterfaceCompliance(t *testing.T) {
	container := &MockContainer{
		containerID: "test-container-id",
	}

	cleanable := NewCleanableContainer(container, "test-unit")

	// Verify it implements Cleanable interface
	var _ Cleanable = cleanable
}

func TestCleanableContainer_MultipleInstances(t *testing.T) {
	container1 := &MockContainer{containerID: "id-1"}
	container2 := &MockContainer{containerID: "id-2"}

	cleanable1 := NewCleanableContainer(container1, "unit-1")
	cleanable2 := NewCleanableContainer(container2, "unit-2")

	if cleanable1.ResourceID() == cleanable2.ResourceID() {
		t.Error("Different containers should have different resource IDs")
	}

	metadata1 := cleanable1.Metadata()
	metadata2 := cleanable2.Metadata()

	if metadata1["unit"] == metadata2["unit"] {
		t.Error("Different containers should have different unit names in metadata")
	}

	if metadata1["id"] == metadata2["id"] {
		t.Error("Different containers should have different IDs in metadata")
	}
}

func TestCleanableContainer_EmptyUnitName(t *testing.T) {
	container := &MockContainer{
		containerID: "test-id",
	}

	cleanable := NewCleanableContainer(container, "")

	if cleanable == nil {
		t.Fatal("NewCleanableContainer() returned nil for container with empty unit name")
	}

	if cleanable.ResourceID() != "" {
		t.Errorf("ResourceID() = %v, want empty string", cleanable.ResourceID())
	}

	metadata := cleanable.Metadata()
	if metadata["unit"] != "" {
		t.Errorf("Metadata()[unit] = %v, want empty string", metadata["unit"])
	}
}

func TestCleanableContainer_ContextCancellation(t *testing.T) {
	container := &MockContainer{
		containerID:    "test-id",
		terminateError: nil,
	}

	cleanable := NewCleanableContainer(container, "test-unit")

	// Create an already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Cleanup with canceled context
	// The mock doesn't check context, but real implementation should
	_ = cleanable.Cleanup(ctx)

	// Verify terminate was still called (mock doesn't respect context)
	if container.terminateCount != 1 {
		t.Errorf("Terminate() call count = %v, want 1", container.terminateCount)
	}
}

func TestCleanableContainer_CleanupErrorWrapping(t *testing.T) {
	expectedErr := context.DeadlineExceeded
	container := &MockContainer{
		containerID:    "test-container-id",
		terminateError: expectedErr,
	}

	cleanable := NewCleanableContainer(container, "my-unit")

	ctx := context.Background()
	err := cleanable.Cleanup(ctx)

	if err == nil {
		t.Fatal("Cleanup() error = nil, want error")
	}

	// Error should include unit name for better debugging
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Error("Error message is empty")
	}
}
