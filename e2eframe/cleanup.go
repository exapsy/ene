package e2eframe

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// CleanupOrphanedNetworks removes Docker networks that were left behind by ene tests.
// If all is true, it will prune all unused Docker networks (equivalent to docker network prune).
// Otherwise, it only removes networks created by testcontainers (identified by name pattern).
func CleanupOrphanedNetworks(ctx context.Context, all bool) error {
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer dockerCli.Close()

	if all {
		// Prune all unused networks
		fmt.Println("Pruning all unused Docker networks...")
		report, err := dockerCli.NetworksPrune(ctx, filters.Args{})
		if err != nil {
			return fmt.Errorf("prune networks: %w", err)
		}

		if len(report.NetworksDeleted) == 0 {
			fmt.Println("No unused networks found.")
		} else {
			fmt.Printf("Removed %d unused network(s):\n", len(report.NetworksDeleted))
			for _, networkName := range report.NetworksDeleted {
				fmt.Printf("  - %s\n", networkName)
			}
		}
		return nil
	}

	// List all networks
	networks, err := dockerCli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}

	// Filter for testcontainers networks (created by ene)
	// Testcontainers typically creates networks with patterns like:
	// - testcontainers-* (older versions)
	// - *-network (common pattern)
	// We'll look for networks that match testcontainers patterns and have no containers
	var eneNetworks []network.Summary
	for _, net := range networks {
		// Skip system networks
		if net.Name == "bridge" || net.Name == "host" || net.Name == "none" {
			continue
		}

		// Check if it's a testcontainers network
		isTestcontainersNetwork := strings.HasPrefix(net.Name, "testcontainers-") ||
			(strings.Contains(net.Name, "-") && len(net.Name) > 10) // Generic pattern for auto-generated names

		// Only consider networks with no containers attached
		if isTestcontainersNetwork && len(net.Containers) == 0 {
			eneNetworks = append(eneNetworks, net)
		}
	}

	if len(eneNetworks) == 0 {
		fmt.Println("No orphaned ene networks found.")
		return nil
	}

	fmt.Printf("Found %d orphaned network(s):\n", len(eneNetworks))

	removedCount := 0
	failedCount := 0
	for _, net := range eneNetworks {
		fmt.Printf("  Removing %s (ID: %s)...", net.Name, net.ID[:12])

		err := dockerCli.NetworkRemove(ctx, net.ID)
		if err != nil {
			fmt.Printf(" failed: %v\n", err)
			failedCount++
		} else {
			fmt.Printf(" ✓\n")
			removedCount++
		}
	}

	fmt.Printf("\nSummary: Removed %d network(s)", removedCount)
	if failedCount > 0 {
		fmt.Printf(", %d failed", failedCount)
	}
	fmt.Println()

	if failedCount > 0 {
		fmt.Println("\nNote: Some networks could not be removed. They may have containers still attached.")
		fmt.Println("Try running: docker network inspect <network-name> to see what's connected.")
	}

	return nil
}
