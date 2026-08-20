package model

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"sync"
	"time"
)

// Span is one timed operation within a trace. Every method is safe to call on a
// nil span, which is what Start returns when the context carries no trace or
// the trace was not sampled.
type Span struct {
	ID         int           `json:"id"`
	ParentID   int           `json:"parent_id,omitempty"`
	TraceID    string        `json:"trace_id"`
	Name       string        `json:"name"`
	Kind       Kind          `json:"kind"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration_ns,omitempty"`
	Depth      int           `json:"depth"`
	Filename   string        `json:"filename,omitempty"`
	Line       int           `json:"line,omitempty"`
	Attributes Attributes    `json:"attributes,omitempty"`
	Error      string        `json:"error,omitempty"`

	// mu is a pointer so span values can be copied into snapshots without
	// copying a lock. It is nil on inert copies.
	mu *sync.Mutex

	trace *Trace

	// err is the value behind Error, so Err returns what was recorded.
	err   error
	ended bool
}

// End records the span duration. It is idempotent: a deferred End plus an
// explicit End on an error path record one duration, not two.
func (s *Span) End() {
	if s == nil {
		return
	}
	s.lock()
	defer s.unlock()
	if s.ended {
		return
	}
	s.ended = true
	s.Duration = s.now().Sub(s.StartedAt)
	if s.Duration < 0 {
		s.Duration = 0
	}
}

// EndWithError records err on the span and ends it.
func (s *Span) EndWithError(err error) {
	s.RecordError(err)
	s.End()
}

// RecordError records an error on the span and marks the trace as failed. A nil
// error is ignored.
func (s *Span) RecordError(err error) {
	if s == nil || err == nil {
		return
	}
	s.lock()
	s.err = err
	s.Error = err.Error()
	trace := s.trace
	s.unlock()
	trace.RecordError(err)
}

// SetAttribute records a key/value pair on the span.
func (s *Span) SetAttribute(key string, value any) {
	if s == nil || key == "" {
		return
	}
	s.lock()
	defer s.unlock()
	if s.Attributes == nil {
		s.Attributes = make(Attributes, 4)
	}
	s.Attributes[key] = value
}

// SetAttributes records several key/value pairs on the span.
func (s *Span) SetAttributes(attributes Attributes) {
	if s == nil || len(attributes) == 0 {
		return
	}
	s.lock()
	defer s.unlock()
	if s.Attributes == nil {
		s.Attributes = make(Attributes, len(attributes))
	}
	maps.Copy(s.Attributes, attributes)
}

// SetSource records the source location shown in the span table.
func (s *Span) SetSource(filename string, line int) {
	if s == nil {
		return
	}
	s.lock()
	defer s.unlock()
	s.Filename = filename
	s.Line = line
}

// SetName replaces the span name.
func (s *Span) SetName(name string) {
	if s == nil || name == "" {
		return
	}
	s.lock()
	defer s.unlock()
	s.Name = name
}

// Err returns the error recorded on the span, or nil. A span decoded from JSON
// kept the message and not the value, and reports an error carrying it.
func (s *Span) Err() error {
	if s == nil {
		return nil
	}
	s.lock()
	defer s.unlock()
	switch {
	case s.err != nil:
		return s.err
	case s.Error != "":
		return errors.New(s.Error)
	default:
		return nil
	}
}

// Ended reports whether the span was ended.
func (s *Span) Ended() bool {
	if s == nil {
		return false
	}
	s.lock()
	defer s.unlock()
	return s.ended
}

// Elapsed returns the recorded duration, or the time since the span started
// when it has not ended yet.
func (s *Span) Elapsed() time.Duration {
	if s == nil {
		return 0
	}
	s.lock()
	defer s.unlock()
	if s.ended {
		return s.Duration
	}
	return s.now().Sub(s.StartedAt)
}

// Trace returns the trace the span belongs to.
func (s *Span) Trace() *Trace {
	if s == nil {
		return nil
	}
	return s.trace
}

// Context returns a context with the span as the active parent, so spans
// started from it nest below this one.
func (s *Span) Context(ctx context.Context) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(withTrace(ctx, s.trace), spanKey{}, s)
}

// SourceText returns the "file:L12" location of the span, or an empty string.
func (s *Span) SourceText() string {
	if s == nil {
		return ""
	}
	return SourceText(s.Filename, s.Line)
}

// SourceText renders a "file:L12" source location.
func SourceText(filename string, line int) string {
	switch {
	case line <= 0:
		return filename
	case filename == "":
		return "L" + strconv.Itoa(line)
	default:
		return filename + ":L" + strconv.Itoa(line)
	}
}

// Inert returns a copy of the span detached from the trace that recorded it,
// safe to embed in a render model or hand to a consumer. Copying a span value
// on its own would carry the lock and the back reference with it.
func (s *Span) Inert() Span {
	if s == nil {
		return Span{}
	}
	copied := *s
	copied.mu = nil
	copied.trace = nil
	return copied
}

// clone returns an inert copy of the span, safe to hand to snapshot consumers.
func (s *Span) clone() *Span {
	if s == nil {
		return nil
	}
	s.lock()
	defer s.unlock()
	copied := *s
	copied.mu = nil
	copied.trace = nil
	if s.Attributes != nil {
		copied.Attributes = maps.Clone(s.Attributes)
	}
	if !copied.ended {
		copied.Duration = s.now().Sub(s.StartedAt)
		if copied.Duration < 0 {
			copied.Duration = 0
		}
	}
	return &copied
}

// now returns the current time from the clock of the owning trace. The caller
// holds the span lock, so this must not take it again.
func (s *Span) now() time.Time {
	if s.trace != nil && s.trace.clock != nil {
		return s.trace.clock()
	}
	return time.Now()
}

func (s *Span) lock() {
	if s != nil && s.mu != nil {
		s.mu.Lock()
	}
}

func (s *Span) unlock() {
	if s != nil && s.mu != nil {
		s.mu.Unlock()
	}
}
