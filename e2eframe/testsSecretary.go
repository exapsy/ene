package e2eframe

import (
	"fmt"
	"time"
)

// TestsSecretary is keeping track of running tests and their statuses.
// It can be utilized to generate reports or summaries at the end of the test run.
type TestsSecretary struct {
	// completedTests holds all test events that were completed during the run.
	completedTests []TestEvent
	// skippedSuites holds all tests that were skipped during the run.
	skippedSuites []SuiteSkippedEvent

	// Metadata about running tests
	totalFailedTests  int
	totalSkippedTests int
	totalPassedTests  int
	startTime         time.Time
}

func NewTestsSecretary(eventChan <-chan Event) *TestsSecretary {
	return &TestsSecretary{
		skippedSuites:     make([]SuiteSkippedEvent, 0),
		completedTests:    make([]TestEvent, 0),
		totalFailedTests:  0,
		totalSkippedTests: 0,
		totalPassedTests:  0,
		startTime:         time.Time{},
	}
}

func (s *TestsSecretary) ConsumeEvent(event Event) error {
	switch event.Type() {
	case EventSuiteStarted:
		if suiteEvent, ok := event.(*BaseEvent); ok {
			if s.startTime.IsZero() {
				s.startTime = suiteEvent.EventTime
			}
		} else {
			return fmt.Errorf("expected BaseEvent, got %T", event)
		}
	case EventTestStarted:
		// no action needed for suite start
	case EventTestCompleted:
		if testEvent, ok := event.(*TestEvent); ok {
			if !testEvent.Passed {
				s.totalFailedTests++
			} else {
				s.totalPassedTests++
			}

			s.completedTests = append(s.completedTests, *testEvent)
		} else {
			return fmt.Errorf("expected TestEvent, got %T", event)
		}
	case EventSuiteSkipped:
		if suiteEvent, ok := event.(*SuiteSkippedEvent); ok {
			s.skippedSuites = append(s.skippedSuites, *suiteEvent)
			s.totalSkippedTests += suiteEvent.TotalSuiteTests
		} else {
			return fmt.Errorf("expected SuiteSkippedEvent, got %T", event)
		}
	case EventSuiteError:
		if _, ok := event.(*BaseEvent); ok {
			s.totalFailedTests++
		} else {
			return fmt.Errorf("expected BaseEvent, got %T", event)
		}
	}

	return nil
}

func (s *TestsSecretary) SkippedTests() []SuiteSkippedEvent {
	return s.skippedSuites
}

func (s *TestsSecretary) TotalFailedTests() int {
	return s.totalFailedTests
}

func (s *TestsSecretary) TotalSkippedTests() int {
	return s.totalSkippedTests
}

func (s *TestsSecretary) TotalPassedTests() int {
	return s.totalPassedTests
}

func (s *TestsSecretary) CompletedTests() []TestEvent {
	return s.completedTests
}

func (s *TestsSecretary) PassedTests() []TestEvent {
	passedTests := make([]TestEvent, 0, len(s.completedTests))

	for _, test := range s.completedTests {
		if test.Passed {
			passedTests = append(passedTests, test)
		}
	}

	return passedTests
}

func (s *TestsSecretary) FailedTests() []TestEvent {
	failedTests := make([]TestEvent, 0, len(s.completedTests))

	for _, test := range s.completedTests {
		if !test.Passed {
			failedTests = append(failedTests, test)
		}
	}

	return failedTests
}

func (s *TestsSecretary) StartTime() time.Time {
	return s.startTime
}
