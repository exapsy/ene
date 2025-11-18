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

	// Suite header tracking for updating with timing
	currentSuiteHeader string // Store suite header for updating
	linesAfterHeader   int    // Count lines printed after suite header
	suiteIndex         int    // Current suite index
	totalSuites        int    // Total suites

	// Spinner for progress indication
	spinner        *Spinner
	spinnerActive  bool
	spinnerTicker  *time.Ticker
	spinnerStop    chan struct{}
	spinnerStopped chan struct{}
}

// NewModernRenderer creates a new modern renderer
func NewModernRenderer(config RendererConfig) *ModernRenderer {
	// Auto-detect TTY for os.Stdout/Stderr
	isTTY := false
	if config.Writer == os.Stdout || config.Writer == os.Stderr {
		if f, ok := config.Writer.(*os.File); ok {
			isTTY = term.IsTerminal(int(f.Fd()))
		}
	} else {
		// For other writers, use the config value
		isTTY = config.IsTTY
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
		spinner: NewDefaultSpinner(),
	}
}

// GetTracker returns the progress tracker for accessing timing data
func (r *ModernRenderer) GetTracker() *ProgressTracker {
	return r.tracker
}

// RenderSuiteStart renders when a suite starts
func (r *ModernRenderer) RenderSuiteStart(suite SuiteInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop any active spinner from previous suite
	if r.spinnerActive {
		r.stopSpinnerLocked()
	}

	r.tracker.StartSuite(suite.Name)

	c := r.colors
	progress := ""
	if suite.Total > 0 {
		progress = fmt.Sprintf("[%d/%d] ", suite.Index, suite.Total)
	}

	line := fmt.Sprintf("\n%s%s%s%s%s%s%s\n",
		c.Cyan, progress, c.Reset, c.Bold, c.White, suite.Name, c.Reset)

	// Store suite info for later update
	r.currentSuiteHeader = line
	// Start at 0 - we'll count each line printed after the header
	// Note: the header itself has \n before and after, creating blank line + header line (2 lines total)
	// but we only care about counting from AFTER the header
	r.linesAfterHeader = 0
	r.suiteIndex = suite.Index
	r.totalSuites = suite.Total

	return r.write(line)
}

// RenderSuiteFinished renders when a suite finishes with timing breakdown
// Updates the suite header line with timing information
func (r *ModernRenderer) RenderSuiteFinished(suite SuiteFinishedInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop any active spinner
	if r.spinnerActive {
		r.stopSpinnerLocked()
	}

	c := r.colors

	// Calculate overhead
	overhead := suite.TotalTime - suite.SetupTime - suite.TestTime
	if overhead < 0 {
		overhead = 0
	}

	// Format timing strings
	setupStr := formatDuration(suite.SetupTime)
	testStr := formatDuration(suite.TestTime)
	overheadStr := formatDuration(overhead)

	// In TTY mode, go back and update the suite header
	if r.isTTY && r.linesAfterHeader > 0 {
		// Move cursor up to get back to the header line
		// We need to move up linesAfterHeader lines to get to just after the header,
		// then one more line to reach the header itself
		r.write(fmt.Sprintf("\033[%dA", r.linesAfterHeader))
		r.write("\033[1A")

		// Move to beginning of line and clear it
		r.write("\r\033[2K")

		// Rewrite header with timing appended
		progress := ""
		if r.totalSuites > 0 {
			progress = fmt.Sprintf("[%d/%d] ", r.suiteIndex, r.totalSuites)
		}

		timingInfo := fmt.Sprintf(" %s(Setup: %s | Tests: %s | Overhead: %s)%s",
			c.Dim+c.Gray, setupStr, testStr, overheadStr, c.Reset)

		updatedLine := fmt.Sprintf("%s%s%s%s%s%s%s%s",
			c.Cyan, progress, c.Reset, c.Bold, c.White, suite.Name, c.Reset, timingInfo)

		r.write(updatedLine)

		// Move cursor back down past the header and all content
		r.write(fmt.Sprintf("\033[%dB", r.linesAfterHeader+1))
		r.write("\r")

		return nil
	}

	// Non-TTY mode: just print timing on a separate line
	line := fmt.Sprintf("  %s(Setup: %s | Tests: %s | Overhead: %s)%s\n",
		c.Dim+c.Gray, setupStr, testStr, overheadStr, c.Reset)

	return r.write(line)
}

