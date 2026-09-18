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

	// ErrorText is the message of the error recorded on the span. The JSON
	// key stays "error"; the Go name makes room for the Error log method.
	ErrorText string `json:"error,omitempty"`

	// mu is a pointer so span values can be copied into snapshots without
	// copying a lock. It is nil on inert copies.
	mu *sync.Mutex

	trace *Trace

	// boxCtx is the preallocated context of the span's box, nil on inert
	// copies.
	boxCtx *spanCtx

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
	// Positive by construction: recorded readings of one trace strictly
	// increase, so the end reading is after the start even when the clock
	// stood still between them.
	s.Duration = s.now().Sub(s.StartedAt)
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
	s.ErrorText = err.Error()
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
	case s.ErrorText != "":
		return errors.New(s.ErrorText)
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
	return s.peek().Sub(s.StartedAt)
}

// Trace returns the trace the span belongs to.
func (s *Span) Trace() *Trace {
	if s == nil {
		return nil
	}
	return s.trace
}

// Context returns a context with the span as the active parent, so spans
// started from it nest below this one. The context comes preallocated with
// the span, so one derivation per span costs nothing; a second call rebinds
// that same context to the new parent.
func (s *Span) Context(ctx context.Context) context.Context {
	if s == nil {
		return ctx
	}
	if s.boxCtx != nil {
		s.boxCtx.Context = ctx
		return s.boxCtx
	}
	// The span alone: TraceFromContext resolves the trace through it, so a
	// span context costs one value instead of two.
	return context.WithValue(ctx, spanKey{}, s)
}

// SourceText returns the "file:L12" location of the span, or an empty string.
func (s *Span) SourceText() string {
	if s == nil {
		return ""
	}
	return sourceText(s.Filename, s.Line)
}

// sourceText renders a "file:L12" source location.
func sourceText(filename string, line int) string {
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
// on its own would carry the lock and the back reference with it, and would
// read the fields of a span another goroutine is still recording into.
func (s *Span) Inert() Span {
	if s == nil {
		return Span{}
	}
	s.lock()
	defer s.unlock()

	copied := *s
	copied.mu = nil
	copied.trace = nil
	copied.boxCtx = nil
	if s.Attributes != nil {
		copied.Attributes = maps.Clone(s.Attributes)
	}
	return copied
}

// cloneInto writes an inert copy of the span into dst, one entry of the
// block Trace.Clone allocates for the whole span list.
func (s *Span) cloneInto(dst *Span) {
	if s == nil {
		return
	}
	s.lock()
	defer s.unlock()
	*dst = *s
	dst.mu = nil
	dst.trace = nil
	dst.boxCtx = nil
	if s.Attributes != nil {
		dst.Attributes = maps.Clone(s.Attributes)
	}
	if !dst.ended {
		dst.Duration = s.peek().Sub(s.StartedAt)
		if dst.Duration < 0 {
			dst.Duration = 0
		}
	}
}

// now returns a recorded reading of the owning trace's clock, strictly after
// every earlier one, for the moments the span writes down. The caller holds
// the span lock, so this must not take it again.
func (s *Span) now() time.Time {
	if s.trace != nil {
		return s.trace.time()
	}
	return time.Now()
}

// peek returns the owning trace's clock without recording it, for readings
// that measure and do not record. The caller holds the span lock, so this
// must not take it again.
func (s *Span) peek() time.Time {
	if s.trace != nil {
		return s.trace.peek()
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

// Spans is the recorded span list of a trace.
type Spans []*Span

// Find returns the span with the id, or nil when the trace recorded none
// with it, which includes an entry written outside any span.
func (s Spans) Find(id int) *Span {
	for _, span := range s {
		if span != nil && span.ID == id {
			return span
		}
	}
	return nil
}

// Info records an informational log entry on the trace of the span, attributed
// to this span, from slog-style key/value pairs. It does nothing with log
// capture disabled, and tolerates a nil span.
func (s *Span) Info(message string, args ...any) {
	if s == nil || !s.trace.logsEnabled() {
		return
	}
	s.trace.appendLog(LevelInfo, s.ID, message, args)
}

// Warn records a warn-level log entry on the trace of the span, attributed to
// this span, from slog-style key/value pairs. It does nothing with log capture
// disabled, and tolerates a nil span.
func (s *Span) Warn(message string, args ...any) {
	if s == nil || !s.trace.logsEnabled() {
		return
	}
	s.trace.appendLog(LevelWarn, s.ID, message, args)
}

// Error records an error-level log entry on the trace of the span, attributed
// to this span; marking the transaction failed is Span.RecordError. With log
// capture disabled it records the text through RecordError, and it tolerates a
// nil span.
func (s *Span) Error(message string, args ...any) {
	if s == nil {
		return
	}
	if !s.trace.logsEnabled() {
		s.RecordError(errors.New(formatLogText(message, args)))
		return
	}
	s.trace.appendLog(LevelError, s.ID, message, args)
}

// StartSpan records a span without deriving a context. Use it for leaf spans
// that will not nest.
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span {
	_, span := Start(ctx, name, kind...)
	return span
}
