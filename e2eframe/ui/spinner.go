package ui

import (
	"sync"
	"time"
)

// SpinnerFrames defines the animation frames for a spinner
type SpinnerFrames []string

// Common spinner styles
var (
	// SpinnerDots uses rotating dots (most common)
	SpinnerDots = SpinnerFrames{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	// SpinnerLine uses a rotating line
	SpinnerLine = SpinnerFrames{"|", "/", "-", "\\"}

	// SpinnerArrow uses rotating arrows
	SpinnerArrow = SpinnerFrames{"←", "↖", "↑", "↗", "→", "↘", "↓", "↙"}

	// SpinnerCircle uses a growing/shrinking circle
	SpinnerCircle = SpinnerFrames{"◐", "◓", "◑", "◒"}

	// SpinnerBounce uses a bouncing dot
	SpinnerBounce = SpinnerFrames{"⠁", "⠂", "⠄", "⠂"}
)

// Spinner manages an animated spinner
type Spinner struct {
	frames   SpinnerFrames
	interval time.Duration
	index    int
	mu       sync.RWMutex
	running  bool
	stopCh   chan struct{}
	ticker   *time.Ticker
}

// NewSpinner creates a new spinner with the given frames
func NewSpinner(frames SpinnerFrames) *Spinner {
	if len(frames) == 0 {
		frames = SpinnerDots
	}

	return &Spinner{
		frames:   frames,
		interval: 80 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}
}

// NewDefaultSpinner creates a spinner with the default dot frames
func NewDefaultSpinner() *Spinner {
	return NewSpinner(SpinnerDots)
}

// SetInterval sets the animation interval
func (s *Spinner) SetInterval(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interval = interval
}

// Start starts the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}

	s.running = true
	s.ticker = time.NewTicker(s.interval)

	// Capture ticker and channels before starting goroutine to avoid race
	ticker := s.ticker
	stopCh := s.stopCh
	s.mu.Unlock()

	go s.animate(ticker, stopCh)
}

// Stop stops the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	if s.ticker != nil {
		s.ticker.Stop()
		s.ticker = nil
	}
	close(s.stopCh)
	s.stopCh = make(chan struct{})
}

// animate runs the animation loop
func (s *Spinner) animate(ticker *time.Ticker, stopCh chan struct{}) {
	if ticker == nil || stopCh == nil {
		return
	}

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.index = (s.index + 1) % len(s.frames)
			s.mu.Unlock()
		}
	}
}

// Frame returns the current spinner frame
func (s *Spinner) Frame() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.frames) == 0 {
		return ""
	}

	return s.frames[s.index]
}

// IsRunning returns whether the spinner is currently running
func (s *Spinner) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Reset resets the spinner to the first frame
func (s *Spinner) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index = 0
}
