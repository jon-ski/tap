package tap

import (
	"sync"
	"time"
)

type EventKind int

const (
	EventDebug EventKind = -4
	EventInfo  EventKind = 0
	EventWarn  EventKind = 4
	EventError EventKind = 8
)

// Well-known meta keys. Optional.
const (
	MetaBytes       = "bytes"       // []byte payload preview
	MetaDir         = "dir"         // "tx" or "rx" if you want it
	MetaAddr        = "addr"        // "host:port" or remote id
	MetaPath        = "path"        // tag/path/browse target
	MetaProto       = "proto"       // "ethernet/ip","modbus-tcp","opc-ua", etc.
	MetaLayer       = "layer"       // "tcp","encap","cpf","mr","mbap","ua-secure", etc.
	MetaCorrelation = "correlation" // any correlation/id you use
	MetaUnitID      = "unit"        // modbus unit id, etc.
)

type Event struct {
	Time time.Time
	Kind EventKind
	ID   string         // generic identifier for correlation
	Op   string         // operation/name
	Msg  string         // short human message, e.g. "success"
	Err  error          // nil if not an error event
	Meta map[string]any // extras: "bytes", "dir", "addr", "path", "proto", "layer", "etc".
}

// ---- Sink

type EventSink func(Event)

// Multi fan-out to many sinks. Returns a single EventSink.
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

type Bus struct {
	sink    EventSink
	ch      chan Event
	once    sync.Once
	bufSize int
	closed  chan struct{}
}

// NewBus creates a bus, bufSize <= 0 uses 256.
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

// Emit is non-blocking. Drops when full.
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

// Close stops the bus. Further Emit() is ignored.
func (b *Bus) Close() {
	if b == nil || b.ch == nil {
		return
	}
	close(b.ch)
	<-b.closed
}
