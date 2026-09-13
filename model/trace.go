package model

import (
	"context"
	"errors"
	"maps"
	"math"
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

	// ErrorText is the message of the error recorded on the trace. The JSON
	// key stays "error"; the Go name makes room for the Error log method.
	ErrorText string `json:"error,omitempty"`

	// InFlight reports whether the trace was still running when it was copied.
	InFlight bool `json:"in_flight,omitempty"`

	HTTP   *HTTPInfo `json:"http,omitempty"`
	Memory MemoryUse `json:"memory"`

	// Attributes is what the transaction recorded about itself, such as the
	// memory limit it ran under.
	Attributes Attributes `json:"attributes,omitempty"`

	Spans        Spans `json:"spans,omitempty"`
	DroppedSpans int   `json:"dropped_spans,omitempty"`

	// mu is a pointer so trace values can be copied into snapshots without
	// copying a lock. It is nil on inert copies.
	mu    *sync.Mutex
	clock func() time.Time

	// err is the value behind Error, so Err returns what was recorded.
	err       error
	maxSpans  int
	sequence  int
	changedAt time.Time
	stateTime [numStates]time.Duration
	memBefore memCounters
	finished  bool

	// Logs are the log lines recorded while the trace ran, in write order,
	// each linked to the span that was active when it was written. They are
	// bounded by the span limit; excess entries are counted in DroppedLogs.
	Logs        []LogEntry `json:"logs,omitempty"`
	DroppedLogs int        `json:"dropped_logs,omitempty"`

	// captureLogs gates Info and Error, set once from TraceOptions.
	captureLogs bool

	// box is the pooled allocation the trace lives in, nil on clones and
	// on decoded traces.
	box *traceBox

	// spanBlock is the backing block the Spans of a clone point into, kept
	// so CloneInto can rewrite it in place instead of allocating a new one.
	// It is nil on live traces.
	spanBlock []Span
}

