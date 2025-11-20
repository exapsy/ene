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

	"github.com/exapsy/ene/e2eframe/ui"
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
	Debug   bool
	color   Color
	// testsSecretary is used to keep track of running tests and their statuses.
	testsSecretary *TestsSecretary
	// renderer is the modern UI renderer
	renderer ui.Renderer
	// suiteCount tracks total and current suite index
	totalSuites  int
	currentSuite int
}

type StdoutHumanOutputProcessorParams struct {
	// Output is the output stream where events will be printed.
	Output io.Writer
	// Pretty indicates whether to pretty-print the output with colors and formatting.
	Pretty bool
	// Verbose indicates whether to print detailed information about the test execution.
	Verbose bool
	// Debug indicates whether to print full error details
	Debug bool
	// TestsSecretary is used to keep track of running tests and their statuses.
	TestsSecretary *TestsSecretary
	// TotalSuites is the total number of suites to run
	TotalSuites int
}

func NewStdoutHumanOutputProcessor(params StdoutHumanOutputProcessorParams) OutputProcessor {
	// Determine render mode based on verbose flag
	mode := ui.RenderModeNormal
	if params.Verbose {
		mode = ui.RenderModeVerbose
	}

	// Create modern renderer
	renderer := ui.NewModernRenderer(ui.RendererConfig{
		Writer: params.Output,
		Mode:   mode,
		Pretty: params.Pretty,
		Debug:  params.Debug,
		IsTTY:  false, // Will be auto-detected
	})

	proc := &StdoutHumanOutputProcessor{
		Sink: params.Output,

		// Formatting options
		Pretty:  params.Pretty,
		Verbose: params.Verbose,
		Debug:   params.Debug,

		color:          NewColor(params.Pretty),
		testsSecretary: params.TestsSecretary,
		renderer:       renderer,
		totalSuites:    params.TotalSuites,
		currentSuite:   0,
	}

	// Print header
	color := proc.color
	proc.printf("%s%s▶ TEST RUN STARTED%s\n",
		color.Bold, color.Cyan, color.Reset)
	proc.printf("%s%s══════════════════════════════════════════════════════════════════════════%s\n",
		color.Dim, color.Gray, color.Reset)

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
	switch event.Type() {
	case EventSuiteStarted:
		p.currentSuite++
		suiteInfo := ui.SuiteInfo{
			Name:  event.SuiteName(),
			Index: p.currentSuite,
			Total: p.totalSuites,
		}
		return p.renderer.RenderSuiteStart(suiteInfo)

	case EventSuiteFinished:
		if suiteEvent, ok := event.(*SuiteFinishedEvent); ok {
			overhead := suiteEvent.TotalTime - suiteEvent.SetupTime - suiteEvent.TestTime
			if overhead < 0 {
				overhead = 0
			}

			suiteInfo := ui.SuiteFinishedInfo{
				Name:         suiteEvent.SuiteName(),
				SetupTime:    suiteEvent.SetupTime,
				TestTime:     suiteEvent.TestTime,
				TotalTime:    suiteEvent.TotalTime,
				Overhead:     overhead,
				PassedCount:  suiteEvent.PassedCount,
				FailedCount:  suiteEvent.FailedCount,
				SkippedCount: suiteEvent.SkippedCount,
			}
			return p.renderer.RenderSuiteFinished(suiteInfo)
		}

	case EventContainerStarting:
		if unitEvent, ok := event.(*UnitEvent); ok {
			containerInfo := ui.ContainerInfo{
				Name: unitEvent.UnitName,
				Kind: string(unitEvent.UnitKind),
			}
			return p.renderer.RenderContainerStarting(containerInfo)
		}

	case EventContainerHealthy:
		if unitEvent, ok := event.(*UnitEvent); ok {
			containerInfo := ui.ContainerInfo{
				Name:     unitEvent.UnitName,
				Kind:     string(unitEvent.UnitKind),
				Endpoint: unitEvent.Endpoint,
				Duration: 0, // Will be calculated by renderer's tracker
			}
			return p.renderer.RenderContainerReady(containerInfo)
		}

	case EventTestStarted:
		if testEvent, ok := event.(*TestEvent); ok {
			testInfo := ui.TestInfo{
				SuiteName: testEvent.SuiteName(),
				Name:      testEvent.TestName,
			}
			return p.renderer.RenderTestStarted(testInfo)
		}

	case EventTestCompleted:
		if testEvent, ok := event.(*TestEvent); ok {
			errorMsg := ""
			if !testEvent.Passed {
				errorMsg = testEvent.Message()
				// Check if error implements PrettyError interface first
				if testEvent.Error != nil {
					if prettyErr, ok := testEvent.Error.(PrettyError); ok {
						errorMsg = prettyErr.PrettyString(p.Pretty)
					} else if friendlyMsg := FormatError(testEvent.Error, p.Debug); friendlyMsg != "" {
						errorMsg = friendlyMsg
					}
				}
			}

			testInfo := ui.TestInfo{
				SuiteName:    testEvent.SuiteName(),
				Name:         testEvent.TestName,
				Passed:       testEvent.Passed,
				Duration:     testEvent.Duration,
				ErrorMessage: errorMsg,
				RetryCount:   0, // Will be set if there were retries
			}
			return p.renderer.RenderTestCompleted(testInfo)
		}

	case EventTestRetrying:
		if testEvent, ok := event.(*TestRetryingEvent); ok {
			testInfo := ui.TestInfo{
				SuiteName: testEvent.SuiteName(),
				Name:      testEvent.TestName,
			}
			return p.renderer.RenderTestRetrying(testInfo, testEvent.RetryCount, testEvent.MaxRetries)
		}

	case EventNetworkCreated:
		// Show brief transition when network is being set up
		p.renderer.RenderTransition("Setting up network...")

	case EventScriptExecuting:
		if baseEvent, ok := event.(*BaseEvent); ok {
			msg := baseEvent.Message()
			if msg == "" {
				msg = "Running setup script..."
			}
			p.renderer.RenderTransition(msg)
		}

	case EventContainerPulling:
		if unitEvent, ok := event.(*UnitEvent); ok {
			p.renderer.RenderTransition(fmt.Sprintf("Pulling %s image...", unitEvent.UnitName))
		}

	case EventContainerStopped:
		// BaseEvent is sent as value, not pointer
		if baseEvent, ok := event.(BaseEvent); ok {
			msg := baseEvent.Message()
			if msg == "" {
				msg = "Stopping container..."
			}
			p.renderer.RenderTransition(msg)
		}

	case EventInfo:
		// BaseEvent is sent as value, not pointer
		if baseEvent, ok := event.(BaseEvent); ok {
			msg := baseEvent.Message()
			if msg != "" {
				p.renderer.RenderTransition(msg)
			}
		}

	case EventSuiteSkipped:
		// Handle skipped suites if needed
		// For now, we'll track this in the secretary and show in summary

	case EventSuiteError:
		// Clear any active spinner first
		p.renderer.ClearSpinner()

		// Handle suite errors
		color := p.color
		if suiteEvent, ok := event.(*SuiteErrorEvent); ok {
			var errorMsg string
			if suiteEvent.Error != nil {
				errorMsg = FormatError(suiteEvent.Error, p.Debug)
			} else {
				errorMsg = suiteEvent.Message()
			}
			// Split error message by newlines and indent each line
			lines := strings.Split(errorMsg, "\n")
			for i, line := range lines {
				if i == 0 {
					// First line with error icon
					p.printf("  %s✗%s  %s\n", color.Red, color.Reset, line)
				} else {
					// Subsequent lines indented further
					p.printf("     %s\n", line)
				}
			}
		} else if suiteEvent, ok := event.(*BaseEvent); ok {
			// Split error message by newlines and indent each line
			lines := strings.Split(suiteEvent.Message(), "\n")
			for i, line := range lines {
				if i == 0 {
					// First line with error icon
					p.printf("  %s✗%s  %s\n", color.Red, color.Reset, line)
				} else {
					// Subsequent lines indented further
					p.printf("     %s\n", line)
				}
			}
		}
	}

	return nil
}

