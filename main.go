/*
 * This program runs the e2e tests for the application.
 */
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/exapsy/ene/e2eframe"
	_ "github.com/exapsy/ene/plugins/httpmockunit"
	_ "github.com/exapsy/ene/plugins/httptest"
	_ "github.com/exapsy/ene/plugins/httpunit"
	_ "github.com/exapsy/ene/plugins/miniotest"
	_ "github.com/exapsy/ene/plugins/miniounit"
	_ "github.com/exapsy/ene/plugins/mongounit"
	_ "github.com/exapsy/ene/plugins/postgresunit"
	"github.com/spf13/cobra"
)

var (
	// Version information - set at build time via ldflags
	version = "dev"
	commit  = "unknown"
	date    = "unknown"

	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorBold   = "\033[1m"
)

var rootCmd = &cobra.Command{
	Use:     "ene",
	Short:   "Run e2e tests or scaffold a new suite",
	Long:    `When called with no sub-command, runs all e2e tests. Use "ene scaffold-test" to create a new suite.`,
	Version: version,
	Run: func(cmd *cobra.Command, args []string) {
		verbose := cmd.Flag("verbose").Value.String()
		pretty := cmd.Flag("pretty").Value.String()
		parallel := cmd.Flag("parallel").Value.String()
		suiteFlag := cmd.Flag("suite").Value.String()
		debug := cmd.Flag("debug").Value.String()
		cleanupCache := cmd.Flag("cleanup-cache").Value.String()
		suitesFilter := strings.Split(suiteFlag, ",")
		htmlReportPath := cmd.Flag("html").Value.String()
		jsonReportPath := cmd.Flag("json").Value.String()
		baseDir := cmd.Flag("base-dir").Value.String()

		isVerbose := verbose == "true"
		isPretty := pretty == "true"
		isParallel := parallel == "true"
		isCleanupCache := cleanupCache == "true"
		isDebug := debug == "true"

		// Function to check if a test should be included based on filter
		shouldIncludeTest := func(suiteName, testName string) bool {
			if len(suitesFilter) == 0 {
				return true // No filter, include all tests
			}

			for _, filter := range suitesFilter {
				if strings.Contains(suiteName, filter) || strings.Contains(testName, filter) {
					return true
				}
			}

			return false
		}

		// Count total suites that will be run (for progress tracking)
		totalSuites, err := e2eframe.CountFilteredTestSuites(baseDir, shouldIncludeTest)
		if err != nil {
			fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)
			os.Exit(1)
		}

		// Collect test results
		eventChan := e2eframe.NewEventChannel()
		var eventSink e2eframe.EventSink = eventChan

		// Use optimized defaults for better performance
		maxRetries := 3       // Keep reliable retry behavior
		isCleanupCache = true // Always cleanup for better performance

		// Set up signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		// Handle signals in a goroutine
		go func() {
			sig := <-sigChan
			fmt.Printf("\n%s%s⚠ Received signal %v, initiating graceful shutdown...%s\n",
				colorBold, colorYellow, sig, colorReset)
			fmt.Printf("%s%sPlease wait for cleanup to complete. Press Ctrl+C again to force quit.%s\n",
				colorBold, colorYellow, colorReset)
			cancel()

			// Second signal forces immediate exit
			<-sigChan
			fmt.Printf("\n%s%s✖ Force quit - Docker resources may be left behind%s\n",
				colorBold, colorRed, colorReset)
			fmt.Printf("%s%sRun 'docker network prune -f' to clean up orphaned networks%s\n",
				colorYellow, colorBold, colorReset)
			os.Exit(130) // 128 + SIGINT
		}()

		err = e2eframe.Run(ctx, &e2eframe.RunOpts{
			FilterFunc:   shouldIncludeTest,
			Verbose:      isVerbose,
			Parallel:     isParallel,
			Events:       eventSink,
			MaxRetries:   maxRetries,
			RetryDelay:   "2s",
			Debug:        isDebug,
			BaseDir:      baseDir,
			CleanupCache: isCleanupCache,
		})
		if err != nil {
			fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)
			os.Exit(1)
		}

		testsSecretary := e2eframe.NewTestsSecretary(eventChan) // keep track of tests

		consumers := []e2eframe.OutputProcessor{
			e2eframe.NewStdoutHumanOutputProcessor(e2eframe.StdoutHumanOutputProcessorParams{
				Verbose:        isVerbose,
				Pretty:         isPretty,
				Debug:          isDebug,
				TestsSecretary: testsSecretary,
				Output:         os.Stdout,
				TotalSuites:    totalSuites,
			}),
		}

		if htmlReportPath != "" {
			htmlConsumer, err := e2eframe.NewHTMLReportProcessor(e2eframe.HTMLReportProcessorParams{
				OutputFile:     htmlReportPath,
				Template:       e2eframe.GetDefaultHTMLTemplate(),
				TestsSecretary: testsSecretary,
			})
			if err != nil {
				fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)

				return
			}
			consumers = append(consumers, htmlConsumer)
		}

		if jsonReportPath != "" {
			jsonConsumer, err := e2eframe.NewJSONReportProcessor(e2eframe.JSONReportProcessorParams{
				OutputFile:     jsonReportPath,
				TestsSecretary: testsSecretary,
			})
			if err != nil {
				fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)

				return
			}
			consumers = append(consumers, jsonConsumer)
		}

		// Process events
		for event := range eventChan {
			err := testsSecretary.ConsumeEvent(event)
			if err != nil {
				fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)

				return
			}

			for _, consumer := range consumers {
				err = consumer.ConsumeEvent(event)
				if err != nil {
					fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)

					return
				}
			}
		}

		// Flush all consumers
		for _, consumer := range consumers {
			err = consumer.Flush()
			if err != nil {
				fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)

				return
			}
		}

		// Exit with appropriate status code
		if testsSecretary.TotalFailedTests() > 0 {
			os.Exit(1)
		}
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ene version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built: %s\n", date)
	},
}

