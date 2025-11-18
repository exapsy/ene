package e2eframe

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Spinner provides a simple loading animation that's CI/CD friendly
type Spinner struct {
	message    string
	frames     []string
	interval   time.Duration
	writer     io.Writer
	mu         sync.Mutex
	active     bool
	done       chan bool
	isTTY      bool
	lastLen    int
	startTime  time.Time
	showTimer  bool
	frameIndex int
}

// SpinnerStyle defines different spinner animation styles
type SpinnerStyle int

const (
	// SpinnerDots is a simple dot animation: . .. ...
	SpinnerDots SpinnerStyle = iota
	// SpinnerCircle is a rotating circle: ◐ ◓ ◑ ◒
	SpinnerCircle
	// SpinnerArrow is a rotating arrow: ← ↖ ↑ ↗ → ↘ ↓ ↙
	SpinnerArrow
	// SpinnerBar is a moving bar: ▏▎▍▌▋▊▉█
	SpinnerBar
	// SpinnerPulse is a pulsing dot: ⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷
	SpinnerPulse
	// SpinnerSimple is CI-friendly: [.  ] [.. ] [...]
	SpinnerSimple
)

var spinnerFrames = map[SpinnerStyle][]string{
	SpinnerDots:   {".", "..", "..."},
	SpinnerCircle: {"◐", "◓", "◑", "◒"},
	SpinnerArrow:  {"←", "↖", "↑", "↗", "→", "↘", "↓", "↙"},
	SpinnerBar:    {"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█", "▉", "▊", "▋", "▌", "▍", "▎"},
	SpinnerPulse:  {"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
	SpinnerSimple: {"[.  ]", "[.. ]", "[...]"},
}

// SpinnerConfig configures spinner behavior
type SpinnerConfig struct {
	// Message to display next to the spinner
	Message string
	// Style of the spinner animation
	Style SpinnerStyle
	// Writer where output is written (defaults to os.Stdout)
	Writer io.Writer
	// ShowTimer displays elapsed time
	ShowTimer bool
	// ForceTTY forces TTY mode even if not detected
	ForceTTY bool
	// ForceSimple forces simple mode (no animations)
	ForceSimple bool
}

// NewSpinner creates a new spinner with the given configuration
func NewSpinner(config SpinnerConfig) *Spinner {
	if config.Writer == nil {
		config.Writer = os.Stdout
	}

	style := config.Style
	isTTY := isTTY(config.Writer) || config.ForceTTY

	// Use simple style for non-TTY environments (CI/CD)
	if !isTTY || config.ForceSimple {
		style = SpinnerSimple
	}

	frames := spinnerFrames[style]
	interval := 100 * time.Millisecond

	// Adjust interval based on style
	switch style {
	case SpinnerDots:
		interval = 500 * time.Millisecond
	case SpinnerSimple:
		interval = 500 * time.Millisecond
	case SpinnerPulse:
		interval = 80 * time.Millisecond
	}

	return &Spinner{
		message:   config.Message,
		frames:    frames,
		interval:  interval,
		writer:    config.Writer,
		done:      make(chan bool),
		isTTY:     isTTY,
		showTimer: config.ShowTimer,
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.startTime = time.Now()
	s.frameIndex = 0
	s.mu.Unlock()

	go s.run()
}

// Stop stops the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()

	s.done <- true
	s.clearLine()
}

// UpdateMessage updates the spinner message while it's running
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
}

// Success stops the spinner and shows a success message
func (s *Spinner) Success(message string) {
	s.Stop()
	elapsed := s.getElapsed()
	if message != "" {
		if s.showTimer {
			fmt.Fprintf(s.writer, "✓ %s (%s)\n", message, elapsed)
		} else {
			fmt.Fprintf(s.writer, "✓ %s\n", message)
		}
	}
}

// Failure stops the spinner and shows a failure message
func (s *Spinner) Failure(message string) {
	s.Stop()
	elapsed := s.getElapsed()
	if message != "" {
		if s.showTimer {
			fmt.Fprintf(s.writer, "✗ %s (%s)\n", message, elapsed)
		} else {
			fmt.Fprintf(s.writer, "✗ %s\n", message)
		}
	}
}

// Warning stops the spinner and shows a warning message
func (s *Spinner) Warning(message string) {
	s.Stop()
	if message != "" {
		fmt.Fprintf(s.writer, "⚠ %s\n", message)
	}
}

// Info stops the spinner and shows an info message
func (s *Spinner) Info(message string) {
	s.Stop()
	if message != "" {
		fmt.Fprintf(s.writer, "ℹ %s\n", message)
	}
}

func (s *Spinner) run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.render()
		}
	}
}

func (s *Spinner) render() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return
	}

	// Get current frame
	frame := s.frames[s.frameIndex]
	s.frameIndex = (s.frameIndex + 1) % len(s.frames)

	// Build output
	var output string
	if s.showTimer {
		elapsed := s.getElapsed()
		output = fmt.Sprintf("%s %s (%s)", frame, s.message, elapsed)
	} else {
		output = fmt.Sprintf("%s %s", frame, s.message)
	}

	// Clear previous line if TTY
	if s.isTTY {
		s.clearLineNoLock()
		fmt.Fprint(s.writer, output)
	} else {
		// For non-TTY (CI/CD), print dots on same line
		if s.frameIndex == 0 {
			fmt.Fprintf(s.writer, "\n%s", output)
		} else {
			fmt.Fprint(s.writer, ".")
		}
	}

	s.lastLen = len(output)
}

func (s *Spinner) clearLine() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearLineNoLock()
}

func (s *Spinner) clearLineNoLock() {
	if s.isTTY && s.lastLen > 0 {
		// Move cursor to beginning of line and clear
		fmt.Fprint(s.writer, "\r")
		for i := 0; i < s.lastLen; i++ {
			fmt.Fprint(s.writer, " ")
		}
		fmt.Fprint(s.writer, "\r")
		s.lastLen = 0
	}
}

func (s *Spinner) getElapsed() string {
	elapsed := time.Since(s.startTime)
	if elapsed < time.Second {
		return fmt.Sprintf("%dms", elapsed.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", elapsed.Seconds())
}

// isTTY checks if the writer is a terminal
func isTTY(w io.Writer) bool {
	// Check for common CI/CD environment variables
	if os.Getenv("CI") != "" || os.Getenv("CONTINUOUS_INTEGRATION") != "" {
		return false
	}
	if os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("GITLAB_CI") != "" {
		return false
	}
	if os.Getenv("JENKINS_HOME") != "" || os.Getenv("BUILDKITE") != "" {
		return false
	}

	// Check if it's a file
	if f, ok := w.(*os.File); ok {
		stat, err := f.Stat()
		if err != nil {
			return false
		}
		// Check if it's a character device (terminal)
		return (stat.Mode() & os.ModeCharDevice) != 0
	}

	return false
}
