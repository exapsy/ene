/*
 * This program runs the e2e tests for the application.
 */
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/exapsy/ene/e2eframe"
	_ "github.com/exapsy/ene/plugins/httpmockunit"
	_ "github.com/exapsy/ene/plugins/httptest"
	_ "github.com/exapsy/ene/plugins/httpunit"
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
	Use:     "e2e",
	Short:   "Run e2e tests or scaffold a new suite",
	Long:    `When called with no sub-command, runs all e2e tests. Use "e2e scaffold-test" to create a new suite.`,
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

		// Collect test results
		eventChan := e2eframe.NewEventChannel()
		var eventSink e2eframe.EventSink = eventChan

		// Use optimized defaults for better performance
		maxRetries := 3       // Keep reliable retry behavior
		isCleanupCache = true // Always cleanup for better performance

		err := e2eframe.Run(context.Background(), &e2eframe.RunOpts{
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

// ScaffoldTest creates a new test suite in ./tests/<name>.
func ScaffoldTest(testName string, templates []string) error {
	if testName == "" {
		testName = "Mytest"
	}

	suiteDir := filepath.Join(e2eframe.TestsDir, testName)
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", suiteDir, err)
	}

	mongoUnitTmpl := `
  - name: mongodb
    kind: mongo
    image: mongo:6.0
	migrations: db.js
	app_port: 27017
	startup_timeout: 15s`
	httpUnitTmpl := `
  - name: app
	kind: http
	dockerfile: Dockerfile
	app_port: 8080
	healthcheck: /v1/health
	startup_timeout: 4m
	env_file: .env
	env:
	  - DB_DSN={{ mongodb.dsn }}`
	httpMockUnitTmpl := `
  - name: app
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

	var unitsTmpl string

	if len(templates) == 0 {
		unitsTmpl = mongoUnitTmpl + httpUnitTmpl + httpMockUnitTmpl
	} else {
		for _, tmpl := range templates {
			switch tmpl {
			case "mongo":
				unitsTmpl += mongoUnitTmpl
			case "http":
				unitsTmpl += httpUnitTmpl
			case "httpmock":
				unitsTmpl += httpMockUnitTmpl
			default:
				return fmt.Errorf("unknown template: %s", tmpl)
			}
		}
	}

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
        - path: data
          present: true
          equal: "ok"
      header_asserts:
        - name: Content-Type
          present: true
		  equal: "{{ content_type_inline }}"
`, testName, strings.TrimSpace(unitsTmpl))

	content := fmt.Sprintf(tmpl, testName, testName)
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
		String("suite", "", "run specific test suites, e.g. 'e2e --suite=healthcheck,TestService_,_Function'")
	rootCmd.Flags().String("html", "", "generate HTML report to this path") // new
	rootCmd.Flags().String("json", "", "generate JSON report to this path")
	rootCmd.Flags().String("base-dir", "", "base directory for tests, defaults to current directory")
	rootCmd.Flags().Bool("cleanup-cache", false, "cleanup old cached Docker images to prevent bloat")

	scaffoldTestCmd.Flags().
		String("tmpl", "", "templates to use for scaffolding, e.g. 'e2e scaffold-test my_test --tmpl=mongo,httpmock'")

	dryRunCmd.Flags().BoolP("verbose", "v", false, "enable detailed logs")
	dryRunCmd.Flags().Bool("debug", false, "enable debug mode")
	dryRunCmd.Flags().String("base-dir", "", "base directory for tests, defaults to current directory")

	rootCmd.AddCommand(scaffoldTestCmd)
	rootCmd.AddCommand(dryRunCmd)
	rootCmd.AddCommand(versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