var scaffoldTestCmd = &cobra.Command{
	Use:   "scaffold-test [name]",
	Args:  cobra.ExactArgs(1),
	Short: "Scaffold a new test",
	Long:  `Create a new test suite under ./tests/<n>`,
	Run: func(cmd *cobra.Command, args []string) {
		tmpl := cmd.Flag("tmpl").Value.String()

		var templates []string
		if tmpl != "" {
			templates = strings.Split(tmpl, ",")
			for i, t := range templates {
				templates[i] = strings.TrimSpace(t)
			}
		} else {
			templates = []string{"mongo", "httpmock"} // Default templates if none specified
		}

		testName := args[0]
		if err := ScaffoldTest(testName, templates); err != nil {
			fmt.Println("Error scaffolding test:", err)

			return
		}
		fmt.Printf("Test %s scaffolded successfully\n", testName)
	},
}

var dryRunCmd = &cobra.Command{
	Use:   "dry-run [test-file]",
	Args:  cobra.MaximumNArgs(1),
	Short: "Validate test configuration without running containers",
	Long:  `Parse and validate test configuration files to check for syntax errors and plugin compatibility`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose := cmd.Flag("verbose").Value.String()
		debug := cmd.Flag("debug").Value.String()
		baseDir := cmd.Flag("base-dir").Value.String()

		isVerbose := verbose == "true"
		isDebug := debug == "true"

		var testFile string
		if len(args) > 0 {
			testFile = args[0]
		}

		err := e2eframe.DryRun(context.Background(), &e2eframe.DryRunOpts{
			TestFile: testFile,
			Verbose:  isVerbose,
			Debug:    isDebug,
			BaseDir:  baseDir,
		})
		if err != nil {
			fmt.Printf("%s%s✖ DRY RUN FAILED: %v%s\n", colorBold, colorRed, err, colorReset)
			os.Exit(1)
		}

		fmt.Printf("%s%s✓ DRY RUN PASSED: Configuration is valid%s\n", colorBold, colorGreen, colorReset)
	},
}

