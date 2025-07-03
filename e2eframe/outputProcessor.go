package e2eframe

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type EventConsumer interface {
	// ConsumeEvent processes a TestEvent.
	ConsumeEvent(event Event) error
}

// OutputProcessor consumes the events of tests,
// and outputs the results in a specific format or sink (html, stdio etc.).
type OutputProcessor interface {
	EventConsumer
	// Flush is called at the end of the test suite to ensure all events are processed.
	// It may be used to finalize output, close files, or perform any cleanup.
	Flush() error
}

// StdoutHumanOutputProcessor is an OutputProcessor that prints test events to stdout in a human-readable format.
type StdoutHumanOutputProcessor struct {
	// Sink is the output stream where events will be printed.
	Sink io.Writer
	// Pretty indicates whether to pretty-print the output with colors and formatting.
	Pretty  bool
	Verbose bool
	color   Color
	// testsSecretary is used to keep track of running tests and their statuses.
	testsSecretary *TestsSecretary
}

type StdoutHumanOutputProcessorParams struct {
	// Output is the output stream where events will be printed.
	Output io.Writer
	// Pretty indicates whether to pretty-print the output with colors and formatting.
	Pretty bool
	// Verbose indicates whether to print detailed information about the test execution.
	Verbose bool
	// TestsSecretary is used to keep track of running tests and their statuses.
	TestsSecretary *TestsSecretary
}

func NewStdoutHumanOutputProcessor(params StdoutHumanOutputProcessorParams) OutputProcessor {
	proc := &StdoutHumanOutputProcessor{
		Sink: params.Output,

		// Formatting options
		Pretty:  params.Pretty,
		Verbose: params.Verbose,

		color:          NewColor(params.Pretty),
		testsSecretary: params.TestsSecretary,
	}

	color := proc.color
	proc.printf("%s%s▶ RUNNING%s\n",
		color.Bold, color.Blue, color.Reset)
	proc.printf("%s%s══════════════════════════════════════════════════════════════%s\n",
		color.Bold, color.Blue, color.Reset)

	return proc
}

func (p *StdoutHumanOutputProcessor) printf(format string, args ...any) {
	if p.Sink != nil {
		fmt.Fprintf(p.Sink, format, args...)
	} else {
		fmt.Printf(format, args...)
	}
}

func (p *StdoutHumanOutputProcessor) ConsumeEvent(event Event) error {
	color := p.color

	switch event.Type() {
	case EventSuiteStarted:
		p.printf("%s▶ SUITE STARTED: %s%s\n",
			color.Cyan+color.Bold, event.SuiteName(), color.Reset,
		)

	case EventContainerStarting:
		if !p.Verbose {
			return nil
		}

		if unitEvent, ok := event.(*UnitEvent); ok {
			p.printf("%s  ⚙ CONTAINER STARTING:%s %s (%s)\n",
				color.Yellow, color.Reset, unitEvent.UnitName, unitEvent.UnitKind)
		}
	case EventContainerStarted:
		if !p.Verbose {
			return nil
		}

		if unitEvent, ok := event.(*UnitEvent); ok {
			p.printf("%s  ▶ CONTAINER STARTED:%s %s at %s\n",
				color.Blue, color.Reset, unitEvent.UnitName, unitEvent.Endpoint)
		}

	case EventContainerHealthy:
		if !p.Verbose {
			return nil
		}

		if unitEvent, ok := event.(*UnitEvent); ok {
			p.printf("%s  ✓ CONTAINER READY:%s %s at %s\n",
				color.Green, color.Reset, unitEvent.UnitName, unitEvent.Endpoint)
		}

	case EventTestStarted:
		if testEvent, ok := event.(*TestEvent); ok {
			p.printf("%s  ▶ TEST RUNNING:%s %s\n",
				color.Blue, color.Reset, testEvent.TestName)
		}

	case EventTestCompleted:
		if testEvent, ok := event.(*TestEvent); ok {
			if testEvent.Passed {
				p.printf("%s  ✓ TEST PASSED:%s %s (%.2fs)\n",
					color.Green, color.Reset, testEvent.TestName, testEvent.Duration.Seconds())
			} else {
				p.printf("%s  ✖ TEST FAILED:%s %s (%.2fs) - %s\n",
					color.Red, color.Reset, testEvent.TestName, testEvent.Duration.Seconds(), testEvent.Message())
			}
		}

	case EventTestRetrying:
		if testEvent, ok := event.(*TestRetryingEvent); ok {
			retryCount := testEvent.RetryCount
			maxRetries := testEvent.MaxRetries
			p.printf("%s  🔄 TEST RETRYING:%s %s (attempt %d/%d)\n",
				color.Yellow, color.Reset, testEvent.TestName, retryCount, maxRetries)
		}

	case EventSuiteSkipped:
		if suiteEvent, ok := event.(*SuiteSkippedEvent); ok {
			if p.Verbose {
				p.printf("%s⏭ SUITE SKIPPED:%s %s - %s\n",
					color.Purple, color.Reset, suiteEvent.SuiteName(), suiteEvent.Message())
			}
		}

	case EventSuiteError:
		if suiteEvent, ok := event.(*BaseEvent); ok {
			p.printf("%s✖ SUITE ERROR:%s %s - %s\n",
				color.Red, color.Reset, suiteEvent.SuiteName(), suiteEvent.Message())
		}

	case EventNetworkDestroyed:
		if networkEvent, ok := event.(*BaseEvent); ok {
			if p.Verbose {
				p.printf("%s  🗑 NETWORK DESTROYED:%s %s\n",
					color.Yellow, color.Reset, networkEvent.Message())
			}
		}

	case EventNetworkCreated:
		if networkEvent, ok := event.(*BaseEvent); ok {
			if p.Verbose {
				p.printf("%s  🆕 NETWORK CREATED:%s %s\n",
					color.Green, color.Reset, networkEvent.Message())
			}
		}
	}

	return nil
}

