package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ModernRenderer implements a clean, modern UI for test output
type ModernRenderer struct {
	writer  io.Writer
	mode    RenderMode
	colors  ColorScheme
	debug   bool
	isTTY   bool
	mu      sync.Mutex
	tracker *ProgressTracker

	// State for in-place updates
	lastLineLength int
	currentTest    string
}

// NewModernRenderer creates a new modern renderer
func NewModernRenderer(config RendererConfig) *ModernRenderer {
	// Auto-detect TTY if not specified
	isTTY := config.IsTTY
	if config.Writer == os.Stdout || config.Writer == os.Stderr {
		if f, ok := config.Writer.(*os.File); ok {
			isTTY = term.IsTerminal(int(f.Fd()))
		}
	}

	// Force CI mode for non-TTY environments
	mode := config.Mode
	if !isTTY && mode == RenderModeNormal {
		mode = RenderModeCI
	}

	return &ModernRenderer{
		writer:  config.Writer,
		mode:    mode,
		colors:  GetColorScheme(config.Pretty && isTTY),
		debug:   config.Debug,
		isTTY:   isTTY,
		tracker: NewProgressTracker(),
	}
}

// RenderSuiteStart renders when a suite starts
func (r *ModernRenderer) RenderSuiteStart(suite SuiteInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tracker.StartSuite(suite.Name)

	c := r.colors
	progress := ""
	if suite.Total > 0 {
		progress = fmt.Sprintf("[%d/%d] ", suite.Index, suite.Total)
	}

	line := fmt.Sprintf("\n%s%s%s%s%s\n",
		c.Cyan, c.Bold, progress, suite.Name, c.Reset)

	return r.write(line)
}

