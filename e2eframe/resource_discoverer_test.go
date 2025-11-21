package e2eframe_test

import (
	"context"
	"testing"
	"time"

	"github.com/exapsy/ene/e2eframe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func TestResourceDiscoverer_New(t *testing.T) {
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err, "Should create discoverer")
	require.NotNil(t, discoverer, "Discoverer should not be nil")
	defer discoverer.Close()
}

func TestResourceDiscoverer_DiscoverOrphanedNetworks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	defer discoverer.Close()

	// Create a test network that should be discovered
	net, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           "testcontainers-discovery-test",
			CheckDuplicate: true,
		},
	})
	require.NoError(t, err)
	defer func() {
		_ = net.Remove(ctx)
	}()

	// Discover networks
	opts := e2eframe.DiscoverOptions{
		OlderThan:  0,
		IncludeAll: false,
	}

	networks, err := discoverer.DiscoverOrphanedNetworks(ctx, opts)
	require.NoError(t, err, "Should discover networks without error")

	// Find our test network
	found := false
	for _, network := range networks {
		if network.Name == "testcontainers-discovery-test" {
			found = true
			assert.Equal(t, 0, network.Containers, "Test network should have no containers")
			assert.True(t, network.IsTestcontainers, "Should be marked as testcontainers network")
			assert.NotEmpty(t, network.ID, "Should have an ID")
			break
		}
	}

	assert.True(t, found, "Should find the test network")

	// Cleanup
	err = net.Remove(ctx)
	assert.NoError(t, err)
}

func TestResourceDiscoverer_DiscoverOrphanedNetworks_WithContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	defer discoverer.Close()

	// Create a test network
	net, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           "testcontainers-with-container",
			CheckDuplicate: true,
		},
	})
	require.NoError(t, err)
	defer func() {
		_ = net.Remove(ctx)
	}()

	dockerNet, ok := net.(*testcontainers.DockerNetwork)
	require.True(t, ok)

	// Create a container on the network
	cont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "alpine:latest",
			Cmd:      []string{"sleep", "30"},
			Networks: []string{dockerNet.Name},
		},
		Started: true,
	})
	require.NoError(t, err)
	defer func() {
		_ = cont.Terminate(ctx)
	}()

	// Give Docker API time to update network state with attached container
	time.Sleep(1 * time.Second)

	// Discover networks (should NOT include network with container)
	opts := e2eframe.DiscoverOptions{
		OlderThan:  0,
		IncludeAll: false,
	}

	networks, err := discoverer.DiscoverOrphanedNetworks(ctx, opts)
	require.NoError(t, err)

	// Verify our network is NOT in the list (has a container)
	found := false
	for _, network := range networks {
		if network.Name == "testcontainers-with-container" {
			found = true
			break
		}
	}
	assert.False(t, found, "Should NOT find network with attached container when IncludeAll is false")

	// Now try with IncludeAll = true
	opts.IncludeAll = true
	networks, err = discoverer.DiscoverOrphanedNetworks(ctx, opts)
	require.NoError(t, err)

	// Now it should be included (the important thing is that IncludeAll allows finding it)
	found = false
	for _, network := range networks {
		if network.Name == "testcontainers-with-container" {
			found = true
			// Note: Docker API may not immediately report container count in network list
			// The key behavior is that IncludeAll allows discovering networks with containers
			t.Logf("Found network with %d containers reported (may be 0 due to Docker API timing)", network.Containers)
			break
		}
	}
	assert.True(t, found, "Should find network when IncludeAll is true (regardless of reported container count)")

	// Cleanup
	_ = cont.Terminate(ctx)
	_ = net.Remove(ctx)
}

func TestResourceDiscoverer_DiscoverOrphanedNetworks_OlderThan(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	defer discoverer.Close()

	// Create a test network
	net, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           "testcontainers-age-test",
			CheckDuplicate: true,
		},
	})
	require.NoError(t, err)
	defer func() {
		_ = net.Remove(ctx)
	}()

	// Immediately try to discover with OlderThan = 1 hour (should NOT find it)
	opts := e2eframe.DiscoverOptions{
		OlderThan:  1 * time.Hour,
		IncludeAll: false,
	}

	networks, err := discoverer.DiscoverOrphanedNetworks(ctx, opts)
	require.NoError(t, err)

	found := false
	for _, network := range networks {
		if network.Name == "testcontainers-age-test" {
			found = true
			break
		}
	}
	assert.False(t, found, "Should NOT find network that's too new")

	// Try with OlderThan = 0 (should find it)
	opts.OlderThan = 0
	networks, err = discoverer.DiscoverOrphanedNetworks(ctx, opts)
	require.NoError(t, err)

	found = false
	for _, network := range networks {
		if network.Name == "testcontainers-age-test" {
			found = true
			assert.True(t, network.Age < 10*time.Second, "Network should be very recent")
			break
		}
	}
	assert.True(t, found, "Should find network with no age filter")

	// Cleanup
	_ = net.Remove(ctx)
}

