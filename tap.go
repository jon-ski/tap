// Package tap provides a lightweight event system.
// It offers a simple interface for emitting structured events with metadata,
// supporting both synchronous sinks and asynchronous event buses.
//
// The package is designed for event-driven architectures, monitoring,
// and applications that need to emit events with different severity levels.
package tap

import (
	"sync"
	"time"
)

// EventLevel represents the severity level of an event.
// Lower values indicate more verbose/debug information,
// while higher values indicate more critical events.
type EventLevel int

const (
	// LevelDebug represents debug-level events with the lowest priority.
	// These are typically verbose logging events used for development.
	LevelDebug EventLevel = -4

	// LevelInfo represents informational events with normal priority.
	// These are general information events about normal operation.
	LevelInfo EventLevel = 0

	// LevelWarn represents warning events with elevated priority.
	// These indicate potential issues or unusual conditions.
	LevelWarn EventLevel = 4

	// LevelError represents error events with the highest priority.
	// These indicate actual errors or failures that occurred.
	LevelError EventLevel = 8
)

// Event represents a single event in the logging/tracing system.
// It contains all the information needed to describe what happened,
// when it happened, and any relevant metadata.
type Event struct {
	// Time is the timestamp when the event occurred.
	// If zero, it will be automatically set to the current time when emitted.
	Time time.Time

	// Level indicates the severity level of the event (debug, info, warn, error).
	Level EventLevel

	// ID is a generic identifier used for correlation between related events.
	// This can be a request ID, transaction ID, or any other tracking identifier.
	ID string

	// Op describes the operation or name of the event.
	// This should be a short, descriptive string like "connect", "read", "write".
	Op string

	// Msg is a short human-readable message describing the event.
	// Typically contains status information like "success", "failed", "timeout".
	Msg string

	// Err contains the error if this is an error event, nil otherwise.
	Err error

	// Meta contains additional metadata as key-value pairs.
	Meta map[string]any
}

// ---- Sink

// EventSink is a function type that handles events.
// It receives an Event and processes it (e.g., logging, forwarding, storing).
// EventSink functions should be safe to call concurrently.
type EventSink func(Event)

// Multi creates a fan-out EventSink that forwards events to multiple sinks.
// It returns a single EventSink that distributes each event to all provided sinks.
//
// Nil sinks are filtered out and ignored.
// If no valid sinks are provided, Multi returns nil.
//
// The returned sink is safe for concurrent use and protects against panics
// from individual sinks - if one sink panics, others continue to receive events.
func Multi(sinks ...EventSink) EventSink {
	out := make([]EventSink, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return func(e Event) {
		for _, s := range out {
			// shield from panics
			func() { defer func() { _ = recover() }(); s(e) }()
		}
	}
}

// ---- Bus (non-blocking, bounded)

// Bus provides an asynchronous, non-blocking event bus.
// Events are queued in a bounded channel and processed by a background goroutine.
// If the queue is full, new events are dropped to prevent blocking.
type Bus struct {
	// sink is the EventSink that receives processed events
	sink EventSink

	// ch is the channel used for queuing events
	ch chan Event

	// once ensures the background goroutine is started only once
	once sync.Once

	// bufSize is the size of the event buffer
	bufSize int

	// closed signals when the bus has been closed
	closed chan struct{}
}

// NewBus creates a new asynchronous event bus.
//
// The sink parameter specifies where events will be forwarded.
// If sink is nil, events will be ignored.
//
// The bufSize parameter sets the size of the event buffer.
// If bufSize is less than or equal to 0, a default size of 256 is used.
//
// The returned Bus starts a background goroutine immediately to process events.
// Call Close() when done to clean up resources.
func NewBus(sink EventSink, bufSize int) *Bus {
	if bufSize <= 0 {
		bufSize = 256
	}
	b := &Bus{
		sink:    sink,
		bufSize: bufSize,
		closed:  make(chan struct{}),
	}
	b.once.Do(func() {
		b.ch = make(chan Event, b.bufSize)
		go func() {
			for e := range b.ch {
				if b.sink != nil {
					func() { defer func() { _ = recover() }(); b.sink(e) }()
				}
			}
			close(b.closed)
		}()
	})
	return b
}

// Emit sends an event to the bus for asynchronous processing.
//
// This method is non-blocking and safe for concurrent use.
// If the event buffer is full, the event is dropped to prevent blocking.
//
// If the event's Time field is zero, it will be automatically set to the current time.
//
// If the bus is nil or has no sink configured, the event is ignored.
func (b *Bus) Emit(e Event) {
	if b == nil || b.sink == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	select {
	case b.ch <- e:
	default:
		// drop under backpressure
	}
}

// Close shuts down the event bus and waits for all queued events to be processed.
//
// After calling Close, any subsequent calls to Emit will be ignored.
// This method blocks until the background goroutine has finished processing
// all remaining events in the queue.
//
// It is safe to call Close multiple times or on a nil Bus.
func (b *Bus) Close() {
	if b == nil || b.ch == nil {
		return
	}
	close(b.ch)
	<-b.closed
}
