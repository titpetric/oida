package oida

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"runtime"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/titpetric/oida/internal"
	"github.com/titpetric/oida/model"
	"github.com/titpetric/oida/storage"
)

// Tracer records traces into a ring buffer and serves the debug front end. The
// zero value is not usable; construct one with New.
type Tracer struct {
	opts    Options
	sampler Sampler
	storage Storage
	events  *internal.Broker
	started time.Time
	enabled atomic.Bool

	// handler is the debug front end, built on the first request ServeHTTP
	// receives so an unmounted tracer never constructs it.
	handlerOnce sync.Once
	handler     http.Handler

	mu        sync.RWMutex
	active    map[string]*Trace
	total     uint64
	sampled   uint64
	unsampled uint64
	failed    uint64
	samples   uint64
	allocated uint64
	stateTime map[State]time.Duration

	// requests counts every request seen per host, sampled or not, so the
	// per-host view reports traffic rather than only what survived sampling.
	requests map[string]uint64
}

// The front end renders whatever satisfies model.Recorder, and this is what
// it is given.
var _ model.Recorder = (*Tracer)(nil)

// New returns a tracer built from opts. Nothing is stored in a package level
// variable: the tracer a request records into is the one in its context, and
// the tracer an entry point uses is the one handed to it.
//
// With Options.ReadEnv set, which is what NewOptions returns, the OIDA_*
// environment is applied to opts first. A variable applies only where the code
// left the field at its default, so options set in code win over the
// environment, and a variable set to nothing leaves the default alone. The
// configuration guide lists them.
func New(opts Options) (*Tracer, error) {
	if opts.ReadEnv {
		if err := internal.OptionsFromEnv(&opts); err != nil {
			return nil, err
		}
	}
	opts = opts.WithDefaults()
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	if opts.Storage == nil {
		opts.Storage = storage.NewMemoryStorage(opts.RingBufferSize)
	}

	tracer := &Tracer{
		opts:      opts,
		sampler:   internal.SamplerFor(opts),
		storage:   opts.Storage,
		events:    internal.NewBroker(),
		started:   internal.ClockNow(opts),
		active:    make(map[string]*Trace),
		stateTime: make(map[State]time.Duration),
		requests:  make(map[string]uint64),
	}
	tracer.enabled.Store(opts.Enabled)
	return tracer, nil
}

// Options returns the options the tracer was built with, as a copy the caller
// owns. The retention driver is left out and the list and map are cloned: a
// reader of the configuration has no business reaching the storage behind it
// or rewriting what the tracer runs on.
func (t *Tracer) Options() Options {
	if t == nil {
		return NewOptions("")
	}
	opts := t.opts
	opts.Storage = nil
	opts.IgnorePaths = slices.Clone(t.opts.IgnorePaths)
	opts.Users = maps.Clone(t.opts.Users)
	return opts
}

// Enabled reports whether the tracer records traces.
func (t *Tracer) Enabled() bool {
	return t != nil && t.enabled.Load()
}

// SetEnabled turns recording on or off at runtime. Retained traces are kept.
func (t *Tracer) SetEnabled(enabled bool) {
	if t == nil {
		return
	}
	t.enabled.Store(enabled)
}

// StartTrace begins a trace for work that does not arrive over HTTP. The caller
// must complete it with Finish.
func (t *Tracer) StartTrace(ctx context.Context, name string) (context.Context, *Trace, error) {
	if t == nil || !t.Enabled() {
		return ctx, nil, ErrDisabled
	}
	id, err := model.NewID(internal.ClockNow(t.opts))
	if err != nil {
		return ctx, nil, err
	}
	trace := t.begin(id, name, nil)
	trace.SetState(StateProcessing)
	ctx, _ = trace.StartSpan(WithTrace(ctx, trace), name, KindInternal)
	return ctx, trace, nil
}

// Observe runs fn inside its own trace, records the returned error and
// completes the trace. It is what background jobs and cron ticks should use.
func (t *Tracer) Observe(ctx context.Context, name string, fn func(context.Context) error) error {
	traced, trace, err := t.StartTrace(ctx, name)
	if err != nil {
		return fn(ctx)
	}
	defer t.Finish(trace)

	err = fn(traced)
	trace.RecordError(err)
	return err
}

// serve records one request into the tracer and hands it to next.
func (t *Tracer) serve(opts Options, next http.Handler, w http.ResponseWriter, r *http.Request) {
	id := internal.RequestID(r, opts)
	if id == "" {
		next.ServeHTTP(w, r)
		return
	}

	r.Header.Set(RequestIDHeader, id)
	w.Header().Set(RequestIDHeader, id)

	name := r.Method + " " + r.URL.Path
	trace := t.begin(id, name, &model.HTTPInfo{
		Method:        r.Method,
		URI:           r.URL.RequestURI(),
		Host:          r.Host,
		Protocol:      r.Proto,
		RemoteAddress: internal.RemoteAddr(r),
		UserAgent:     r.UserAgent(),
	})
	trace.SetState(StateReading)

	// The root span alone carries the trace into the request context;
	// TraceFromContext resolves through it.
	ctx, span := trace.StartSpan(r.Context(), name, KindHTTP)
	r = r.WithContext(ctx)

	writer := internal.NewResponseWriter(w, trace)

	defer func() {
		if recovered := recover(); recovered != nil {
			err := fmt.Errorf("panic: %v", recovered)
			span.RecordError(err)
			t.finalize(opts, trace, writer, r)
			panic(recovered)
		}
		t.finalize(opts, trace, writer, r)
	}()

	trace.SetState(StateProcessing)
	next.ServeHTTP(writer, r)
}