var listSuitesCmd = &cobra.Command{
	Use:   "list-suites",
	Short: "List all available test suites",
	Long:  `List all test suites found in the tests directory`,
	Run: func(cmd *cobra.Command, args []string) {
		baseDir := cmd.Flag("base-dir").Value.String()

		suiteNames, err := e2eframe.ListTestSuiteNames(baseDir)
		if err != nil {
			fmt.Printf("%s%s✖ ERROR: %v%s\n", colorBold, colorRed, err, colorReset)
			os.Exit(1)
		}

		if len(suiteNames) == 0 {
			fmt.Printf("%s%sNo test suites found%s\n", colorBold, colorYellow, colorReset)
			return
		}

		fmt.Printf("%s%sAvailable test suites:%s\n", colorBold, colorGreen, colorReset)
		for _, name := range suiteNames {
			fmt.Printf("  %s\n", name)
		}
	},
}

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [resource-type]",
	Short: "Clean up orphaned Docker resources created by ene",
	Long: `Discovers and removes orphaned Docker resources created by ene tests.

Supported resource types:
  networks    - Clean up Docker networks
  containers  - Clean up Docker containers
  all         - Clean up all resource types (default)

Examples:
  ene cleanup                      # Interactive cleanup (shows what will be removed)
  ene cleanup --dry-run            # Show what would be removed without removing
  ene cleanup --force              # Skip confirmation prompt
  ene cleanup networks             # Clean only networks
  ene cleanup containers           # Clean only containers
  ene cleanup --older-than=1h      # Clean resources older than 1 hour
  ene cleanup --all                # Clean all resource types

The cleanup command uses the CleanupRegistry to ensure proper ordering:
containers are removed before networks to prevent "network has active endpoints" errors.`,
	Run: func(cmd *cobra.Command, args []string) {
		dryRun := cmd.Flag("dry-run").Value.String() == "true"
		force := cmd.Flag("force").Value.String() == "true"
		all := cmd.Flag("all").Value.String() == "true"
		olderThanStr := cmd.Flag("older-than").Value.String()
		verbose := cmd.Flag("verbose").Value.String() == "true"

		// Parse older-than duration
		var olderThan time.Duration
		if olderThanStr != "" {
			var err error
			olderThan, err = time.ParseDuration(olderThanStr)
			if err != nil {
				fmt.Printf("%s%s✖ ERROR: Invalid duration format: %v%s\n", colorBold, colorRed, err, colorReset)
				fmt.Printf("Examples: 1h, 30m, 1h30m\n")
				os.Exit(1)
			}
		}

		// Determine resource type from args
		resourceType := "all"
		if len(args) > 0 {
			resourceType = args[0]
			// Validate resource type
			if resourceType != "all" && resourceType != "networks" && resourceType != "containers" {
				fmt.Printf("%s%s✖ ERROR: Invalid resource type: %s%s\n", colorBold, colorRed, resourceType, colorReset)
				fmt.Printf("Valid types: all, networks, containers\n")
				os.Exit(1)
			}
		}

		ctx := context.Background()

		// Create discoverer
		discoverer, err := e2eframe.NewResourceDiscoverer()
		if err != nil {
			fmt.Printf("%s%s✖ ERROR: Failed to connect to Docker: %v%s\n", colorBold, colorRed, err, colorReset)
			fmt.Printf("Is Docker running?\n")
			os.Exit(1)
		}
		defer discoverer.Close()

		// Discover resources
		opts := e2eframe.DiscoverOptions{
			OlderThan:  olderThan,
			IncludeAll: all,
		}

		var networks []e2eframe.OrphanedNetwork
		var containers []e2eframe.OrphanedContainer

		if resourceType == "all" || resourceType == "networks" {
			networks, err = discoverer.DiscoverOrphanedNetworks(ctx, opts)
			if err != nil {
				fmt.Printf("%s%s✖ ERROR: Failed to discover networks: %v%s\n", colorBold, colorRed, err, colorReset)
				os.Exit(1)
			}
		}

		if resourceType == "all" || resourceType == "containers" {
			containers, err = discoverer.DiscoverOrphanedContainers(ctx, opts)
			if err != nil {
				fmt.Printf("%s%s✖ ERROR: Failed to discover containers: %v%s\n", colorBold, colorRed, err, colorReset)
				os.Exit(1)
			}
		}

		// Display what was found
		if len(networks) == 0 && len(containers) == 0 {
			fmt.Printf("%s%s✓ No orphaned resources found.%s\n", colorBold, colorGreen, colorReset)
			return
		}

		fmt.Printf("%s%sFound orphaned resources:%s\n", colorBold, colorCyan, colorReset)
		if len(containers) > 0 {
			fmt.Printf("  %sContainers:%s %d\n", colorYellow, colorReset, len(containers))
			if verbose {
				for _, cont := range containers {
					fmt.Printf("    - %s (state: %s, age: %v)\n", cont.Name, cont.State, cont.Age.Round(time.Second))
				}
			}
		}
		if len(networks) > 0 {
			fmt.Printf("  %sNetworks:%s %d\n", colorYellow, colorReset, len(networks))
			if verbose {
				for _, net := range networks {
					fmt.Printf("    - %s (containers: %d, age: %v)\n", net.Name, net.Containers, net.Age.Round(time.Second))
				}
			}
		}
		fmt.Println()

		// In dry-run mode, just list and exit
		if dryRun {
			fmt.Printf("%s%sDry-run mode - no resources will be removed%s\n", colorBold, colorYellow, colorReset)
			fmt.Printf("Run without --dry-run to perform cleanup\n")
			return
		}

		// Confirm if not forced
		if !force {
			fmt.Printf("Proceed with cleanup? [y/N]: ")
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Printf("%s%sCleanup cancelled.%s\n", colorBold, colorYellow, colorReset)
				return
			}
		}

		// Perform cleanup using orchestrator
		orchestrator, err := e2eframe.NewCleanupOrchestrator()
		if err != nil {
			fmt.Printf("%s%s✖ ERROR: Failed to create cleanup orchestrator: %v%s\n", colorBold, colorRed, err, colorReset)
			os.Exit(1)
		}
		defer orchestrator.Close()

		fmt.Printf("%s%sCleaning up resources...%s\n", colorBold, colorCyan, colorReset)
		result, err := orchestrator.CleanupOrphanedResources(ctx, opts)
		if err != nil {
			fmt.Printf("%s%s✖ ERROR: Cleanup failed: %v%s\n", colorBold, colorRed, err, colorReset)
			os.Exit(1)
		}

		// Display results
		fmt.Println()
		if result.ContainersRemoved > 0 || result.NetworksRemoved > 0 {
			fmt.Printf("%s%s✓ Cleanup completed in %v%s\n", colorBold, colorGreen, result.Duration.Round(time.Millisecond), colorReset)
		}

		fmt.Printf("  Containers: %d found, %s%d removed%s", result.ContainersFound, colorGreen, result.ContainersRemoved, colorReset)
		if result.ContainersFailed > 0 {
			fmt.Printf(", %s%d failed%s", colorRed, result.ContainersFailed, colorReset)
		}
		fmt.Println()

		fmt.Printf("  Networks:   %d found, %s%d removed%s", result.NetworksFound, colorGreen, result.NetworksRemoved, colorReset)
		if result.NetworksFailed > 0 {
			fmt.Printf(", %s%d failed%s", colorRed, result.NetworksFailed, colorReset)
		}
		fmt.Println()

		if len(result.Errors) > 0 {
			fmt.Printf("\n%s%sErrors occurred during cleanup:%s\n", colorBold, colorYellow, colorReset)
			for _, cleanupErr := range result.Errors {
				fmt.Printf("  ✖ %v\n", cleanupErr)
			}
			os.Exit(1)
		}
	},
}

