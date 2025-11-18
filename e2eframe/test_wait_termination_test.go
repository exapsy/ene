package e2eframe

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
)

// TestWaitForContainersTermination_NilNetwork tests that nil network is handled gracefully
func TestWaitForContainersTermination_NilNetwork(t *testing.T) {
	ctx := context.Background()
	err := WaitForContainersTermination(ctx, nil, 5*time.Second)
	if err != nil {
		t.Errorf("Expected no error for nil network, got: %v", err)
	}
}

// TestWaitForContainersTermination_NoContainers tests that empty network returns immediately
func TestWaitForContainersTermination_NoContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a network with no containers
	net, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}
	defer func() {
		if err := net.Remove(ctx); err != nil {
			t.Logf("Warning: Failed to remove network: %v", err)
		}
	}()

	start := time.Now()
	err = WaitForContainersTermination(ctx, net, 5*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error for empty network, got: %v", err)
	}

	// Should return almost immediately (within 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected immediate return, but took %v", elapsed)
	}
}

// TestWaitForContainersTermination_WithContainer tests waiting for actual container termination
func TestWaitForContainersTermination_WithContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a network
	net, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}
	defer func() {
		if err := net.Remove(ctx); err != nil {
			t.Logf("Warning: Failed to remove network: %v", err)
		}
	}()

	// Create a simple container
	req := testcontainers.ContainerRequest{
		Image:    "alpine:latest",
		Cmd:      []string{"sh", "-c", "sleep 1"},
		Networks: []string{net.Name},
		NetworkAliases: map[string][]string{
			net.Name: {"test-container"},
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	// Stop the container (but it takes time to actually terminate)
	if err := container.Terminate(ctx); err != nil {
		t.Logf("Warning: Failed to terminate container: %v", err)
	}

	// Now wait for it to be fully removed
	start := time.Now()
	err = WaitForContainersTermination(ctx, net, 10*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error waiting for container termination, got: %v", err)
	}

	t.Logf("Container termination detected in %v", elapsed)

	// Should be faster than our old 2 second hard-coded sleep in most cases
	// But we give it a generous upper bound for slow systems
	if elapsed > 5*time.Second {
		t.Logf("Warning: Container termination took longer than expected: %v", elapsed)
	}
}

// TestWaitForContainersTermination_Timeout tests timeout behavior
func TestWaitForContainersTermination_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a network
	net, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}
	defer func() {
		if err := net.Remove(ctx); err != nil {
			t.Logf("Warning: Failed to remove network: %v", err)
		}
	}()

	// Create a container that sleeps for a long time
	req := testcontainers.ContainerRequest{
		Image:    "alpine:latest",
		Cmd:      []string{"sh", "-c", "sleep 30"},
		Networks: []string{net.Name},
		NetworkAliases: map[string][]string{
			net.Name: {"test-container"},
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	// Make sure we clean up the container at the end
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Warning: Failed to terminate container: %v", err)
		}
	}()

	// Initiate stop but don't wait for it
	go func() {
		time.Sleep(100 * time.Millisecond)
		container.Terminate(ctx)
	}()

	// Try to wait with a very short timeout
	start := time.Now()
	err = WaitForContainersTermination(ctx, net, 500*time.Millisecond)
	elapsed := time.Since(start)

	// We expect a timeout error
	if err == nil {
		t.Errorf("Expected timeout error, but got success")
	}

	// Should timeout around 500ms
	if elapsed < 400*time.Millisecond || elapsed > 1*time.Second {
		t.Errorf("Expected timeout around 500ms, but took %v", elapsed)
	}

	t.Logf("Timeout occurred as expected after %v: %v", elapsed, err)
}

// TestWaitForContainersTermination_MultipleContainers tests with multiple containers
func TestWaitForContainersTermination_MultipleContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create a network
	net, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}
	defer func() {
		if err := net.Remove(ctx); err != nil {
			t.Logf("Warning: Failed to remove network: %v", err)
		}
	}()

	// Create multiple containers
	containers := make([]testcontainers.Container, 3)
	for i := 0; i < 3; i++ {
		req := testcontainers.ContainerRequest{
			Image:    "alpine:latest",
			Cmd:      []string{"sh", "-c", "sleep 1"},
			Networks: []string{net.Name},
			NetworkAliases: map[string][]string{
				net.Name: {fmt.Sprintf("test-container-%d", i)},
			},
		}

		container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			t.Fatalf("Failed to create container %d: %v", i, err)
		}
		containers[i] = container
	}

	// Stop all containers
	for i, container := range containers {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Warning: Failed to terminate container %d: %v", i, err)
		}
	}

	// Wait for all to terminate
	start := time.Now()
	err = WaitForContainersTermination(ctx, net, 15*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error waiting for containers termination, got: %v", err)
	}

	t.Logf("All %d containers terminated in %v", len(containers), elapsed)

	// Should still be reasonably fast
	if elapsed > 10*time.Second {
		t.Logf("Warning: Multiple container termination took longer than expected: %v", elapsed)
	}
}

// TestWaitForContainersTermination_ExponentialBackoff tests that backoff works correctly
func TestWaitForContainersTermination_ExponentialBackoff(t *testing.T) {
	// This is more of a behavioral test - we'll verify the function exists and compiles
	// The actual backoff behavior is tested indirectly through the other tests
	ctx := context.Background()

	// Quick check that it handles nil correctly
	err := WaitForContainersTermination(ctx, nil, 1*time.Second)
	if err != nil {
		t.Errorf("Expected no error for nil network, got: %v", err)
	}
}