// finalize records the response metadata and completes the trace.
func (t *Tracer) finalize(opts Options, trace *Trace, w *internal.ResponseWriter, r *http.Request) {
	route := internal.RoutePattern(r, opts)
	trace.SetResponse(w.Status(), w.Bytes(), route)
	if route != "" {
		trace.SetName(r.Method + " " + route)
	}
	if w.Status() >= http.StatusInternalServerError && trace.Err() == nil {
		trace.RecordError(fmt.Errorf("http %d", w.Status()))
	}
	t.Finish(trace)
	// The middleware owns this trace end to end: once the handler returned,
	// nothing outside serve holds it, so its box goes back to the pool.
	// Traces from StartTrace and Observe are never released; a caller may
	// hold those past Finish.
	trace.Release()
	w.Release()
}

// begin registers a new trace as active.
func (t *Tracer) begin(id, name string, info *model.HTTPInfo) *Trace {
	trace := model.NewTrace(id, name, internal.TraceOptionsFor(t.opts))
	trace.SetHTTPInfo(info)
	if t.opts.TrackMemoryUse {
		trace.TrackMemory()
	}

	t.mu.Lock()
	t.total++
	t.sampled++
	t.requests[model.TraceHost(trace)]++
	t.active[id] = trace
	t.mu.Unlock()

	t.events.Notify()
	return trace
}

// Finish completes a trace and moves it into the ring buffer. The trace
// keeps its recorded values, so a caller holding it may still read it; the
// stored copy is read back through Traces, Trace and Snapshot.
func (t *Tracer) Finish(trace *Trace) {
	if t == nil || trace == nil {
		return
	}
	trace.Finish()

	if t.opts.TrackMemoryUse {
		trace.RecordMemory()
	}
	durations := trace.StateTimes()
	failed := trace.Failed()

	t.mu.Lock()
	delete(t.active, trace.ID)
	if failed {
		t.failed++
	}
	for i, duration := range durations {
		if duration != 0 {
			t.stateTime[model.States()[i]] += duration
		}
	}
	if t.opts.TrackMemoryUse {
		t.samples++
		t.allocated += trace.Memory.AllocatedBytes
	}
	t.mu.Unlock()

	// Storage clones what it keeps; the live trace stays the caller's.
	if err := t.storage.Save(context.Background(), trace); err != nil {
		t.onError(err)
	}
	t.events.Notify()
}

// Subscribe returns a channel notified whenever a trace starts or completes,
// and a function releasing it. The live view streams from this.
//
//	events, cancel := tracer.Subscribe()
//	defer cancel()
//	for range events {
//		render(tracer.Live())
//	}
//
// Notifications are coalesced, so a slow consumer cannot slow down recording.
func (t *Tracer) Subscribe() (<-chan struct{}, func()) {
	if t == nil {
		return nil, func() {}
	}
	return t.events.Subscribe()
}

// ReportError forwards a failure to Options.OnError, which is where the front
// end reports its render failures too. Nothing is written to stdout or stderr.
func (t *Tracer) ReportError(err error) {
	t.onError(err)
}

// onError reports a recording or storage failure to the configured handler.
// Nothing is written to stdout or stderr.
func (t *Tracer) onError(err error) {
	if t == nil || err == nil || t.opts.OnError == nil {
		return
	}
	t.opts.OnError(err)
}