// RenderContainerStarting renders when a container is starting
func (r *ModernRenderer) RenderContainerStarting(container ContainerInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Track start time
	r.tracker.StartContainer(container)

	// Count lines after suite header
	if r.mode == RenderModeVerbose {
		r.linesAfterHeader++
	}

	// Only show in verbose mode or with spinner in normal mode
	if r.mode == RenderModeCI {
		return nil
	}

	c := r.colors

	if r.mode == RenderModeVerbose {
		// Verbose mode: show static line
		line := fmt.Sprintf("  %s⚙  %s container starting...%s\n",
			c.Dim+c.Yellow, container.Name, c.Reset)
		return r.write(line)
	}

	// Normal mode with TTY: start spinner (spinner replaces the ⚙ symbol)
	if r.isTTY {
		r.startSpinner(fmt.Sprintf("  %s%%s  %s container starting...%s",
			c.Dim+c.Yellow, container.Name, c.Reset))
	}

	return nil
}

// RenderContainerReady renders when a container is ready
func (r *ModernRenderer) RenderContainerReady(container ContainerInfo) error {
	// Show in normal and verbose modes
	if r.mode == RenderModeCI {
		return r.renderContainerReadyCI(container)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Stop spinner if active
	if r.spinnerActive {
		r.stopSpinnerLocked()
	}

	// Count lines after suite header
	r.linesAfterHeader++

	// Let tracker calculate duration from start time
	r.tracker.ReadyContainer(container)

	// Get the container info with calculated duration from tracker
	c := r.colors
	var duration time.Duration
	if ready := r.tracker.GetContainerReady(container.Name); ready != nil {
		duration = ready.Duration
	}

	timeStr := formatDuration(duration)
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

	// Let tracker calculate duration from start time
	r.tracker.ReadyContainer(container)

	// Get the container info with calculated duration from tracker
	var duration time.Duration
	if ready := r.tracker.GetContainerReady(container.Name); ready != nil {
		duration = ready.Duration
	}

	timeStr := formatDuration(duration)
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
		// TTY normal mode: start spinner (spinner replaces the ⋯ symbol)
		r.startSpinner(fmt.Sprintf("  %s%%s  %s...%s",
			c.Dim+c.Gray, test.Name, c.Reset))
		return nil
	} else if r.mode == RenderModeVerbose {
		// Verbose mode: show with newline
		line = fmt.Sprintf("  ▶  %s...\n", test.Name)
		r.linesAfterHeader++
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

	// Track the retry count for this test
	r.tracker.TrackTestRetry(test.SuiteName, test.Name, attempt)

	// In verbose mode, show each retry on its own line
	if r.mode == RenderModeVerbose {
		c := r.colors
		line := fmt.Sprintf("  %s⟳%s  %s (retry %d/%d)...\n",
			c.Dim+c.Yellow, c.Reset, test.Name, attempt, maxAttempts)
		r.linesAfterHeader++
		return r.write(line)
	}

	// In normal mode with TTY, update spinner with retry info
	if r.isTTY && r.mode == RenderModeNormal {
		c := r.colors
		// Stop current spinner and start new one with retry info
		if r.spinnerActive {
			r.stopSpinnerLocked()
		}
		// Spinner replaces the ⟳ symbol
		r.startSpinner(fmt.Sprintf("  %s%%s  %s (retry %d/%d)...%s",
			c.Dim+c.Yellow, test.Name, attempt, maxAttempts, c.Reset))
		return nil
	}

	// In non-TTY normal mode, don't show retries (we'll show final result with retry count)
	return nil
}

// RenderTestCompleted renders when a test completes
func (r *ModernRenderer) RenderTestCompleted(test TestInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get retry count from tracker before completing
	test.RetryCount = r.tracker.GetTestRetryCount(test.SuiteName, test.Name)

	r.tracker.CompleteTest(test)

	// Stop spinner if active
	if r.spinnerActive {
		r.stopSpinnerLocked()
	}

	// Line counting is done inside the if/else branches below

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
		line := fmt.Sprintf("  %s✓%s  %s%s%s %s%s%s\n",
			c.Green, c.Reset,
			c.White, test.Name, c.Reset,
			c.Dim+c.Gray, timeStr, c.Reset)
		r.linesAfterHeader++
		return r.write(line)
	}

	// Failed test - always show
	retryInfo := ""
	if test.RetryCount > 0 {
		retryInfo = fmt.Sprintf(" %s(failed after %d retries)%s",
			c.Dim+c.Yellow, test.RetryCount, c.Reset)
	} else {
		retryInfo = fmt.Sprintf(" %s(%s)%s",
			c.Dim+c.Gray, timeStr, c.Reset)
	}

	errorIndent := ""
	errorLineCount := 0
	if test.ErrorMessage != "" {
		// Format error message with proper indentation
		errorLines := strings.Split(test.ErrorMessage, "\n")
		errorParts := make([]string, len(errorLines))
		for i, line := range errorLines {
			if strings.TrimSpace(line) == "" {
				continue
			}

			// Default to red text
			formattedLine := c.Red + line + c.Reset

			errorParts[i] = fmt.Sprintf("     %s└─%s %s",
				c.Dim+c.Gray, c.Reset,
				formattedLine)
			errorLineCount++
		}
		errorIndent = "\n" + strings.Join(errorParts, "\n")
	}

	line := fmt.Sprintf("  %s✗%s  %s%s%s%s%s\n",
		c.Red, c.Reset,
		c.White, test.Name, c.Reset,
		retryInfo,
		errorIndent)

	// Count lines: 1 for test line + error lines
	r.linesAfterHeader += 1 + errorLineCount

	return r.write(line)
}

