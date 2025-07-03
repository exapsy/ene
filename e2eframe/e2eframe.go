package e2eframe

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed assets/html_report.tmpl
var defaultHTMLTemplate string

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
		return nil, err
	}
	defer file.Close()

	type TestSuiteConfig struct {
		Kind string `yaml:"kind"`
	}

	var config TestSuiteConfig
	if err := yaml.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	switch config.Kind {
	case string(ConfigKindE2ETest):
		file.Seek(0, 0) // Reset file pointer to the beginning

		var testSuiteConfig TestSuiteConfigV1
		if err := yaml.NewDecoder(file).Decode(&testSuiteConfig); err != nil {
			return nil, err
		}

		base := filepath.Dir(path)

		params := CreateSuiteParams{RelativePath: base}

		testSuite, err := testSuiteConfig.CreateTestSuite(params)
		if err != nil {
			return nil, err
		}

		return testSuite, nil
	}

	return nil, fmt.Errorf("unrecognized test suite kind: %s", config.Kind)
}

func LoadTestSuites() ([]TestSuite, error) {
	testsDir, err := os.ReadDir(TestsDir)
	if err != nil {
		return nil, err
	}

	var testSuites []TestSuite

	for _, testDir := range testsDir {
		testDirPath := filepath.Join(TestsDir, testDir.Name())

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
				return nil, err
			}

			testSuites = append(testSuites, testSuite)
		}
	}

	return testSuites, nil
}

type RunOpts struct {
	FilterFunc func(test, testName string) bool
	Verbose    bool
	Parallel   bool
	Events     EventSink
	MaxRetries int    // Number of retries for failed tests
	RetryDelay string // Delay between retries (e.g. "2s")
	Debug      bool   // Enable debug mode
}

func Run(ctx context.Context, opts *RunOpts) error {
	var err error

	testSuites, err := LoadTestSuites()
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

		//runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		//defer cancel()
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
				FilterFunc: opts.FilterFunc,
				Verbose:    opts.Verbose,
				EventSink:  events,
				MaxRetries: opts.MaxRetries,
				RetryDelay: opts.RetryDelay,
				Debug:      opts.Debug,
			})
			if err != nil {
				events <- &BaseEvent{
					EventType:    EventSuiteError,
					EventTime:    time.Now(),
					Suite:        testSuite.Name(),
					EventMessage: fmt.Sprintf("Error running test suite %s: %v", testSuite.Name(), err),
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
			FilterFunc: opts.FilterFunc,
			Verbose:    opts.Verbose,
			EventSink:  events,
			MaxRetries: opts.MaxRetries,
			RetryDelay: opts.RetryDelay,
			Debug:      opts.Debug,
		})
		if err != nil {
			events <- &BaseEvent{
				EventType:    EventSuiteError,
				EventTime:    time.Now(),
				Suite:        testSuite.Name(),
				EventMessage: fmt.Sprintf("Error running test suite %s: %v", testSuite.Name(), err),
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
