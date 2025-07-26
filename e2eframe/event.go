package e2eframe

import "time"

// EventType defines the kind of event being reported.
type EventType string

const (
	// Suite lifecycle events.
	EventSuiteStarted   EventType = "test_suite_started"
	EventSuiteCompleted EventType = "test_suite_completed"
	EventSuiteError     EventType = "test_suite_error"
	EventSuiteSkipped   EventType = "test_suite_skipped"

	// Test lifecycle events.
	EventTestStarted   EventType = "test_started"
	EventTestCompleted EventType = "test_completed"
	EventTestSkipped   EventType = "test_skipped"
	EventTestRetrying  EventType = "test_retrying"

	// Container/unit lifecycle events.
	EventContainerPulling  EventType = "container_pulling"
	EventContainerStarting EventType = "container_starting"
	EventContainerStarted  EventType = "container_started"
	EventContainerHealthy  EventType = "container_healthy"
	EventContainerFailed   EventType = "container_failed"
	EventContainerStopped  EventType = "container_stopped"

	// Mock http server events.
	EventMockServerBuilding EventType = "mock_server_building"
	EventMockServerStarted  EventType = "mock_server_started"
	EventMockServerStopped  EventType = "mock_server_stopped"
	EventMockServerError    EventType = "mock_server_error"

	// Network events.
	EventNetworkCreated   EventType = "network_created"
	EventNetworkDestroyed EventType = "network_destroyed"

	// Script execution events.
	EventScriptExecuting EventType = "script_executing"
	EventScriptCompleted EventType = "script_completed"
	EventScriptFailed    EventType = "script_failed"

	// General events.
	EventInfo    EventType = "info"
	EventWarning EventType = "warning"
	EventError   EventType = "error"
)

// Event represents any occurrence during test execution.
type Event interface {
	// Type returns the event type
	Type() EventType
	// Timestamp returns when the event occurred
	Timestamp() time.Time
	// SuiteName returns the associated test suite
	SuiteName() string
	// Message returns the human-readable event description
	Message() string
}

// BaseEvent provides common event functionality.
type BaseEvent struct {
	EventType    EventType
	EventTime    time.Time
	Suite        string
	EventMessage string
}

func (e BaseEvent) Type() EventType      { return e.EventType }
func (e BaseEvent) Timestamp() time.Time { return e.EventTime }
func (e BaseEvent) SuiteName() string    { return e.Suite }
func (e BaseEvent) Message() string      { return e.EventMessage }

// TestEvent represents test-specific events.
type TestEvent struct {
	BaseEvent
	TestName string
	Passed   bool
	Error    error
	Duration time.Duration
}

func (te *TestEvent) Unwrap() error {
	return te.Error
}

type TestRetryingEvent struct {
	BaseEvent
	TestName   string
	RetryCount int
	MaxRetries int
}

// UnitEvent represents container/unit lifecycle events.
type UnitEvent struct {
	BaseEvent
	UnitName string
	UnitKind UnitKind
	Endpoint string
	Error    error
}

type EventSink chan<- Event

// NewEvents creates a new event sink channel.
func NewEventChannel() chan Event {
	return make(chan Event, 100) // Buffered channel to avoid blocking
}

type SuiteSkippedEvent struct {
	BaseEvent
	TotalSuiteTests int
}

// SuiteErrorEvent represents suite error events with preserved original error
type SuiteErrorEvent struct {
	BaseEvent
	Error error
}

func (e SuiteErrorEvent) Unwrap() error {
	return e.Error
}