// ScaffoldTest creates a new test suite in ./tests/<name>.
func ScaffoldTest(testName string, templates []string) error {
	if testName == "" {
		testName = "Mytest"
	}

	suiteDir := filepath.Join(e2eframe.TestsDir, testName)
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", suiteDir, err)
	}

	mongoUnitTmpl := `  - name: mongodb
    kind: mongo
    image: mongo:6.0
    migrations: db.js
    app_port: 27017
    startup_timeout: 15s`
	httpUnitTmpl := `  - name: app
    kind: http
    dockerfile: Dockerfile
    app_port: 8080
    healthcheck: /v1/health
    startup_timeout: 4m
    env_file: .env
    env:
      - DB_DSN={{ mongodb.dsn }}`
	httpMockUnitTmpl := `  - name: app
    kind: httpmock
    app_port: 8080
    routes:
      - path: /healthcheck
        method: GET
        response:
          status: 200
          body:
            data: ok
          headers:
            Content-Type: "{{ content_type_inline }}"`

	var units []string

	if len(templates) == 0 {
		units = append(units, mongoUnitTmpl, httpUnitTmpl, httpMockUnitTmpl)
	} else {
		for _, tmpl := range templates {
			switch tmpl {
			case "mongo":
				units = append(units, mongoUnitTmpl)
			case "http":
				units = append(units, httpUnitTmpl)
			case "httpmock":
				units = append(units, httpMockUnitTmpl)
			default:
				return fmt.Errorf("unknown template: %s", tmpl)
			}
		}
	}

	unitsTmpl := strings.Join(units, "\n")

	// scaffold test.yaml
	configPath := filepath.Join(suiteDir, e2eframe.SuiteYamlFile)
	tmpl := fmt.Sprintf(`kind: e2e_test:v1
name: %s

fixtures:
  - name: content_type_inline
    value: application/json; charset=utf-8

units:
%s

target: app

tests:
  - name: ping
    kind: http
    request:
      path: /v1/health
      method: GET
      timeout: 5s
    expect:
      status: 200
      body_asserts:
        data:
          present: true
          equals: "ok"
      header_asserts:
        Content-Type:
          present: true
          equals: "{{ content_type_inline }}"
`, testName, unitsTmpl)

	content := tmpl
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("could not write file %s: %w", configPath, err)
	}

	// scaffold empty db.js
	//dbPath := filepath.Join(suiteDir, DbMigrationFileName)
	//if err := os.WriteFile(dbPath, []byte{}, 0644); err != nil {
	//	return fmt.Errorf("could not write file %s: %w", dbPath, err)
	//}

	return nil
}