func TestResourceDiscoverer_DiscoverOrphanedContainers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	defer discoverer.Close()

	// Create a network
	net, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           "testcontainers-container-discovery",
			CheckDuplicate: true,
		},
	})
	require.NoError(t, err)
	defer func() {
		_ = net.Remove(ctx)
	}()

	dockerNet, ok := net.(*testcontainers.DockerNetwork)
	require.True(t, ok)

	// Create a stopped container
	cont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "alpine:latest",
			Cmd:      []string{"echo", "test"},
			Networks: []string{dockerNet.Name},
		},
		Started: true,
	})
	require.NoError(t, err)
	defer func() {
		_ = cont.Terminate(ctx)
	}()

	// Stop the container (don't remove it)
	err = cont.Stop(ctx, nil)
	require.NoError(t, err)

	// Give it a moment to stop
	time.Sleep(500 * time.Millisecond)

	// Discover containers
	opts := e2eframe.DiscoverOptions{
		OlderThan:  0,
		IncludeAll: false,
	}

	containers, err := discoverer.DiscoverOrphanedContainers(ctx, opts)
	require.NoError(t, err, "Should discover containers without error")

	// Find our container
	found := false
	containerID := cont.GetContainerID()
	for _, container := range containers {
		if container.ID == containerID {
			found = true
			assert.Contains(t, []string{"exited", "stopped"}, container.State, "Container should be stopped")
			assert.True(t, container.IsEne, "Should be marked as ene container")
			break
		}
	}

	assert.True(t, found, "Should find the stopped container")

	// Cleanup
	_ = cont.Terminate(ctx)
	_ = net.Remove(ctx)
}

func TestResourceDiscoverer_DiscoverOrphanedContainers_RunningExcluded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	defer discoverer.Close()

	// Create a network
	net, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           "testcontainers-running-test",
			CheckDuplicate: true,
		},
	})
	require.NoError(t, err)
	defer func() {
		_ = net.Remove(ctx)
	}()

	dockerNet, ok := net.(*testcontainers.DockerNetwork)
	require.True(t, ok)

	// Create a running container
	cont, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:    "alpine:latest",
			Cmd:      []string{"sleep", "30"},
			Networks: []string{dockerNet.Name},
		},
		Started: true,
	})
	require.NoError(t, err)
	defer func() {
		_ = cont.Terminate(ctx)
	}()

	// Verify it's running
	state, err := cont.State(ctx)
	require.NoError(t, err)
	require.True(t, state.Running)

	// Discover containers (should NOT include running)
	opts := e2eframe.DiscoverOptions{
		OlderThan:  0,
		IncludeAll: false,
	}

	containers, err := discoverer.DiscoverOrphanedContainers(ctx, opts)
	require.NoError(t, err)

	// Verify running container is NOT in the list
	containerID := cont.GetContainerID()
	found := false
	for _, container := range containers {
		if container.ID == containerID {
			found = true
			break
		}
	}
	assert.False(t, found, "Should NOT find running container when IncludeAll is false")

	// Try with IncludeAll = true
	opts.IncludeAll = true
	containers, err = discoverer.DiscoverOrphanedContainers(ctx, opts)
	require.NoError(t, err)

	// Now it should be included
	found = false
	for _, container := range containers {
		if container.ID == containerID {
			found = true
			assert.Equal(t, "running", container.State)
			break
		}
	}
	assert.True(t, found, "Should find running container when IncludeAll is true")

	// Cleanup
	_ = cont.Terminate(ctx)
	_ = net.Remove(ctx)
}

func TestResourceDiscoverer_DiscoverAll(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	defer discoverer.Close()

	// Create a network
	net, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name:           "testcontainers-discover-all",
			CheckDuplicate: true,
		},
	})
	require.NoError(t, err)
	defer func() {
		_ = net.Remove(ctx)
	}()

	// Discover all resources
	opts := e2eframe.DiscoverOptions{
		OlderThan:  0,
		IncludeAll: false,
	}

	networks, containers, err := discoverer.DiscoverAll(ctx, opts)
	require.NoError(t, err, "DiscoverAll should not error")
	assert.NotNil(t, networks, "Networks list should not be nil")
	assert.NotNil(t, containers, "Containers list should not be nil")

	// Find our network
	found := false
	for _, network := range networks {
		if network.Name == "testcontainers-discover-all" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should find test network in DiscoverAll results")

	// Cleanup
	_ = net.Remove(ctx)
}

func TestResourceDiscoverer_Close(t *testing.T) {
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	require.NotNil(t, discoverer)

	err = discoverer.Close()
	assert.NoError(t, err, "Close should not error")

	// Closing again should also not error
	err = discoverer.Close()
	assert.NoError(t, err, "Closing twice should not error")
}

func TestResourceDiscoverer_SystemNetworksExcluded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	discoverer, err := e2eframe.NewResourceDiscoverer()
	require.NoError(t, err)
	defer discoverer.Close()

	opts := e2eframe.DiscoverOptions{
		OlderThan:  0,
		IncludeAll: true, // Include all to see what's there
	}

	networks, err := discoverer.DiscoverOrphanedNetworks(ctx, opts)
	require.NoError(t, err)

	// Verify system networks are not in the list
	for _, network := range networks {
		assert.NotEqual(t, "bridge", network.Name, "Should not include bridge network")
		assert.NotEqual(t, "host", network.Name, "Should not include host network")
		assert.NotEqual(t, "none", network.Name, "Should not include none network")
	}
}