func (p *StdoutHumanOutputProcessor) Flush() error {
	color := p.color

	// No specific flush logic needed for stdout output
	startTime := p.testsSecretary.StartTime()
	passedTests := p.testsSecretary.PassedTests()
	failedTests := p.testsSecretary.FailedTests()
	skippedTests := p.testsSecretary.SkippedTests()

	if len(passedTests) > 0 {
		fmt.Printf("\n%s%s✓ PASSED%s\n",
			color.Bold, color.Green, color.Reset)
		fmt.Printf("%s%s──────────────────────────────────────────────────────────────%s\n",
			color.Bold, color.Green, color.Reset)

		// Group by test suite
		passedBySuite := make(map[string][]TestEvent)

		for _, test := range passedTests {
			suiteName := test.SuiteName()
			if suiteName == "" {
				suiteName = "Unknown Suite"
			}

			passedBySuite[suiteName] = append(passedBySuite[suiteName], test)
		}

		for suiteName, tests := range passedBySuite {
			fmt.Printf("%s%s✓ Suite: %s%s\n", color.Bold, color.Green, suiteName, color.Reset)

			for _, test := range tests {
				fmt.Printf("  %s%s✓ %s%s\n", color.Bold, color.Green, test.TestName, color.Reset)
			}
		}
	}

	elapsedTime := time.Since(startTime)
	total := len(p.testsSecretary.CompletedTests())

	if len(failedTests) > 0 {
		fmt.Printf("\n%s%s▶ FAILED%s\n",
			color.Bold, color.Red, color.Reset)
		fmt.Printf("%s%s══════════════════════════════════════════════════════════════%s\n",
			color.Bold, color.Red, color.Reset)

		// Group failures by suite
		failuresBySuite := make(map[string][]TestEvent)
		for _, failure := range failedTests {
			failuresBySuite[failure.SuiteName()] = append(
				failuresBySuite[failure.SuiteName()],
				failure,
			)
		}

		// Print failures grouped by suite
		for suiteName, suiteFailures := range failuresBySuite {
			fmt.Printf("\n%s%s▶ Suite: %s%s\n",
				color.Bold, color.Yellow, suiteName, color.Reset)

			for _, failure := range suiteFailures {
				fmt.Printf("\n  %s%s✖ Test: %s%s\n",
					color.Bold, color.Red, failure.TestName, color.Reset)

				// Split error messages if there are multiple

				errorMsg := failure.Message()
				if strings.TrimSpace(errorMsg) == "" {
					continue
				}

				// if error is multi-line, add indentation
				if strings.Contains(errorMsg, "\n") {
					lines := strings.Split(errorMsg, "\n")
					for i, line := range lines {
						if i == 0 {
							errorMsg = fmt.Sprintf("    %s", line)
						} else {
							errorMsg += fmt.Sprintf("\n    %s", line)
						}
					}
				} else {
					errorMsg = fmt.Sprintf("    %s", errorMsg)
				}

				fmt.Printf("%s\n", errorMsg)
			}
		}
	}

	// Print summary
	fmt.Printf("\n%s%s▶ TEST SUMMARY (%.2fs)%s\n",
		color.Bold, color.Blue, elapsedTime.Seconds(), color.Reset)
	fmt.Printf("%s%s══════════════════════════════════════════════════════════════%s\n",
		color.Bold, color.Blue, color.Reset)

	if len(failedTests) > 0 {
		fmt.Printf(
			"%s%s✖ FAILED:%s %d/%d (%.1f%%)\n",
			color.Bold,
			color.Red,
			color.Reset,
			len(failedTests),
			total,
			float64(len(failedTests))/float64(total)*100,
		)
	}

	if len(passedTests) > 0 {
		fmt.Printf(
			"%s%s✓ PASSED:%s %d/%d (%.1f%%)\n",
			color.Bold,
			color.Green,
			color.Reset,
			len(passedTests),
			total,
			float64(len(passedTests))/float64(total)*100,
		)
	}

	fmt.Printf("%s%s▶ TOTAL:%s %d tests\n",
		color.Bold, color.White, color.Reset, total)

	// Update summary to include skipped tests
	if len(skippedTests) > 0 {
		fmt.Printf("%s%s▶ SKIPPED:%s %d tests (filtered out)\n",
			color.Bold, color.Cyan, color.Reset, len(skippedTests))
	}

	return nil
}

