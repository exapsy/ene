package ui

import (
	"io"
	"time"
)

// RenderMode defines how the UI should render output
type RenderMode int

const (
	// RenderModeNormal shows compact output with real-time updates
	RenderModeNormal RenderMode = iota
	// RenderModeVerbose shows all details including retries and infrastructure
	RenderModeVerbose
	// RenderModeCI shows non-interactive output suitable for CI/CD
	RenderModeCI
)

// Renderer is the main interface for rendering test output
type Renderer interface {
	// RenderSuiteStart renders when a suite starts
	RenderSuiteStart(suite SuiteInfo) error

	// RenderContainerStarting renders when a container is starting
	RenderContainerStarting(container ContainerInfo) error

	// RenderContainerReady renders when a container is ready
	RenderContainerReady(container ContainerInfo) error

	// RenderTestStarted renders when a test starts
	RenderTestStarted(test TestInfo) error

	// RenderTestRetrying renders when a test is retrying
	RenderTestRetrying(test TestInfo, attempt, maxAttempts int) error

	// RenderTestCompleted renders when a test completes
	RenderTestCompleted(test TestInfo) error

	// RenderSummary renders the final summary
	RenderSummary(summary Summary) error

	// Flush ensures all output is written
	Flush() error
}

// SuiteInfo contains information about a test suite
type SuiteInfo struct {
	Name     string
	Index    int // Current suite index (1-based)
	Total    int // Total number of suites
	TestsDir string
}

// ContainerInfo contains information about a container
type ContainerInfo struct {
	Name     string
	Kind     string
	Endpoint string
	Duration time.Duration
}

// TestInfo contains information about a test
type TestInfo struct {
	SuiteName    string
	Name         string
	Passed       bool
	Duration     time.Duration
	ErrorMessage string
	RetryCount   int
	MaxRetries   int
}

// Summary contains the final test run summary
type Summary struct {
	TotalDuration time.Duration
	TotalTests    int
	PassedTests   []TestInfo
	FailedTests   []TestInfo
	SkippedTests  int
}

// RendererConfig contains configuration for the renderer
type RendererConfig struct {
	Writer io.Writer
	Mode   RenderMode
	Pretty bool // Enable colors
	Debug  bool // Show debug information
	IsTTY  bool // Whether output is a TTY (for interactive features)
}

// Color codes for terminal output
type ColorScheme struct {
	Reset  string
	Bold   string
	Dim    string
	Red    string
	Green  string
	Yellow string
	Blue   string
	Cyan   string
	Gray   string
	White  string
}

// GetColorScheme returns a color scheme based on whether colors are enabled
func GetColorScheme(enabled bool) ColorScheme {
	if !enabled {
		return ColorScheme{}
	}

	return ColorScheme{
		Reset:  "\033[0m",
		Bold:   "\033[1m",
		Dim:    "\033[2m",
		Red:    "\033[31m",
		Green:  "\033[32m",
		Yellow: "\033[33m",
		Blue:   "\033[34m",
		Cyan:   "\033[36m",
		Gray:   "\033[90m",
		White:  "\033[97m",
	}
}