func init() {
	os.Setenv("DOCKER_API_VERSION", "1.45") // Fixes compatibility issues with Docker API versions

	rootCmd.Flags().BoolP("verbose", "v", false, "enable detailed logs")
	rootCmd.Flags().Bool("pretty", true, "pretty print output")
	rootCmd.Flags().Bool("debug", false, "enable debug mode")
	rootCmd.Flags().Bool("parallel", false, "run tests in parallel")
	rootCmd.Flags().
		String("suite", "", "run specific test suites (comma-separated), e.g. 'ene --suite=suite1,suite2' or partial matches 'ene --suite=TestService_,_Function'")
	rootCmd.Flags().String("html", "", "generate HTML report to this path") // new
	rootCmd.Flags().String("json", "", "generate JSON report to this path")
	rootCmd.Flags().String("base-dir", "", "base directory for tests, defaults to current directory")
	rootCmd.Flags().Bool("cleanup-cache", false, "cleanup old cached Docker images to prevent bloat")

	scaffoldTestCmd.Flags().
		String("tmpl", "", "templates to use for scaffolding, e.g. 'e2e scaffold-test my_test --tmpl=mongo,httpmock'")

	dryRunCmd.Flags().BoolP("verbose", "v", false, "enable detailed logs")
	dryRunCmd.Flags().Bool("debug", false, "enable debug mode")
	dryRunCmd.Flags().String("base-dir", "", "base directory for tests, defaults to current directory")

	listSuitesCmd.Flags().String("base-dir", "", "base directory for tests, defaults to current directory")

	cleanupCmd.Flags().Bool("dry-run", false, "show what would be removed without actually removing")
	cleanupCmd.Flags().Bool("force", false, "skip confirmation prompt")
	cleanupCmd.Flags().Bool("all", false, "include all matching resources, even if in use")
	cleanupCmd.Flags().String("older-than", "", "only clean resources older than this duration (e.g., 1h, 30m)")
	cleanupCmd.Flags().BoolP("verbose", "v", false, "show detailed information about resources")

	rootCmd.AddCommand(scaffoldTestCmd)
	rootCmd.AddCommand(dryRunCmd)
	rootCmd.AddCommand(listSuitesCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(versionCmd)

	// Add custom completion for --suite flag
	rootCmd.RegisterFlagCompletionFunc("suite", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		baseDir := cmd.Flag("base-dir").Value.String()
		suiteNames, err := e2eframe.ListTestSuiteNames(baseDir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		// Handle comma-separated values
		// If toComplete contains commas, we want to complete the part after the last comma
		if strings.Contains(toComplete, ",") {
			parts := strings.Split(toComplete, ",")
			prefix := strings.Join(parts[:len(parts)-1], ",") + ","
			lastPart := parts[len(parts)-1]

			var completions []string
			for _, suite := range suiteNames {
				if strings.HasPrefix(suite, lastPart) {
					completions = append(completions, prefix+suite)
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		}

		// No comma found, filter based on partial match
		var completions []string
		for _, suite := range suiteNames {
			if strings.HasPrefix(suite, toComplete) {
				completions = append(completions, suite)
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})

	// Add completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = false
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
