package e2eframe

import (
	"context"
	"errors"
	"testing"
)

// MockCleanable is a mock implementation of Cleanable for testing
type MockCleanable struct {
	resourceType string
	resourceID   string
	metadata     map[string]string
	cleanupFunc  func(ctx context.Context) error
	cleanupCount int
}

func (m *MockCleanable) Cleanup(ctx context.Context) error {
	m.cleanupCount++
	if m.cleanupFunc != nil {
		return m.cleanupFunc(ctx)
	}
	return nil
}

func (m *MockCleanable) ResourceType() string {
	return m.resourceType
}

func (m *MockCleanable) ResourceID() string {
	return m.resourceID
}

func (m *MockCleanable) Metadata() map[string]string {
	if m.metadata == nil {
		return make(map[string]string)
	}
	return m.metadata
}

func TestMockCleanable_Implementation(t *testing.T) {
	mock := &MockCleanable{
		resourceType: "test-resource",
		resourceID:   "test-id",
		metadata: map[string]string{
			"key": "value",
		},
	}

	// Test interface compliance
	var _ Cleanable = mock

	// Test methods
	if got := mock.ResourceType(); got != "test-resource" {
		t.Errorf("ResourceType() = %v, want %v", got, "test-resource")
	}

	if got := mock.ResourceID(); got != "test-id" {
		t.Errorf("ResourceID() = %v, want %v", got, "test-id")
	}

	metadata := mock.Metadata()
	if metadata["key"] != "value" {
		t.Errorf("Metadata()[key] = %v, want %v", metadata["key"], "value")
	}

	// Test cleanup
	ctx := context.Background()
	if err := mock.Cleanup(ctx); err != nil {
		t.Errorf("Cleanup() error = %v, want nil", err)
	}

	if mock.cleanupCount != 1 {
		t.Errorf("cleanupCount = %v, want 1", mock.cleanupCount)
	}
}

func TestMockCleanable_CleanupError(t *testing.T) {
	expectedErr := errors.New("cleanup failed")
	mock := &MockCleanable{
		resourceType: "test-resource",
		resourceID:   "test-id",
		cleanupFunc: func(ctx context.Context) error {
			return expectedErr
		},
	}

	ctx := context.Background()
	err := mock.Cleanup(ctx)
	if err == nil {
		t.Fatal("Cleanup() error = nil, want error")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("Cleanup() error = %v, want %v", err, expectedErr)
	}
}

func TestMockCleanable_Idempotency(t *testing.T) {
	mock := &MockCleanable{
		resourceType: "test-resource",
		resourceID:   "test-id",
	}

	ctx := context.Background()

	// Call cleanup multiple times
	for i := 0; i < 3; i++ {
		if err := mock.Cleanup(ctx); err != nil {
			t.Errorf("Cleanup() call %d error = %v, want nil", i+1, err)
		}
	}

	if mock.cleanupCount != 3 {
		t.Errorf("cleanupCount = %v, want 3", mock.cleanupCount)
	}
}

func TestCleanupError_Error(t *testing.T) {
	baseErr := errors.New("original error")
	cleanupErr := &CleanupError{
		ResourceType: "container",
		ResourceID:   "test-container-1",
		Err:          baseErr,
	}

	expected := "failed to cleanup container test-container-1: original error"
	if got := cleanupErr.Error(); got != expected {
		t.Errorf("Error() = %v, want %v", got, expected)
	}
}

func TestCleanupError_Unwrap(t *testing.T) {
	baseErr := errors.New("original error")
	cleanupErr := &CleanupError{
		ResourceType: "network",
		ResourceID:   "test-network-1",
		Err:          baseErr,
	}

	unwrapped := cleanupErr.Unwrap()
	if unwrapped != baseErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, baseErr)
	}

	// Test that errors.Is works
	if !errors.Is(cleanupErr, baseErr) {
		t.Error("errors.Is(cleanupErr, baseErr) = false, want true")
	}
}

func TestCleanupError_NilError(t *testing.T) {
	cleanupErr := &CleanupError{
		ResourceType: "volume",
		ResourceID:   "test-volume-1",
		Err:          nil,
	}

	expected := "failed to cleanup volume test-volume-1: <nil>"
	if got := cleanupErr.Error(); got != expected {
		t.Errorf("Error() = %v, want %v", got, expected)
	}

	if unwrapped := cleanupErr.Unwrap(); unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

func TestCleanupError_WithWrappedError(t *testing.T) {
	rootErr := errors.New("root cause")
	wrappedErr := errors.New("wrapped: " + rootErr.Error())
	cleanupErr := &CleanupError{
		ResourceType: "file",
		ResourceID:   "/tmp/test-file",
		Err:          wrappedErr,
	}

	// Test error message contains all parts
	errMsg := cleanupErr.Error()
	if errMsg == "" {
		t.Error("Error() returned empty string")
	}

	// Test unwrapping
	if unwrapped := cleanupErr.Unwrap(); unwrapped != wrappedErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, wrappedErr)
	}
}

func TestMockCleanable_NilMetadata(t *testing.T) {
	mock := &MockCleanable{
		resourceType: "test-resource",
		resourceID:   "test-id",
		metadata:     nil,
	}

	metadata := mock.Metadata()
	if metadata == nil {
		t.Error("Metadata() returned nil, want empty map")
	}

	if len(metadata) != 0 {
		t.Errorf("Metadata() len = %v, want 0", len(metadata))
	}
}

func TestMockCleanable_ContextCancellation(t *testing.T) {
	mock := &MockCleanable{
		resourceType: "test-resource",
		resourceID:   "test-id",
		cleanupFunc: func(ctx context.Context) error {
			// Simulate respecting context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		},
	}

	// Test with canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mock.Cleanup(ctx)
	if err == nil {
		t.Fatal("Cleanup() with canceled context error = nil, want error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Cleanup() error = %v, want context.Canceled", err)
	}
}