// Snapshot returns a race free copy of the tracer state. Nothing in the result
// aliases live state.
func (t *Tracer) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	t.mu.RLock()
	stateTime := make(map[State]time.Duration, len(t.stateTime))
	for state, duration := range t.stateTime {
		stateTime[state] = duration
	}
	// Cloned under the read lock: Finish recycles a trace only after the
	// write lock that removes it from active, so a trace cloned here
	// cannot be released underneath the clone.
	live := make([]Trace, 0, len(t.active))
	for _, trace := range t.active {
		live = append(live, trace.Clone())
		for state, duration := range trace.Durations() {
			stateTime[state] += duration
		}
	}
	total, sampled, unsampled := t.total, t.sampled, t.unsampled
	failed := t.failed
	samples, allocated := t.samples, t.allocated
	requests := make(map[string]uint64, len(t.requests))
	for host, count := range t.requests {
		requests[host] = count
	}
	t.mu.RUnlock()

	sort.Slice(live, func(i, j int) bool { return live[i].StartedAt.After(live[j].StartedAt) })

	log, err := t.storage.List(context.Background(), 0)
	if err != nil {
		t.onError(err)
		log = nil
	}
	windowLimit := t.storage.Cap()

	dropped := unsampled
	if retained := uint64(len(log) + len(live)); sampled > retained {
		dropped += sampled - retained
	}

	// Recorded traces are the only ones whose outcome is known, so they are the
	// denominator. With full sampling that is every request.
	sla := 100.0
	if sampled > 0 {
		sla = 100 - float64(failed)*100/float64(sampled)
	}

	limit := internal.MemoryLimit()
	pool := model.PoolEstimate{Samples: samples}
	if samples > 0 {
		pool.AverageAllocatedBytes = allocated / samples
		if pool.AverageAllocatedBytes > 0 {
			if mem.NextGC > mem.HeapAlloc {
				pool.BeforeNextGC = (mem.NextGC - mem.HeapAlloc) / pool.AverageAllocatedBytes
			}
			if limit > mem.Sys {
				pool.WithinMemoryLimit = (limit - mem.Sys) / pool.AverageAllocatedBytes
			}
		}
	}

	return Snapshot{
		Service:    t.opts.ServiceName,
		StartedAt:  t.started,
		Uptime:     internal.ClockNow(t.opts).Sub(t.started),
		PID:        os.Getpid(),
		GoVersion:  runtime.Version(),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
		Goroutines: runtime.NumGoroutine(),
		Total:      total,
		Sampled:    sampled,
		Dropped:    dropped,
		Active:     len(live),
		Errors:     failed,
		SLA:        sla,
		StateTime:  model.StateDurations(stateTime),
		Memory: model.Memory{
			HeapAlloc:     mem.HeapAlloc,
			HeapInuse:     mem.HeapInuse,
			HeapObjects:   mem.HeapObjects,
			StackInuse:    mem.StackInuse,
			System:        mem.Sys,
			NextGC:        mem.NextGC,
			NumGC:         mem.NumGC,
			GCPauseTotal:  mem.PauseTotalNs,
			GCCPUFraction: mem.GCCPUFraction,
			Limit:         limit,
		},
		Pool:       pool,
		Live:       live,
		Log:        log,
		Statistics: model.Statistics(log, windowLimit, t.opts.TopRequests, requests),
	}
}

// Traces returns the retained traces, newest first. The result is read only:
// its spans are the ones the front end renders, so recording into them is not
// a caller's to do.
func (t *Tracer) Traces() []Trace {
	if t == nil {
		return nil
	}
	traces, err := t.storage.List(context.Background(), 0)
	if err != nil {
		t.onError(err)
		return nil
	}
	return traces
}

// Live returns the traces currently in flight, newest first.
func (t *Tracer) Live() []Trace {
	if t == nil {
		return nil
	}
	// Cloned under the read lock, so Finish cannot recycle a trace between
	// collecting it and copying it.
	t.mu.RLock()
	out := make([]Trace, 0, len(t.active))
	for _, trace := range t.active {
		out = append(out, trace.Clone())
	}
	t.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// Trace returns the retained or in flight trace with the given ID. A retained
// trace is read only, the way Traces returns them; an in flight one is a copy
// the caller owns.
func (t *Tracer) Trace(id string) (Trace, bool) {
	if t == nil || id == "" {
		return Trace{}, false
	}

	trace, err := t.storage.Load(context.Background(), id)
	switch {
	case err == nil:
		return trace, true
	case !errors.Is(err, ErrTraceNotFound):
		t.onError(err)
	}

	// Cloned under the read lock, so Finish cannot recycle the trace
	// between the lookup and the copy.
	t.mu.RLock()
	active, ok := t.active[id]
	var copied Trace
	if ok {
		copied = active.Clone()
	}
	t.mu.RUnlock()
	return copied, ok
}

// Reset drops every retained trace and the lifetime counters. Traces in flight
// are left alone and are recorded when they complete.
func (t *Tracer) Reset() {
	if t == nil {
		return
	}
	if err := t.storage.Reset(context.Background()); err != nil {
		t.onError(err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.total, t.sampled, t.unsampled, t.failed = 0, 0, 0, 0
	t.samples, t.allocated = 0, 0
	clear(t.stateTime)
	clear(t.requests)
}

// Middleware records every sampled request handled by next. It is compatible
// with chi's Use, with alice, and with any func(http.Handler) http.Handler
// chain:
//
//	r.Use(tracer.Middleware)
//
// A nil tracer passes every request through, so instrumented wiring runs
// unchanged in a process that built none.
func (t *Tracer) Middleware(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	if t == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !t.Enabled() || internal.IgnoredPath(t.opts, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !t.sampler.Sample(r) {
			t.countUnsampled(r.Host)
			next.ServeHTTP(w, r)
			return
		}
		t.serve(t.opts, next, w, r)
	})
}

// countUnsampled records a request the sampler rejected. Sampled units of work
// are counted by begin, so Snapshot.Total covers HTTP requests and background
// traces alike. The host is still counted: a host that is sampled at one in a
// hundred still has traffic worth seeing.
func (t *Tracer) countUnsampled(host string) {
	if t == nil {
		return
	}
	if host == "" {
		host = model.BackgroundHost
	}

	t.mu.Lock()
	t.total++
	t.unsampled++
	t.requests[host]++
	t.mu.Unlock()
}
