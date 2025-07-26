package e2eframe

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

//go:embed assets/html_report.tmpl
var defaultHTMLTemplate string

//go:embed test_schema.json
var testSchemaJSON string

func GetDefaultHTMLTemplate() string {
	return defaultHTMLTemplate
}

//go:embed assets/logo.svg
var logoBytes []byte

const (
	// TestsDir is the directory where test suites are located.
	TestsDir      = "./tests"
	SuiteYamlFile = "suite.yml"
)

var (
	FixtureInterpolationRegex         = regexp.MustCompile(`{{\s*([a-zA-Z0-9_]+)\s*}}`)
	ServiceVariableInterpolationRegex = regexp.MustCompile(
		`{{\s*([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)\s*}}`,
	)
)

// InterpolateString replaces all occurrences of fixtures in the string
// The regex is of the form {{ (fixture_name) }}
//
// Example:
// "Hello {{ name }}!" with fixture { Name: "name", Value: "World" }
// will return "Hello World!".
func InterpolateString(regx *regexp.Regexp, str string, fixtures []Fixture) string {
	// Replace all occurrences of the regex in the string with the corresponding fixture value
	// The regex is of the form {{ (fixture_name) }}
	// The whole string should be replaced with the fixture value
	matches := regx.FindAllStringSubmatch(str, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue // No fixture name found
		}

		fixtureName := match[1]

		// Find the fixture by name
		var fixtureValue []byte

		for _, fixture := range fixtures {
			if fixture.Name() == fixtureName {
				fixtureValue = fixture.Value()

				break
			}
		}

		if len(fixtureValue) == 0 {
			// If fixture value is empty, skip replacement
			continue
		}

		// Replace the match with the fixture value
		str = strings.ReplaceAll(str, match[0], string(fixtureValue))
	}

	return str
}

func LoadTestSuite(path string) (TestSuite, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open test suite file %s: %w", path, err)
	}
	defer file.Close()

	type TestSuiteConfig struct {
		Kind string `yaml:"kind"`
	}

	var config TestSuiteConfig
	if err := yaml.NewDecoder(file).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to read test suite kind from %s: %w", path, err)
	}

	switch config.Kind {
	case string(ConfigKindE2ETest):
		file.Seek(0, 0) // Reset file pointer to the beginning

		// Pre-validate using JSON schema
		if err := validateTestSuiteSchema(file, path); err != nil {
			return nil, err
		}

		file.Seek(0, 0) // Reset file pointer again for YAML parsing

		var testSuiteConfig TestSuiteConfigV1
		decoder := yaml.NewDecoder(file)
		if err := decoder.Decode(&testSuiteConfig); err != nil {
			// Try to provide more context about the YAML error
			if yamlErr, ok := err.(*yaml.TypeError); ok {
				return nil, NewYAMLError(yamlErr.Error(), path)
			}
			return nil, NewYAMLError(err.Error(), path)
		}

		suitePath := filepath.Dir(path)
		workingDir := filepath.Join(suitePath, "../../")

		params := CreateSuiteParams{
			RelativePath: suitePath,
			WorkingDir:   workingDir,
		}

		testSuite, err := testSuiteConfig.CreateTestSuite(params)
		if err != nil {
			return nil, fmt.Errorf("failed to create test suite from %s: %w", path, err)
		}

		return testSuite, nil

	default:
		return nil, fmt.Errorf("unsupported test suite kind: %s", config.Kind)
	}
}

// validateTestSuiteSchema validates the test suite YAML against the JSON schema
func validateTestSuiteSchema(file *os.File, path string) error {
	// Convert YAML to JSON for schema validation
	yamlBytes, err := io.ReadAll(file)
	if err != nil {
		return NewValidationError("failed to read test suite file", path, 0)
	}

	var yamlData interface{}
	if err := yaml.Unmarshal(yamlBytes, &yamlData); err != nil {
		return NewYAMLError(fmt.Sprintf("invalid YAML syntax: %s", err.Error()), path)
	}

	jsonBytes, err := json.Marshal(yamlData)
	if err != nil {
		return NewValidationError("failed to convert YAML to JSON for validation", path, 0)
	}

	// Load schema and document
	schemaLoader := gojsonschema.NewStringLoader(testSchemaJSON)
	documentLoader := gojsonschema.NewBytesLoader(jsonBytes)

	// Validate
	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return NewValidationError(fmt.Sprintf("schema validation failed: %s", err.Error()), path, 0)
	}

	if !result.Valid() {
		// Create a user-friendly error message from validation errors
		var errorMessages []string
		for _, desc := range result.Errors() {
			errorMessages = append(errorMessages, fmt.Sprintf("- %s at %s", desc.Description(), desc.Field()))
		}

		errorMsg := fmt.Sprintf("test suite validation failed:\n%s", strings.Join(errorMessages, "\n"))
		return NewValidationError(errorMsg, path, 0)
	}

	return nil
}

func LoadTestSuites(baseDir string) ([]TestSuite, error) {
	if baseDir == "" {
		baseDir, _ = os.Getwd() // Get current working directory if no baseDir is provided
	}

	testsDirPath := filepath.Join(baseDir, TestsDir)

	testsDir, err := os.ReadDir(testsDirPath)
	if err != nil {
		return nil, err
	}

	var testSuites []TestSuite

	for _, testDir := range testsDir {
		testDirPath := filepath.Join(testsDirPath, testDir.Name())

		testFiles, err := os.ReadDir(testDirPath)
		if err != nil {
			return nil, err
		}

		for _, testFile := range testFiles {
			if testFile.IsDir() {
				continue
			}

			if testFile.Name() != SuiteYamlFile {
				continue
			}

			testFilePath := filepath.Join(testDirPath, testFile.Name())

			testSuite, err := LoadTestSuite(testFilePath)
			if err != nil {
				return nil, fmt.Errorf("failed to load test suite from %s: %w", testFilePath, err)
			}

			testSuites = append(testSuites, testSuite)
		}
	}

	return testSuites, nil
}