// RenderContainerStarting renders when a container is starting
func (r *ModernRenderer) RenderContainerStarting(container ContainerInfo) error {
	// Only show in verbose mode
	if r.mode != RenderModeVerbose {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tracker.StartContainer(container)

	c := r.colors
	line := fmt.Sprintf("  %s⚙  %s container starting...%s\n",
		c.Dim+c.Yellow, container.Name, c.Reset)

	return r.write(line)
}

// RenderContainerReady renders when a container is ready
func (r *ModernRenderer) RenderContainerReady(container ContainerInfo) error {
	// Show in normal and verbose modes
	if r.mode == RenderModeCI {
		return r.renderContainerReadyCI(container)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tracker.ReadyContainer(container)

	c := r.colors
	timeStr := formatDuration(container.Duration)
	endpoint := ""
	if container.Endpoint != "" {
		endpoint = fmt.Sprintf(" (%s%s%s)", c.Cyan, container.Endpoint, c.Gray)
	}

	line := fmt.Sprintf("  %s✓%s  %s ready%s %s%s%s\n",
		c.Green, c.Reset,
		container.Name,
		endpoint,
		c.Dim+c.Gray, timeStr, c.Reset)

	return r.write(line)
}

// renderContainerReadyCI renders container ready in CI mode
func (r *ModernRenderer) renderContainerReadyCI(container ContainerInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tracker.ReadyContainer(container)

	timeStr := formatDuration(container.Duration)
	endpoint := ""
	if container.Endpoint != "" {
		endpoint = fmt.Sprintf(" (%s)", container.Endpoint)
	}

	line := fmt.Sprintf("  ✓  %s ready%s %s\n",
		container.Name, endpoint, timeStr)

	return r.write(line)
}

// RenderTestStarted renders when a test starts
func (r *ModernRenderer) RenderTestStarted(test TestInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tracker.StartTest(test)
	r.currentTest = test.Name

	// In TTY mode with normal mode, we'll update this line in place
	// In non-TTY or verbose mode, show each step
	if r.mode != RenderModeVerbose && !r.isTTY {
		// Don't show test start in non-TTY normal mode (we'll show result)
		return nil
	}

	c := r.colors
	var line string
	if r.isTTY && r.mode == RenderModeNormal {
		// TTY normal mode: show in-place updating line
		line = fmt.Sprintf("  %s⋯%s  %s...",
			c.Dim+c.Gray, c.Reset, test.Name)
		r.lastLineLength = len(line)
	} else if r.mode == RenderModeVerbose {
		// Verbose mode: show with newline
		line = fmt.Sprintf("  ▶  %s...\n", test.Name)
	}

	if line != "" {
		return r.write(line)
	}
	return nil
}

// RenderTestRetrying renders when a test is retrying
func (r *ModernRenderer) RenderTestRetrying(test TestInfo, attempt, maxAttempts int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// In verbose mode, show each retry on its own line
	if r.mode == RenderModeVerbose {
		c := r.colors
		line := fmt.Sprintf("  %s⟳%s  %s attempt %d/%d...\n",
			c.Dim+c.Yellow, c.Reset, test.Name, attempt, maxAttempts)
		return r.write(line)
	}

	// In normal mode with TTY, update the same line in place
	if r.isTTY && r.mode == RenderModeNormal {
		c := r.colors
		line := fmt.Sprintf("  %s⟳%s  %s (attempt %d/%d)...",
			c.Dim+c.Yellow, c.Reset, test.Name, attempt, maxAttempts)

		// Clear previous line and write new one
		if r.lastLineLength > 0 {
			if err := r.clearLine(); err != nil {
				return err
			}
		}
		r.lastLineLength = len(line)
		return r.write(line)
	}

	// In non-TTY normal mode, don't show retries (we'll show final result with retry count)
	return nil
}

// RenderTestCompleted renders when a test completes
func (r *ModernRenderer) RenderTestCompleted(test TestInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tracker.CompleteTest(test)

	// Clear any in-progress line in TTY mode
	if r.isTTY && r.lastLineLength > 0 {
		if err := r.clearLine(); err != nil {
			return err
		}
		r.lastLineLength = 0
	}
	r.currentTest = ""

	c := r.colors
	timeStr := formatDuration(test.Duration)

	if test.Passed {
		// In non-verbose mode, don't show passing tests
		if r.mode == RenderModeNormal {
			// Clear any retry line that might be showing
			if r.isTTY {
				return nil
			}
			return nil
		}

		// Verbose mode: show passing tests
		line := fmt.Sprintf("  %s✓%s  %s %s%s%s\n",
			c.Green, c.Reset,
			test.Name,
			c.Dim+c.Gray, timeStr, c.Reset)
		return r.write(line)
	}

	// Failed test - always show
	retryInfo := ""
	if test.RetryCount > 0 {
		retryInfo = fmt.Sprintf(" %s(failed after %d attempts, %s total)%s",
			c.Dim+c.Yellow, test.RetryCount+1, timeStr, c.Reset)
	} else {
		retryInfo = fmt.Sprintf(" %s(%s)%s",
			c.Dim+c.Gray, timeStr, c.Reset)
	}

	errorIndent := ""
	if test.ErrorMessage != "" {
		// Format error message with proper indentation
		errorLines := strings.Split(test.ErrorMessage, "\n")
		errorParts := make([]string, len(errorLines))
		for i, line := range errorLines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			errorParts[i] = fmt.Sprintf("     %s└─%s %s%s%s",
				c.Dim+c.Gray, c.Reset,
				c.Red, line, c.Reset)
		}
		errorIndent = "\n" + strings.Join(errorParts, "\n")
	}

	line := fmt.Sprintf("  %s✗%s  %s%s%s\n",
		c.Red, c.Reset,
		test.Name,
		retryInfo,
		errorIndent)

	return r.write(line)
}

// RenderSummary renders the final summary
func (r *ModernRenderer) RenderSummary(summary Summary) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c := r.colors

	// Clear any in-progress lines
	if r.isTTY && r.currentTest != "" {
		if err := r.clearLine(); err != nil {
			return err
		}
		r.currentTest = ""
	}

	var sb strings.Builder

	// Separator line
	sb.WriteString(fmt.Sprintf("\n%s%s", c.Dim+c.Gray, strings.Repeat("═", 76)))
	sb.WriteString(fmt.Sprintf("%s\n", c.Reset))

	// Summary header
	totalTimeStr := formatDuration(summary.TotalDuration)
	sb.WriteString(fmt.Sprintf("%s%sSUMMARY%s", c.Bold, c.White, c.Reset))
	sb.WriteString(fmt.Sprintf("%s%s%s\n",
		strings.Repeat(" ", 76-7-len(totalTimeStr)),
		c.Dim+c.Cyan, totalTimeStr))
	sb.WriteString(c.Reset)
	sb.WriteString("\n")

	// Test counts
	failed := len(summary.FailedTests)
	passed := len(summary.PassedTests)
	skipped := summary.SkippedTests

	if failed > 0 {
		sb.WriteString(fmt.Sprintf("  %s%d failed%s",
			c.Red+c.Bold, failed, c.Reset))
	} else {
		sb.WriteString(fmt.Sprintf("  %s%d failed%s",
			c.Dim+c.Gray, failed, c.Reset))
	}

	sb.WriteString("  |  ")

	if passed > 0 {
		sb.WriteString(fmt.Sprintf("%s%d passed%s",
			c.Green+c.Bold, passed, c.Reset))
	} else {
		sb.WriteString(fmt.Sprintf("%s%d passed%s",
			c.Dim+c.Gray, passed, c.Reset))
	}

	if skipped > 0 {
		sb.WriteString(fmt.Sprintf("  |  %s%d skipped%s",
			c.Dim+c.Yellow, skipped, c.Reset))
	}

	sb.WriteString("\n")

	// Failed tests section
	if len(summary.FailedTests) > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("%s%sFailed Tests:%s\n",
			c.Bold, c.Red, c.Reset))

		// Group by suite
		failuresBySuite := make(map[string][]TestInfo)
		for _, test := range summary.FailedTests {
			failuresBySuite[test.SuiteName] = append(
				failuresBySuite[test.SuiteName],
				test,
			)
		}

		for suiteName, tests := range failuresBySuite {
			for _, test := range tests {
				shortError := test.ErrorMessage
				if len(shortError) > 60 {
					shortError = shortError[:57] + "..."
				}

				sb.WriteString(fmt.Sprintf("  %s•%s %s%s%s %s→%s %s",
					c.Dim+c.Gray, c.Reset,
					c.White, suiteName, c.Reset,
					c.Dim+c.Gray, c.Reset,
					test.Name))

				if shortError != "" {
					sb.WriteString(fmt.Sprintf(" %s(%s)%s",
						c.Dim+c.Gray, shortError, c.Reset))
				}

				sb.WriteString("\n")
			}
		}
	}

	// Passed tests section (verbose only)
	if r.mode == RenderModeVerbose && len(summary.PassedTests) > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("%s%sPassed Tests:%s\n",
			c.Bold, c.Green, c.Reset))

		// Group by suite
		passesBySuite := make(map[string][]TestInfo)
		for _, test := range summary.PassedTests {
			passesBySuite[test.SuiteName] = append(
				passesBySuite[test.SuiteName],
				test,
			)
		}

		for suiteName, tests := range passesBySuite {
			for _, test := range tests {
				timeStr := formatDuration(test.Duration)
				sb.WriteString(fmt.Sprintf("  %s•%s %s%s%s %s→%s %s %s%s%s\n",
					c.Dim+c.Gray, c.Reset,
					c.White, suiteName, c.Reset,
					c.Dim+c.Gray, c.Reset,
					test.Name,
					c.Dim+c.Gray, timeStr, c.Reset))
			}
		}
	}

	return r.write(sb.String())
}

// Flush ensures all output is written
func (r *ModernRenderer) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isTTY && r.currentTest != "" {
		if err := r.clearLine(); err != nil {
			return err
		}
		r.currentTest = ""
	}

	if f, ok := r.writer.(interface{ Sync() error }); ok {
		return f.Sync()
	}

	return nil
}

// write writes output to the writer
func (r *ModernRenderer) write(s string) error {
	_, err := r.writer.Write([]byte(s))
	return err
}

// clearLine clears the current line in TTY mode
func (r *ModernRenderer) clearLine() error {
	if !r.isTTY {
		return nil
	}

	// Move cursor to beginning of line and clear it
	_, err := r.writer.Write([]byte("\r\033[K"))
	return err
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", d.Seconds()*1000)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}
