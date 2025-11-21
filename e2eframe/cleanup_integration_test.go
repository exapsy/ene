package e2eframe

import (
	"context"
	"testing"
	"time"

	"errors"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
)

func TestCleanupRegistry_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	registry := NewCleanupRegistry()

	// Create a network
	net, err := tcnetwork.New(ctx,
		tcnetwork.WithCheckDuplicate(),
		tcnetwork.WithAttachable(),
	)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}

	networkID := net.ID
	t.Logf("Created network: %s (ID: %s)", net.Name, networkID)

	// Register the network for cleanup
	registry.Register(NewCleanableNetwork(net))

	// Create a container
	req := testcontainers.ContainerRequest{
		Image:    "alpine:latest",
		Cmd:      []string{"sleep", "300"},
		Networks: []string{net.Name},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	containerID := container.GetContainerID()
	t.Logf("Created container: %s", containerID)

	// Register the container for cleanup
	registry.Register(NewCleanableContainer(container, "test-alpine"))

	// Verify both resources are registered
	if count := registry.Count(); count != 2 {
		t.Errorf("Registry count = %d, want 2", count)
	}

	if containerCount := registry.CountByType("container"); containerCount != 1 {
		t.Errorf("Container count = %d, want 1", containerCount)
	}

	if networkCount := registry.CountByType("network"); networkCount != 1 {
		t.Errorf("Network count = %d, want 1", networkCount)
	}

	// Cleanup all resources
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = registry.CleanupAll(cleanupCtx)
	if err != nil {
		t.Errorf("CleanupAll() error = %v, want nil", err)
	}

	// Verify registry is now empty
	if count := registry.Count(); count != 0 {
		t.Errorf("Registry count after cleanup = %d, want 0", count)
	}

	// Verify container is removed
	t.Logf("Verifying container %s is removed...", containerID)
	if err := verifyContainerRemoved(ctx, containerID); err != nil {
		t.Errorf("Container verification failed: %v", err)
	}

	// Verify network is removed
	t.Logf("Verifying network %s is removed...", networkID)
	if err := verifyNetworkRemoved(ctx, networkID); err != nil {
		t.Errorf("Network verification failed: %v", err)
	}
}

func TestCleanupRegistry_Integration_OrderMatters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	registry := NewCleanupRegistry()

	// Create a network
	net, err := tcnetwork.New(ctx,
		tcnetwork.WithCheckDuplicate(),
		tcnetwork.WithAttachable(),
	)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}

	t.Logf("Created network: %s (ID: %s)", net.Name, net.ID)

	// Create a container attached to the network
	req := testcontainers.ContainerRequest{
		Image:    "alpine:latest",
		Cmd:      []string{"sleep", "300"},
		Networks: []string{net.Name},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	t.Logf("Created container: %s", container.GetContainerID())

	// Register in reverse order (network first, container second)
	// The registry should still clean up containers before networks
	registry.Register(NewCleanableNetwork(net))
	registry.Register(NewCleanableContainer(container, "test-alpine"))

	// Cleanup all - should respect order (container before network)
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = registry.CleanupAll(cleanupCtx)
	if err != nil {
		t.Errorf("CleanupAll() error = %v, want nil", err)
	}

	// Both should be cleaned up successfully
	if count := registry.Count(); count != 0 {
		t.Errorf("Registry count after cleanup = %d, want 0", count)
	}
}

func TestCleanupRegistry_Integration_PartialCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	registry := NewCleanupRegistry()

	// Create two containers
	req := testcontainers.ContainerRequest{
		Image: "alpine:latest",
		Cmd:   []string{"sleep", "300"},
	}

	container1, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create container1: %v", err)
	}

	container2, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create container2: %v", err)
	}

	// Create a network
	net, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("Failed to create network: %v", err)
	}

	// Register all resources
	registry.Register(NewCleanableContainer(container1, "alpine-1"))
	registry.Register(NewCleanableContainer(container2, "alpine-2"))
	registry.Register(NewCleanableNetwork(net))

	if count := registry.Count(); count != 3 {
		t.Fatalf("Registry count = %d, want 3", count)
	}

	// Cleanup only containers
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = registry.CleanupByType(cleanupCtx, "container")
	if err != nil {
		t.Errorf("CleanupByType(container) error = %v, want nil", err)
	}

	// Network should still be in registry
	if count := registry.Count(); count != 1 {
		t.Errorf("Registry count after partial cleanup = %d, want 1", count)
	}

	if networkCount := registry.CountByType("network"); networkCount != 1 {
		t.Errorf("Network count = %d, want 1", networkCount)
	}

	// Cleanup the remaining network
	err = registry.CleanupAll(cleanupCtx)
	if err != nil {
		t.Errorf("CleanupAll() error = %v, want nil", err)
	}

	if count := registry.Count(); count != 0 {
		t.Errorf("Registry count after full cleanup = %d, want 0", count)
	}
}

// verifyContainerRemoved checks that a container with the given ID no longer exists
func verifyContainerRemoved(ctx context.Context, containerID string) error {
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer dockerCli.Close()

	// List containers with the specific ID filter
	containers, err := dockerCli.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("id", containerID),
		),
	})
	if err != nil {
		return err
	}

	if len(containers) > 0 {
		return errors.New("container still exists: " + containerID)
	}

	return nil
}

// verifyNetworkRemoved checks that a network with the given ID no longer exists
func verifyNetworkRemoved(ctx context.Context, networkID string) error {
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer dockerCli.Close()

	// Try to inspect the network
	_, err = dockerCli.NetworkInspect(ctx, networkID, network.InspectOptions{
		Verbose: false,
	})
	if err != nil {
		// Network not found is expected
		if client.IsErrNotFound(err) {
			return nil
		}
		return err
	}

	// If we got here, the network still exists
	return errors.New("network still exists: " + networkID)
}