// RenderTransition renders an ephemeral transition state with a spinner
func (r *ModernRenderer) RenderTransition(message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c := r.colors

	// In verbose mode or CI mode, show as static text line
	if r.mode == RenderModeVerbose || r.mode == RenderModeCI {
		line := fmt.Sprintf("  %s⋯%s  %s\n",
			c.Dim+c.Gray, c.Reset, message)
		if r.mode == RenderModeVerbose {
			r.linesAfterHeader++
		}
		return r.write(line)
	}

	// Normal mode with TTY: show with animated spinner
	if r.isTTY {
		// Stop any existing spinner first
		if r.spinnerActive {
			r.stopSpinnerLocked()
		}

		// Start spinner with transition message (dimmed since it's ephemeral)
		r.startSpinner(fmt.Sprintf("  %s%%s  %s%s",
			c.Dim+c.Gray, message, c.Reset))

		return nil
	}

	// Normal mode without TTY: show as visible persistent text
	// Use lighter gray text to indicate it's informational but keep it readable
	line := fmt.Sprintf("  %s⋯  %s%s\n",
		c.Gray, message, c.Reset)
	r.linesAfterHeader++
	return r.write(line)
}

// RenderSummary renders the final summary
func (r *ModernRenderer) RenderSummary(summary Summary) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	c := r.colors

	// Stop any active spinner first
	if r.spinnerActive {
		r.stopSpinnerLocked()
	}

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

	// Summary header with time breakdown
	totalTimeStr := formatDuration(summary.TotalDuration)
	sb.WriteString(fmt.Sprintf("%s%sSUMMARY%s", c.Bold, c.White, c.Reset))
	sb.WriteString(fmt.Sprintf("%s%s%s\n",
		strings.Repeat(" ", 76-7-len(totalTimeStr)),
		c.Dim+c.Cyan, totalTimeStr))
	sb.WriteString(c.Reset)

	// Show time breakdown (in both verbose and non-verbose modes)
	if summary.ContainerTime > 0 || summary.TestExecutionTime > 0 {
		containerTimeStr := formatDuration(summary.ContainerTime)
		testTimeStr := formatDuration(summary.TestExecutionTime)
		overhead := summary.TotalDuration - summary.ContainerTime - summary.TestExecutionTime
		overheadStr := formatDuration(overhead)

		// Different formatting for verbose vs non-verbose
		if r.mode == RenderModeVerbose {
			sb.WriteString(fmt.Sprintf("%s  Setup: %s  |  Tests: %s  |  Overhead: %s%s\n",
				c.Dim+c.Gray, containerTimeStr, testTimeStr, overheadStr, c.Reset))
			// Explain what overhead includes in verbose mode
			sb.WriteString(fmt.Sprintf("%s  (Overhead: Docker networks, framework initialization, cleanup)%s\n",
				c.Dim+c.Gray, c.Reset))
		} else {
			// Non-verbose: more compact, on same line or separate
			sb.WriteString(fmt.Sprintf("%s  Setup: %s  |  Tests: %s  |  Overhead: %s%s\n",
				c.Dim+c.Gray, containerTimeStr, testTimeStr, overheadStr, c.Reset))
		}
	}
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

	// Stop spinner if active
	if r.spinnerActive {
		r.stopSpinnerLocked()
	}

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