// NewTrace returns a trace ready to record spans. The recorder passes the parts
// of its configuration a trace needs; everything else about a trace is set by
// recording it.
//
// Every trace is backed by a pooled box: the trace, its mutex, its HTTP info
// and slots for the first spans share one reused allocation. A trace that is
// never released is collected like any other value; Release is the
// recorder's, for the traces whose lifetime it owns end to end.
func NewTrace(id, name string, opts TraceOptions) *Trace {
	box := tracePool.Get().(*traceBox)
	*box = traceBox{}
	now := opts.now()
	trace := &box.trace
	*trace = Trace{
		ID:        id,
		Name:      name,
		Service:   opts.Service,
		State:     StateStarting,
		StartedAt: now,
		UpdatedAt: now,
		mu:        &box.mu,
		clock:     opts.Clock,
		maxSpans:  opts.MaxSpans,
		changedAt: now,

		captureLogs: opts.CaptureLogs,
		box:         box,
	}
	trace.Spans = box.ptrs[:0:len(box.ptrs)]
	return trace
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
	// One allocation carries the span and its lock; a pooled trace hands
	// out slots from its own box before touching the heap.
	var box *spanBox
	if t.box != nil && t.box.used < len(t.box.spans) {
		box = &t.box.spans[t.box.used]
		t.box.used++
	} else {
		box = new(spanBox)
	}
	span := &box.span
	*span = Span{
		ID:        t.sequence,
		TraceID:   t.ID,
		Name:      name,
		Kind:      kind,
		StartedAt: t.time(),
		mu:        &box.mu,
		trace:     t,
		boxCtx:    &box.ctx,
	}
	box.ctx.span = span
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

// Current returns the innermost open span: the most recently started span that
// has not ended. It is nil on a nil trace and when no span is open, so a
// caller holding only the trace can still attribute work to the active span.
func (t *Trace) Current() *Span {
	if t == nil {
		return nil
	}
	t.lock()
	defer t.unlock()
	return t.openSpan()
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
	t.stateTime[stateIndex(t.State)] += now.Sub(t.changedAt)
	t.State = state
	t.changedAt = now
	t.UpdatedAt = now
}

// RecordError records an error on the trace and moves it to StateError. It is
// what Span.RecordError calls after recording on the span. A nil error is
// ignored.
func (t *Trace) RecordError(err error) {
	if t == nil || err == nil {
		return
	}
	t.lock()
	defer t.unlock()
	t.err = err
	t.ErrorText = err.Error()
	now := t.time()
	t.stateTime[stateIndex(t.State)] += now.Sub(t.changedAt)
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

// SetAttribute records a key/value pair on the trace. Use it for what holds for
// the whole transaction; what holds for one operation belongs on its span.
func (t *Trace) SetAttribute(key string, value any) {
	if t == nil || key == "" {
		return
	}
	t.lock()
	defer t.unlock()
	if t.Attributes == nil {
		t.Attributes = make(Attributes, 4)
	}
	t.Attributes[key] = value
}

// SetAttributes records several key/value pairs on the trace.
func (t *Trace) SetAttributes(attributes Attributes) {
	if t == nil || len(attributes) == 0 {
		return
	}
	t.lock()
	defer t.unlock()
	if t.Attributes == nil {
		t.Attributes = make(Attributes, len(attributes))
	}
	maps.Copy(t.Attributes, attributes)
}

// Attribute returns an attribute of the trace, and whether it was recorded.
func (t *Trace) Attribute(key string) (any, bool) {
	if t == nil {
		return nil, false
	}
	t.lock()
	defer t.unlock()
	value, ok := t.Attributes[key]
	return value, ok
}

// SetHTTPInfo records the request metadata of an HTTP trace, copying the
// value. The pointer is not retained, so a caller's stack-allocated HTTPInfo
// stays on the stack; a pooled trace copies into its own box. A nil info
// leaves the trace as it is.
func (t *Trace) SetHTTPInfo(info *HTTPInfo) {
	if t == nil || info == nil {
		return
	}
	t.lock()
	defer t.unlock()
	if t.box != nil {
		t.box.http = *info
		t.HTTP = &t.box.http
		return
	}
	copied := *info
	t.HTTP = &copied
}

// SetResponse records the response metadata of an HTTP trace.
func (t *Trace) SetResponse(status int, bytes int64, route string) {
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

// Err returns the error recorded on the trace, or nil. A trace decoded from
// JSON kept the message and not the value, and reports an error carrying it.
func (t *Trace) Err() error {
	if t == nil {
		return nil
	}
	t.lock()
	defer t.unlock()
	switch {
	case t.err != nil:
		return t.err
	case t.ErrorText != "":
		return errors.New(t.ErrorText)
	default:
		return nil
	}
}

// Failed reports whether the trace recorded an error, by message or by
// state.
func (t *Trace) Failed() bool {
	if t == nil {
		return false
	}
	t.lock()
	defer t.unlock()
	return t.ErrorText != "" || t.State == StateError
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
	var copied Trace
	t.CloneInto(&copied)
	return copied
}

// CloneInto writes an inert deep copy of the trace into dst, reusing the
// allocations dst already owns when they fit: the HTTPInfo, the span block,
// the span pointer slice and the log slice are rewritten in place. It is how
// the ring buffer retains a trace without allocating for it, and Clone with
// an empty destination.
func (t *Trace) CloneInto(dst *Trace) {
	if dst == nil {
		return
	}
	if t == nil {
		*dst = Trace{}
		return
	}
	t.lock()
	defer t.unlock()

	// What the destination brings to reuse, taken before it is overwritten.
	reuseHTTP := dst.HTTP
	reuseSpans := dst.Spans
	reuseBlock := dst.spanBlock
	reuseLogs := dst.Logs

	copied := *t
	copied.mu = nil
	copied.clock = nil
	copied.InFlight = !t.finished
	copied.box = nil
	copied.Spans = nil
	copied.spanBlock = nil
	copied.Logs = nil
	if t.HTTP != nil {
		if reuseHTTP == nil {
			reuseHTTP = &HTTPInfo{}
		}
		*reuseHTTP = *t.HTTP
		copied.HTTP = reuseHTTP
	}
	if t.Attributes != nil {
		copied.Attributes = maps.Clone(t.Attributes)
	}
	if n := len(t.Spans); n > 0 {
		// One block carries every span copy: two allocations for the
		// whole list, and none once the destination holds the capacity.
		if cap(reuseBlock) < n {
			reuseBlock = make([]Span, n)
		}
		if cap(reuseSpans) < n {
			reuseSpans = make(Spans, n)
		}
		block, spans := reuseBlock[:n], reuseSpans[:n]
		for i, span := range t.Spans {
			span.cloneInto(&block[i])
			spans[i] = &block[i]
		}
		copied.Spans = spans
		copied.spanBlock = block
	}
	if copied.Duration == 0 {
		copied.Duration = t.time().Sub(t.StartedAt)
	}
	if n := len(t.Logs); n > 0 {
		if cap(reuseLogs) < n {
			reuseLogs = make([]LogEntry, 0, n)
		}
		logs := reuseLogs[:0]
		for _, entry := range t.Logs {
			logs = append(logs, entry.clone())
		}
		copied.Logs = logs
	}
	*dst = copied
}

// Durations returns the time spent per state, including the time accumulated in
// the current state up to now.
func (t *Trace) Durations() map[State]time.Duration {
	if t == nil {
		return nil
	}
	t.lock()
	defer t.unlock()
	result := make(map[State]time.Duration, numStates)
	for i, duration := range t.stateTime {
		if duration != 0 {
			result[states[i]] = duration
		}
	}
	if !t.finished {
		result[t.State] += t.time().Sub(t.changedAt)
	}
	return result
}

// StateTimes returns the time spent per state, indexed as States lists them.
// It is Durations without the map, for a caller aggregating many traces.
func (t *Trace) StateTimes() (out [numStates]time.Duration) {
	if t == nil {
		return out
	}
	t.lock()
	defer t.unlock()
	out = t.stateTime
	if !t.finished {
		out[stateIndex(t.State)] += t.time().Sub(t.changedAt)
	}
	return out
}

// Finish closes the trace, ending every open span. It is idempotent.
func (t *Trace) Finish() {
	if t == nil {
		return
	}
	t.lock()
	if t.finished {
		t.unlock()
		return
	}
	now := t.time()
	t.stateTime[stateIndex(t.State)] += now.Sub(t.changedAt)
	t.changedAt = now
	t.UpdatedAt = now
	t.Duration = now.Sub(t.StartedAt)
	if t.Duration < 0 {
		t.Duration = 0
	}
	t.finished = true
	// The slice header alone: spans append only while the trace runs, and
	// the finished flag above just ended that.
	spans := t.Spans
	t.unlock()

	for i := len(spans) - 1; i >= 0; i-- {
		spans[i].End()
	}
}

// TrackMemory records the process memory counters the trace started with, so
// RecordMemory can report what it allocated.
func (t *Trace) TrackMemory() {
	if t == nil {
		return
	}
	t.memBefore = readMemCounters()
}

// RecordMemory records the process-wide allocation deltas observed while the
// trace ran. Concurrent traces overlap, so the values are indicative: this is
// the process moving, measured across one trace, not the trace on its own.
func (t *Trace) RecordMemory() {
	if t == nil {
		return
	}
	after := readMemCounters()

	t.Memory = MemoryUse{
		HeapDelta:      signedDelta(after.heapBytes, t.memBefore.heapBytes),
		AllocatedBytes: delta(after.allocBytes, t.memBefore.allocBytes),
		Allocations:    delta(after.allocs, t.memBefore.allocs),
		GCCycles:       uint32(delta(after.gcCycles, t.memBefore.gcCycles)),
		GCPause:        time.Duration(delta(after.pauseNs, t.memBefore.pauseNs)),
	}
	t.memBefore = memCounters{}
}

// spanBox is one allocation holding a span, the mutex it locks with and the
// context it derives, kept apart from the copyable Span value itself.
type spanBox struct {
	span Span
	mu   sync.Mutex
	ctx  spanCtx
}

// delta returns after-before, clamped at zero.
func delta(after, before uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

// signedDelta returns after-before as a signed value, saturating at the int64
// bounds.
func signedDelta(after, before uint64) int64 {
	if after >= before {
		d := after - before
		if d > math.MaxInt64 {
			return math.MaxInt64
		}
		return int64(d)
	}
	d := before - after
	if d > math.MaxInt64 {
		return math.MinInt64
	}
	return -int64(d)
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

// Info records an informational log entry on the trace, attributed to the
// innermost open span when one is open. No context is needed: a caller holding
// only the trace still lands the entry on the right span.
//
//	trace.Info("cache warmed", "keys", 128)
//
// Args are slog-style key/value pairs, kept on LogEntry.Attributes; the
// message is stored verbatim. When log capture is disabled it does nothing.
// Safe to call on a nil trace.
func (t *Trace) Info(message string, args ...any) {
	if !t.logsEnabled() {
		return
	}
	t.appendLog(LevelInfo, 0, message, args)
}

// Error records an error-level log entry on the trace, attributed to the
// innermost open span when one is open. It only logs: the trace state and
// Trace.ErrorText are untouched, which is RecordError's job. When log capture
// is disabled it records the formatted text through RecordError instead, on
// the innermost open span when one is open, so the message is not lost. Safe
// to call on a nil trace.
func (t *Trace) Error(message string, args ...any) {
	if t == nil {
		return
	}
	if !t.logsEnabled() {
		err := errors.New(formatLogText(message, args))
		if span := t.Current(); span != nil {
			span.RecordError(err)
			return
		}
		t.RecordError(err)
		return
	}
	t.appendLog(LevelError, 0, message, args)
}