func (p *StdoutHumanOutputProcessor) Flush() error {
	// Convert test secretary data to UI summary format
	passedTests := p.testsSecretary.PassedTests()
	failedTests := p.testsSecretary.FailedTests()
	skippedTests := p.testsSecretary.SkippedTests()

	// Convert to UI test info format
	passedInfos := make([]ui.TestInfo, len(passedTests))
	for i, test := range passedTests {
		passedInfos[i] = ui.TestInfo{
			SuiteName: test.SuiteName(),
			Name:      test.TestName,
			Passed:    true,
			Duration:  test.Duration,
		}
	}

	failedInfos := make([]ui.TestInfo, len(failedTests))
	for i, test := range failedTests {
		errorMsg := test.Message()
		if test.Error != nil {
			// Check if error implements PrettyError interface first
			if prettyErr, ok := test.Error.(PrettyError); ok {
				errorMsg = prettyErr.PrettyString(p.Pretty)
			} else if friendlyMsg := FormatError(test.Error, p.Debug); friendlyMsg != "" {
				errorMsg = friendlyMsg
			}
		}

		failedInfos[i] = ui.TestInfo{
			SuiteName:    test.SuiteName(),
			Name:         test.TestName,
			Passed:       false,
			Duration:     test.Duration,
			ErrorMessage: errorMsg,
		}
	}

	// Get timing information from renderer's tracker
	var containerTime, testTime time.Duration
	if renderer, ok := p.renderer.(*ui.ModernRenderer); ok {
		tracker := renderer.GetTracker()
		containerTime = tracker.GetTotalContainerTime()

		// Calculate total test execution time
		for _, test := range passedInfos {
			testTime += test.Duration
		}
		for _, test := range failedInfos {
			testTime += test.Duration
		}
	}

	summary := ui.Summary{
		TotalDuration:     time.Since(p.testsSecretary.StartTime()),
		TotalTests:        len(p.testsSecretary.CompletedTests()),
		PassedTests:       passedInfos,
		FailedTests:       failedInfos,
		SkippedTests:      len(skippedTests),
		ContainerTime:     containerTime,
		TestExecutionTime: testTime,
	}

	return p.renderer.RenderSummary(summary)
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
