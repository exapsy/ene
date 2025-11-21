package e2eframe

import (
	"sync"
)

// EventSinkWithFlush extends the basic event sink concept with the ability to wait
// for all pending events to be processed. This is useful when you need to guarantee
// that all events have been fully handled before proceeding (e.g., before sending a
// suite finished event that updates display based on previous events).
type EventSinkWithFlush interface {
	// Flush blocks until all events that were in the channel have been processed.
	// Safe to call multiple times and from multiple goroutines.
	Flush()
}

// FlushableEventSink wraps a regular event channel and adds flush capability
// using a simple token-based approach: Flush() sends a special token event through
// the channel and waits for it to be processed, guaranteeing all previous events
// have been handled.
//
// Usage:
//
//	sink, eventCh := NewFlushableEventSink(100)
//	go func() {
//	    for event := range eventCh {
//	        if sink.IsFlushToken(event) {
//	            sink.MarkFlushComplete(event)
//	            continue
//	        }
//	        processEvent(event)
//	    }
//	}()
//
//	// Send events normally through the channel
//	eventCh <- event1
//	eventCh <- event2
//	sink.Flush() // Waits until event1 and event2 are processed
type FlushableEventSink struct {
	ch         chan Event
	flushChans map[uint64]chan struct{}
	nextID     uint64
	mu         sync.Mutex
}

// flushToken is a special event used internally for flush synchronization
type flushToken struct {
	BaseEvent
	id uint64
}

// NewFlushableEventSink creates a new flushable event sink with the specified buffer size.
// Returns both the sink (for flush capability) and the channel for sending/receiving events.
func NewFlushableEventSink(bufferSize int) (*FlushableEventSink, chan Event) {
	ch := make(chan Event, bufferSize)
	sink := &FlushableEventSink{
		ch:         ch,
		flushChans: make(map[uint64]chan struct{}),
	}
	return sink, ch
}

// Flush blocks until all events that were in the channel when Flush was called
// have been processed. It works by sending a special flush token through the channel
// and waiting for the event loop to signal that it received and handled the token.
func (s *FlushableEventSink) Flush() {
	s.mu.Lock()
	// Generate unique ID for this flush
	id := s.nextID
	s.nextID++

	// Create completion channel
	done := make(chan struct{})
	s.flushChans[id] = done
	s.mu.Unlock()

	// Send flush token through the channel
	// This ensures all previous events are processed first (FIFO)
	s.ch <- &flushToken{
		BaseEvent: BaseEvent{
			EventType:    EventType("_flush_token"),
			EventMessage: "internal flush synchronization",
		},
		id: id,
	}

	// Wait for flush to complete
	<-done
}

// IsFlushToken checks if an event is a flush token.
// Event processing loops should check this and call MarkFlushComplete instead of processing.
func (s *FlushableEventSink) IsFlushToken(event Event) bool {
	_, ok := event.(*flushToken)
	return ok
}

// MarkFlushComplete should be called by the event processing loop when it receives
// a flush token. This signals to the waiting Flush() call that all previous events
// have been processed.
//
// Example event loop:
//
//	for event := range eventCh {
//	    if sink.IsFlushToken(event) {
//	        sink.MarkFlushComplete(event)
//	        continue
//	    }
//	    handleEvent(event)
//	}
func (s *FlushableEventSink) MarkFlushComplete(event Event) {
	token, ok := event.(*flushToken)
	if !ok {
		return
	}

	s.mu.Lock()
	if done, exists := s.flushChans[token.id]; exists {
		close(done)
		delete(s.flushChans, token.id)
	}
	s.mu.Unlock()
}

// Close closes the underlying channel.
// No more events can be sent after calling Close.
func (s *FlushableEventSink) Close() {
	close(s.ch)
}
