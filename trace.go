package oida

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// Trace is one recorded unit of work: an HTTP request, a background job, a cron
// tick or a startup step. Every method is safe to call on a nil trace.
type Trace struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Service   string        `json:"service,omitempty"`
	State     State         `json:"state"`
	StartedAt time.Time     `json:"started_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Duration  time.Duration `json:"duration_ns"`
	Error     string        `json:"error,omitempty"`

	// InFlight reports whether the trace was still running when it was copied.
	InFlight bool `json:"in_flight,omitempty"`

	HTTP   *HTTPInfo `json:"http,omitempty"`
	Memory MemoryUse `json:"memory"`

	Spans        []*Span `json:"spans,omitempty"`
	DroppedSpans int     `json:"dropped_spans,omitempty"`

	// mu is a pointer so trace values can be copied into snapshots without
	// copying a lock. It is nil on inert copies.
	mu        *sync.Mutex
	clock     func() time.Time
	maxSpans  int
	sequence  int
	changedAt time.Time
	stateTime map[State]time.Duration
	memStats  runtime.MemStats
	finished  bool
}

// newTrace returns a trace ready to record spans.
func newTrace(id, name string, opts Options) *Trace {
	now := opts.now()
	return &Trace{
		ID:        id,
		Name:      name,
		Service:   opts.ServiceName,
		State:     StateStarting,
		StartedAt: now,
		UpdatedAt: now,
		mu:        new(sync.Mutex),
		clock:     opts.Clock,
		maxSpans:  opts.MaxSpansPerTrace,
		changedAt: now,
		stateTime: make(map[State]time.Duration, len(states)),
	}
}

// StartSpan records a span whose parent is the active span in ctx. The returned
// context carries the new span, so nested StartSpan calls nest below it.
func (t *Trace) StartSpan(ctx context.Context, name string, kind ...Kind) (context.Context, *Span) {
	if t == nil {
		return ctx, nil
	}
	span := t.appendSpan(spanFromContext(ctx), name, firstKind(kind))
	if span == nil {
		return ctx, nil
	}
	return span.Context(ctx), span
}

// appendSpan records a span below parent, honouring the span limit.
func (t *Trace) appendSpan(parent *Span, name string, kind Kind) *Span {
	t.lock()
	defer t.unlock()

	if t.finished {
		return nil
	}
	if t.maxSpans > 0 && len(t.Spans) >= t.maxSpans {
		t.DroppedSpans++
		return nil
	}

	t.sequence++
	span := &Span{
		ID:        t.sequence,
		TraceID:   t.ID,
		Name:      name,
		Kind:      kind,
		StartedAt: t.time(),
		mu:        new(sync.Mutex),
		trace:     t,
	}
	if parent != nil && parent.TraceID == t.ID {
		span.ParentID = parent.ID
		span.Depth = parent.Depth + 1
	}
	t.Spans = append(t.Spans, span)
	t.UpdatedAt = span.StartedAt
	return span
}

// Root returns the first recorded span, or nil.
func (t *Trace) Root() *Span {
	if t == nil {
		return nil
	}
	t.lock()
	defer t.unlock()
	if len(t.Spans) == 0 {
		return nil
	}
	return t.Spans[0]
}

// SetState transitions the trace state, accumulating the time spent in the
// previous state.
func (t *Trace) SetState(state State) {
	if t == nil {
		return
	}
	t.lock()
	defer t.unlock()
	if t.State == StateError && state != StateError {
		return
	}
	now := t.time()
	t.stateTime[t.State] += now.Sub(t.changedAt)
	t.State = state
	t.changedAt = now
	t.UpdatedAt = now
}

// Fail records a trace level error and marks the trace as failed.
func (t *Trace) Fail(err error) {
	if t == nil || err == nil {
		return
	}
	t.lock()
	defer t.unlock()
	t.Error = err.Error()
	now := t.time()
	t.stateTime[t.State] += now.Sub(t.changedAt)
	t.State = StateError
	t.changedAt = now
	t.UpdatedAt = now
}

// SetName replaces the trace name.
func (t *Trace) SetName(name string) {
	if t == nil || name == "" {
		return
	}
	t.lock()
	defer t.unlock()
	t.Name = name
}

// setResponse records the response metadata of an HTTP trace.
func (t *Trace) setResponse(status int, bytes int64, route string) {
	if t == nil {
		return
	}
	t.lock()
	defer t.unlock()
	if t.HTTP == nil {
		t.HTTP = &HTTPInfo{}
	}
	t.HTTP.Status = status
	t.HTTP.ResponseBytes = bytes
	if route != "" {
		t.HTTP.Route = route
	}
	t.UpdatedAt = t.time()
}

// Failed reports whether the trace recorded an error.
func (t *Trace) Failed() bool {
	if t == nil {
		return false
	}
	t.lock()
	defer t.unlock()
	return t.Error != "" || t.State == StateError
}

// SpanCount returns the number of recorded spans.
func (t *Trace) SpanCount() int {
	if t == nil {
		return 0
	}
	t.lock()
	defer t.unlock()
	return len(t.Spans)
}

// Elapsed returns the recorded duration, or the time since the trace started
// when it is still in flight.
func (t *Trace) Elapsed() time.Duration {
	if t == nil {
		return 0
	}
	t.lock()
	defer t.unlock()
	if t.Duration > 0 {
		return t.Duration
	}
	return t.time().Sub(t.StartedAt)
}

// Status returns the HTTP response status of the trace, or zero.
func (t *Trace) Status() int {
	if t == nil || t.HTTP == nil {
		return 0
	}
	return t.HTTP.Status
}

// Kinds returns the distinct span kinds recorded in the trace, in first use
// order.
func (t *Trace) Kinds() []Kind {
	if t == nil {
		return nil
	}
	t.lock()
	defer t.unlock()
	seen := make(map[Kind]struct{}, len(t.Spans))
	kinds := make([]Kind, 0, len(t.Spans))
	for _, span := range t.Spans {
		if span == nil {
			continue
		}
		if _, ok := seen[span.Kind]; ok {
			continue
		}
		seen[span.Kind] = struct{}{}
		kinds = append(kinds, span.Kind)
	}
	return kinds
}

// HasKind reports whether the trace recorded a span of the given kind.
func (t *Trace) HasKind(kind Kind) bool {
	if t == nil || kind == "" {
		return false
	}
	for _, recorded := range t.Kinds() {
		if recorded == kind {
			return true
		}
	}
	return false
}

// Clone returns an inert deep copy of the trace, safe to hand to snapshot
// consumers. Mutating the copy cannot affect the tracer.
func (t *Trace) Clone() Trace {
	if t == nil {
		return Trace{}
	}
	t.lock()
	defer t.unlock()

	copied := *t
	copied.mu = nil
	copied.clock = nil
	copied.stateTime = nil
	copied.InFlight = !t.finished
	copied.memStats = runtime.MemStats{}
	if t.HTTP != nil {
		info := *t.HTTP
		copied.HTTP = &info
	}
	if len(t.Spans) > 0 {
		copied.Spans = make([]*Span, 0, len(t.Spans))
		for _, span := range t.Spans {
			copied.Spans = append(copied.Spans, span.clone())
		}
	}
	if copied.Duration == 0 {
		copied.Duration = t.time().Sub(t.StartedAt)
	}
	return copied
}

// durations returns the time spent per state, including the time accumulated in
// the current state up to now.
func (t *Trace) durations() map[State]time.Duration {
	if t == nil {
		return nil
	}
	t.lock()
	defer t.unlock()
	result := make(map[State]time.Duration, len(t.stateTime))
	for state, duration := range t.stateTime {
		result[state] = duration
	}
	if !t.finished {
		result[t.State] += t.time().Sub(t.changedAt)
	}
	return result
}

// finish closes the trace, ending every open span.
func (t *Trace) finish() {
	if t == nil {
		return
	}
	t.lock()
	if t.finished {
		t.unlock()
		return
	}
	now := t.time()
	t.stateTime[t.State] += now.Sub(t.changedAt)
	t.changedAt = now
	t.UpdatedAt = now
	t.Duration = now.Sub(t.StartedAt)
	if t.Duration < 0 {
		t.Duration = 0
	}
	t.finished = true
	spans := make([]*Span, len(t.Spans))
	copy(spans, t.Spans)
	t.unlock()

	for i := len(spans) - 1; i >= 0; i-- {
		spans[i].End()
	}
}

// time returns the current time from the trace clock. Callers hold the lock.
func (t *Trace) time() time.Time {
	if t.clock == nil {
		return time.Now()
	}
	return t.clock()
}

func (t *Trace) lock() {
	if t != nil && t.mu != nil {
		t.mu.Lock()
	}
}

func (t *Trace) unlock() {
	if t != nil && t.mu != nil {
		t.mu.Unlock()
	}
}

// firstKind returns the first kind of the variadic argument, defaulting to
// KindInternal.
func firstKind(kinds []Kind) Kind {
	for _, kind := range kinds {
		if kind != "" {
			return kind
		}
	}
	return KindInternal
}
