package ui

import (
	"sync"
	"time"
)

// ProgressTracker tracks the progress of test execution
type ProgressTracker struct {
	mu sync.RWMutex

	// Suite tracking
	currentSuite    string
	suiteIndex      int
	totalSuites     int
	suitesCompleted map[string]bool

	// Test tracking
	testsRunning   map[string]*TestInfo
	testsCompleted []TestInfo
	testsFailed    []TestInfo
	testsPassed    []TestInfo
	testRetryCount map[string]int // Track retry attempts per test

	// Container tracking
	containersStarting map[string]*ContainerInfo
	containersReady    map[string]*ContainerInfo
	containerStartTime map[string]time.Time

	// Timing
	startTime time.Time
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		suitesCompleted:    make(map[string]bool),
		testsRunning:       make(map[string]*TestInfo),
		testsCompleted:     make([]TestInfo, 0),
		testsFailed:        make([]TestInfo, 0),
		testsPassed:        make([]TestInfo, 0),
		testRetryCount:     make(map[string]int),
		containersStarting: make(map[string]*ContainerInfo),
		containersReady:    make(map[string]*ContainerInfo),
		containerStartTime: make(map[string]time.Time),
		startTime:          time.Now(),
	}
}

// SetTotalSuites sets the total number of suites to run
func (p *ProgressTracker) SetTotalSuites(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalSuites = total
}

// StartSuite marks a suite as started
func (p *ProgressTracker) StartSuite(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentSuite = name
	p.suiteIndex++
}

// CompleteSuite marks a suite as completed
func (p *ProgressTracker) CompleteSuite(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.suitesCompleted[name] = true
}

// GetCurrentSuite returns the current suite info
func (p *ProgressTracker) GetCurrentSuite() (name string, index, total int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentSuite, p.suiteIndex, p.totalSuites
}

// StartContainer marks a container as starting
func (p *ProgressTracker) StartContainer(info ContainerInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.containersStarting[info.Name] = &info
	p.containerStartTime[info.Name] = time.Now()
}

// ReadyContainer marks a container as ready and calculates duration
func (p *ProgressTracker) ReadyContainer(info ContainerInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Calculate actual duration from start time if available
	if startTime, exists := p.containerStartTime[info.Name]; exists {
		info.Duration = time.Since(startTime)
		delete(p.containerStartTime, info.Name)
	}

	delete(p.containersStarting, info.Name)
	p.containersReady[info.Name] = &info
}

// GetContainerReady returns the container info with calculated duration
func (p *ProgressTracker) GetContainerReady(name string) *ContainerInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.containersReady[name]
}

// TrackTestRetry records a retry attempt for a test
func (p *ProgressTracker) TrackTestRetry(suiteName, testName string, retryCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := suiteName + "::" + testName
	p.testRetryCount[key] = retryCount
}

// GetTestRetryCount returns the retry count for a test
func (p *ProgressTracker) GetTestRetryCount(suiteName, testName string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := suiteName + "::" + testName
	return p.testRetryCount[key]
}

// GetTotalContainerTime returns the total time spent starting containers
func (p *ProgressTracker) GetTotalContainerTime() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var total time.Duration
	for _, container := range p.containersReady {
		if container != nil {
			total += container.Duration
		}
	}
	return total
}

// StartTest marks a test as running
func (p *ProgressTracker) StartTest(info TestInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := info.SuiteName + "::" + info.Name
	p.testsRunning[key] = &info
}

// CompleteTest marks a test as completed
func (p *ProgressTracker) CompleteTest(info TestInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := info.SuiteName + "::" + info.Name
	delete(p.testsRunning, key)

	// Set retry count from tracked value
	info.RetryCount = p.testRetryCount[key]

	p.testsCompleted = append(p.testsCompleted, info)

	if info.Passed {
		p.testsPassed = append(p.testsPassed, info)
	} else {
		p.testsFailed = append(p.testsFailed, info)
	}

	// Clean up retry count
	delete(p.testRetryCount, key)
}

// GetRunningTests returns all currently running tests
func (p *ProgressTracker) GetRunningTests() []TestInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tests := make([]TestInfo, 0, len(p.testsRunning))
	for _, test := range p.testsRunning {
		tests = append(tests, *test)
	}
	return tests
}

// GetStats returns current test statistics
func (p *ProgressTracker) GetStats() (total, passed, failed int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.testsCompleted), len(p.testsPassed), len(p.testsFailed)
}

// GetFailedTests returns all failed tests
func (p *ProgressTracker) GetFailedTests() []TestInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tests := make([]TestInfo, len(p.testsFailed))
	copy(tests, p.testsFailed)
	return tests
}

// GetPassedTests returns all passed tests
func (p *ProgressTracker) GetPassedTests() []TestInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	tests := make([]TestInfo, len(p.testsPassed))
	copy(tests, p.testsPassed)
	return tests
}

// GetElapsedTime returns the elapsed time since start
func (p *ProgressTracker) GetElapsedTime() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Since(p.startTime)
}

// GetSummary returns a summary of the test run
func (p *ProgressTracker) GetSummary(skippedCount int) Summary {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Calculate total test execution time
	var testTime time.Duration
	for _, test := range p.testsCompleted {
		testTime += test.Duration
	}

	return Summary{
		TotalDuration:     time.Since(p.startTime),
		TotalTests:        len(p.testsCompleted),
		PassedTests:       p.testsPassed,
		FailedTests:       p.testsFailed,
		SkippedTests:      skippedCount,
		ContainerTime:     p.GetTotalContainerTime(),
		TestExecutionTime: testTime,
	}
}