// startSpinner starts the spinner with the given format string (must contain %s for spinner frame)
func (r *ModernRenderer) startSpinner(format string) {
	if !r.isTTY {
		return
	}

	// Stop any existing spinner
	if r.spinnerActive {
		r.stopSpinnerLocked()
	}

	r.spinnerActive = true
	r.spinner.Reset()
	r.spinner.Start()
	r.spinnerTicker = time.NewTicker(80 * time.Millisecond)
	r.spinnerStop = make(chan struct{})
	r.spinnerStopped = make(chan struct{})

	// Capture channels before starting goroutine to avoid race
	stopCh := r.spinnerStop
	stoppedCh := r.spinnerStopped
	ticker := r.spinnerTicker

	go r.animateSpinner(format, ticker, stopCh, stoppedCh)
}

// stopSpinnerLocked stops the spinner (caller must hold lock)
func (r *ModernRenderer) stopSpinnerLocked() {
	if !r.spinnerActive {
		return
	}

	r.spinnerActive = false
	r.spinner.Stop()
	if r.spinnerTicker != nil {
		r.spinnerTicker.Stop()
		r.spinnerTicker = nil
	}

	// Signal the goroutine to stop
	if r.spinnerStop != nil {
		close(r.spinnerStop)
		r.spinnerStop = nil
		// Wait for goroutine to finish (with timeout)
		select {
		case <-r.spinnerStopped:
		case <-time.After(200 * time.Millisecond):
		}
	}

	// Clear the spinner line
	r.clearLine()
}

// animateSpinner runs the spinner animation loop
func (r *ModernRenderer) animateSpinner(format string, ticker *time.Ticker, stopCh, stoppedCh chan struct{}) {
	defer func() {
		if stoppedCh != nil {
			close(stoppedCh)
		}
	}()

	if ticker == nil || stopCh == nil {
		return
	}

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			r.mu.Lock()
			if !r.spinnerActive {
				r.mu.Unlock()
				return
			}

			// Get current frame and format the line
			frame := r.spinner.Frame()
			line := fmt.Sprintf(format, frame)

			// Clear and rewrite line
			r.clearLine()
			r.write(line)
			r.lastLineLength = len(line)
			r.mu.Unlock()
		}
	}
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