type RunOpts struct {
	FilterFunc   func(test, testName string) bool
	Verbose      bool
	Parallel     bool
	Events       EventSink
	MaxRetries   int    // Number of retries for failed tests
	RetryDelay   string // Delay between retries (e.g. "2s")
	Debug        bool   // Enable debug mode
	BaseDir      string // Base directory for test suites
	CleanupCache bool   // Cleanup old cached Docker images to prevent bloat
}

func Run(ctx context.Context, opts *RunOpts) error {
	var err error

	testSuites, err := LoadTestSuites(opts.BaseDir)
	if err != nil {
		return fmt.Errorf("load test suites: %w", err)
	}

	filteredSuites := make([]TestSuite, 0, len(testSuites))

	for _, testSuite := range testSuites {
		isFilteredIn := opts.FilterFunc(testSuite.Name(), "")
		if opts.FilterFunc == nil || isFilteredIn {
			filteredSuites = append(filteredSuites, testSuite)
		} else {
			opts.Events <- &SuiteSkippedEvent{
				BaseEvent: BaseEvent{
					EventType:    EventSuiteSkipped,
					EventTime:    time.Now(),
					Suite:        testSuite.Name(),
					EventMessage: fmt.Sprintf("Test suite %s was skipped by filter", testSuite.Name()),
				},
				TotalSuiteTests: len(testSuite.Tests()),
			}

			continue
		}
	}

	testResults := make(chan TestResult)
	go func() {
		defer close(testResults)

		// runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		// defer cancel()
		runCtx := ctx

		done := make(chan struct{})

		go func() {
			// Wait for all tests to complete or for the context to be done
			defer close(done)
			// Close the events channel when done, so that the main goroutine can exit cleanly
			defer close(opts.Events)

			if opts.Parallel {
				runTestsInParallel(runCtx, filteredSuites, opts, opts.Events)
			} else {
				runTestsSequentially(runCtx, filteredSuites, opts, opts.Events)
			}
		}()

		// Wait for either completion or timeout
		select {
		case <-done:
			// All tests completed normally
		case <-runCtx.Done():
			// Timeout or cancellation occurred
			opts.Events <- &BaseEvent{
				EventType:    EventWarning,
				EventTime:    time.Now(),
				EventMessage: "Test run was cancelled or timed out",
			}
		}
	}()

	return nil
}

func runTestsInParallel(
	ctx context.Context,
	testSuites []TestSuite,
	opts *RunOpts,
	events EventSink,
) {
	wg := sync.WaitGroup{}

	for _, testSuite := range testSuites {
		wg.Add(1)

		if ctx.Err() != nil {
			break
		}

		go func(testSuite TestSuite) {
			defer wg.Done()

			err := testSuite.Run(ctx, &RunTestOptions{
				FilterFunc:   opts.FilterFunc,
				Verbose:      opts.Verbose,
				CleanupCache: opts.CleanupCache,
				EventSink:    events,
				MaxRetries:   opts.MaxRetries,
				RetryDelay:   opts.RetryDelay,
				Debug:        opts.Debug,
				BaseDir:      opts.BaseDir,
			})
			if err != nil {
				events <- &SuiteErrorEvent{
					BaseEvent: BaseEvent{
						EventType:    EventSuiteError,
						EventTime:    time.Now(),
						Suite:        testSuite.Name(),
						EventMessage: fmt.Sprintf("Error running test suite %s: %v", testSuite.Name(), err),
					},
					Error: err,
				}
			} else {
				events <- &BaseEvent{
					EventType:    EventSuiteCompleted,
					EventTime:    time.Now(),
					Suite:        testSuite.Name(),
					EventMessage: fmt.Sprintf("Completed test suite: %s", testSuite.Name()),
				}
			}
		}(testSuite)
	}

	wg.Wait()
}

func runTestsSequentially(
	ctx context.Context,
	testSuites []TestSuite,
	opts *RunOpts,
	events EventSink,
) {
	for _, testSuite := range testSuites {
		events <- &BaseEvent{
			EventType:    EventSuiteStarted,
			EventTime:    time.Now(),
			Suite:        testSuite.Name(),
			EventMessage: fmt.Sprintf("Starting test suite: %s", testSuite.Name()),
		}

		// Create test options with filter
		err := testSuite.Run(ctx, &RunTestOptions{
			FilterFunc:   opts.FilterFunc,
			Verbose:      opts.Verbose,
			CleanupCache: opts.CleanupCache,
			EventSink:    events,
			MaxRetries:   opts.MaxRetries,
			RetryDelay:   opts.RetryDelay,
			Debug:        opts.Debug,
			BaseDir:      opts.BaseDir,
		})
		if err != nil {
			events <- &SuiteErrorEvent{
				BaseEvent: BaseEvent{
					EventType:    EventSuiteError,
					EventTime:    time.Now(),
					Suite:        testSuite.Name(),
					EventMessage: fmt.Sprintf("Error running test suite %s: %v", testSuite.Name(), err),
				},
				Error: err,
			}
		} else {
			events <- &BaseEvent{
				EventType:    EventSuiteCompleted,
				EventTime:    time.Now(),
				Suite:        testSuite.Name(),
				EventMessage: fmt.Sprintf("Completed test suite: %s", testSuite.Name()),
			}
		}
	}
}