// HTMLReportProcessor generates an HTML report of test results.
type HTMLReportProcessor struct {
	// File where the HTML report will be written
	OutputFile string
	// HTML template content
	Template string
	// Used to track tests and their statuses
	testsSecretary *TestsSecretary
}

type HTMLReportProcessorParams struct {
	// Path where the HTML report will be written
	OutputFile string
	// HTML template content as a string
	Template string
	// Test secretary to track test execution
	TestsSecretary *TestsSecretary
}

// NewHTMLReportProcessor creates a new HTMLReportProcessor.
func NewHTMLReportProcessor(params HTMLReportProcessorParams) (OutputProcessor, error) {
	// Create output directory if it doesn't exist
	dir := filepath.Dir(params.OutputFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	return &HTMLReportProcessor{
		OutputFile:     params.OutputFile,
		Template:       params.Template,
		testsSecretary: params.TestsSecretary,
	}, nil
}

// ConsumeEvent collects test events (no direct action needed as TestsSecretary handles it).
func (p *HTMLReportProcessor) ConsumeEvent(event Event) error {
	// The testsSecretary already collects all the events we need
	return nil
}

// Flush generates the HTML report and writes it to the output file.
func (p *HTMLReportProcessor) Flush() error {
	// Create the output file
	file, err := os.Create(p.OutputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	// Prepare data for the template
	endTime := time.Now()
	startTime := p.testsSecretary.StartTime()
	duration := endTime.Sub(startTime)

	mimeType := "image/svg+xml"

	// Read the SVG file
	dataURI := fmt.Sprintf("data:%s;base64,%s",
		mimeType,
		base64.StdEncoding.EncodeToString(logoBytes),
	)
	logoData := template.URL(dataURI) // Keep using template.URL for proper escaping

	templateData := map[string]interface{}{
		"StartTime":     startTime,
		"EndTime":       endTime,
		"Duration":      duration,
		"PassedTests":   p.testsSecretary.PassedTests(),
		"FailedTests":   p.testsSecretary.FailedTests(),
		"SkippedSuites": p.testsSecretary.SkippedTests(),
		"TotalTests":    len(p.testsSecretary.CompletedTests()),
		"TotalPassed":   p.testsSecretary.TotalPassedTests(),
		"TotalFailed":   p.testsSecretary.TotalFailedTests(),
		"TotalSkipped":  p.testsSecretary.TotalSkippedTests(),
		"LogoBase64":    logoData,
	}

	// Group tests by suite for better organization in report
	testsBySuite := make(map[string][]TestEvent)

	for _, test := range p.testsSecretary.CompletedTests() {
		suite := test.SuiteName()
		if suite == "" {
			suite = "Unknown Suite"
		}

		testsBySuite[suite] = append(testsBySuite[suite], test)
	}

	templateData["TestsBySuite"] = testsBySuite

	// Create function map BEFORE parsing the template
	funcMap := template.FuncMap{
		"add":     func(a, b int) int { return a + b },
		"div":     func(a, b float64) float64 { return a / b },
		"mul":     func(a, b float64) float64 { return a * b },
		"float64": func(i int) float64 { return float64(i) },
	}

	// Parse the template with functions already registered
	tmpl, err := template.New("report").Funcs(funcMap).Parse(p.Template)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	if err := tmpl.Execute(file, templateData); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	fmt.Printf("HTML report generated: %s\n", p.OutputFile)

	return nil
}

// JSONReportProcessor generates a JSON report of test results.
type JSONReportProcessor struct {
	// File where the JSON report will be written
	OutputFile string
	// Used to track tests and their statuses
	testsSecretary *TestsSecretary
}

type JSONReportProcessorParams struct {
	// Path where the JSON report will be written
	OutputFile string
	// Test secretary to track test execution
	TestsSecretary *TestsSecretary
}

// NewJSONReportProcessor creates a new JSONReportProcessor.
func NewJSONReportProcessor(params JSONReportProcessorParams) (OutputProcessor, error) {
	// Create output directory if it doesn't exist
	dir := filepath.Dir(params.OutputFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	return &JSONReportProcessor{
		OutputFile:     params.OutputFile,
		testsSecretary: params.TestsSecretary,
	}, nil
}

// ConsumeEvent collects test events (no direct action needed as TestsSecretary handles it).
func (p *JSONReportProcessor) ConsumeEvent(event Event) error {
	// The testsSecretary already collects all the events we need
	return nil
}

// Flush generates the JSON report and writes it to the output file.
func (p *JSONReportProcessor) Flush() error {
	// Create the output file
	file, err := os.Create(p.OutputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	// Prepare data for JSON serialization
	endTime := time.Now()
	startTime := p.testsSecretary.StartTime()
	duration := endTime.Sub(startTime)

	// Structure for the JSON output
	jsonData := map[string]interface{}{
		"metadata": map[string]interface{}{
			"startTime":    startTime.Format(time.RFC3339),
			"endTime":      endTime.Format(time.RFC3339),
			"durationMs":   duration.Milliseconds(),
			"totalTests":   len(p.testsSecretary.CompletedTests()),
			"totalPassed":  p.testsSecretary.TotalPassedTests(),
			"totalFailed":  p.testsSecretary.TotalFailedTests(),
			"totalSkipped": p.testsSecretary.TotalSkippedTests(),
		},
		"suites":        []map[string]interface{}{},
		"skippedSuites": []map[string]interface{}{},
	}

	// Group tests by suite
	testsBySuite := make(map[string][]TestEvent)

	for _, test := range p.testsSecretary.CompletedTests() {
		suite := test.SuiteName()
		if suite == "" {
			suite = "Unknown Suite"
		}

		testsBySuite[suite] = append(testsBySuite[suite], test)
	}

	// Convert test data to JSON-friendly format
	suites := []map[string]interface{}{}

	for suiteName, tests := range testsBySuite {
		passCount := 0

		for _, test := range tests {
			if test.Passed {
				passCount++
			}
		}

		suiteData := map[string]interface{}{
			"name":       suiteName,
			"totalTests": len(tests),
			"passed":     passCount,
			"failed":     len(tests) - passCount,
			"tests":      []map[string]interface{}{},
		}

		testItems := []map[string]interface{}{}

		for _, test := range tests {
			testData := map[string]interface{}{
				"name":     test.TestName,
				"passed":   test.Passed,
				"duration": test.Duration.Milliseconds(),
			}

			if !test.Passed {
				testData["message"] = test.Message()
			}

			testItems = append(testItems, testData)
		}

		suiteData["tests"] = testItems

		suites = append(suites, suiteData)
	}

	jsonData["suites"] = suites

	// Add skipped suites
	skippedSuites := []map[string]interface{}{}
	for _, skipped := range p.testsSecretary.SkippedTests() {
		skippedSuites = append(skippedSuites, map[string]interface{}{
			"name":       skipped.SuiteName(),
			"message":    skipped.Message(),
			"totalTests": skipped.TotalSuiteTests,
		})
	}

	jsonData["skippedSuites"] = skippedSuites

	// Encode as JSON with indentation for readability
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(jsonData); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	fmt.Printf("JSON report generated: %s\n", p.OutputFile)

	return nil
}
