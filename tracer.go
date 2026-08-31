package oida

import (
	"context"
	"errors"
	"maps"
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/titpetric/oida/model"
	"github.com/titpetric/oida/storage"
)

// Tracer records traces into a ring buffer and serves the debug front end. The
// zero value is not usable; construct one with New.
type Tracer struct {
	opts    Options
	sampler Sampler
	storage Storage
	events  *broker
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
// the tracer an entry point uses is the one in Options.Tracer.
//
// With Options.ReadEnv set, which is what NewOptions returns, the OIDA_*
// environment is applied to opts first. A variable applies only where the code
// left the field at its default, so options set in code win over the
// environment, and a variable set to nothing leaves the default alone. The
// configuration guide lists them.
func New(opts Options) (*Tracer, error) {
	if opts.ReadEnv {
		if err := optionsFromEnv(&opts); err != nil {
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
		sampler:   samplerFor(opts),
		storage:   opts.Storage,
		events:    newBroker(),
		started:   clockNow(opts),
		active:    make(map[string]*Trace),
		stateTime: make(map[State]time.Duration),
		requests:  make(map[string]uint64),
	}
	tracer.enabled.Store(opts.Enabled)
	return tracer, nil
}

// Options returns the options the tracer was built with, as a copy the caller
// owns. The retention driver and the recorder itself are left out, and the
// list and map are cloned: a reader of the configuration has no business
// reaching the storage behind it or rewriting what the tracer runs on.
func (t *Tracer) Options() Options {
	if t == nil {
		return NewOptions("")
	}
	opts := t.opts
	opts.Storage = nil
	opts.Tracer = nil
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
	id, err := model.NewID(clockNow(t.opts))
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

// begin registers a new trace as active.
func (t *Tracer) begin(id, name string, info *model.HTTPInfo) *Trace {
	trace := model.NewTrace(id, name, traceOptionsFor(t.opts))
	trace.HTTP = info
	if t.opts.TrackMemoryUse {
		trace.TrackMemory()
	}

	t.mu.Lock()
	t.total++
	t.sampled++
	t.requests[model.TraceHost(*trace)]++
	t.active[id] = trace
	t.mu.Unlock()

	t.events.notify()
	return trace
}

// Finish completes a trace and moves it into the ring buffer.
func (t *Tracer) Finish(trace *Trace) {
	if t == nil || trace == nil {
		return
	}
	trace.Finish()

	if t.opts.TrackMemoryUse {
		trace.RecordMemory()
	}
	durations := trace.Durations()
	stored := trace.Clone()
	failed := stored.ErrorText != "" || stored.State == StateError

	t.mu.Lock()
	delete(t.active, trace.ID)
	if failed {
		t.failed++
	}
	for state, duration := range durations {
		t.stateTime[state] += duration
	}
	if t.opts.TrackMemoryUse {
		t.samples++
		t.allocated += trace.Memory.AllocatedBytes
	}
	t.mu.Unlock()

	if err := t.storage.Save(context.Background(), stored); err != nil {
		t.onError(err)
	}
	t.events.notify()
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
	return t.events.subscribe()
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
	live := make([]Trace, 0, len(t.active))
	pending := make([]*Trace, 0, len(t.active))
	for _, trace := range t.active {
		pending = append(pending, trace)
	}
	total, sampled, unsampled := t.total, t.sampled, t.unsampled
	failed := t.failed
	samples, allocated := t.samples, t.allocated
	requests := make(map[string]uint64, len(t.requests))
	for host, count := range t.requests {
		requests[host] = count
	}
	t.mu.RUnlock()

	for _, trace := range pending {
		live = append(live, trace.Clone())
		for state, duration := range trace.Durations() {
			stateTime[state] += duration
		}
	}
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

	limit := memoryLimit()
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
		Uptime:     clockNow(t.opts).Sub(t.started),
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
	t.mu.RLock()
	pending := make([]*Trace, 0, len(t.active))
	for _, trace := range t.active {
		pending = append(pending, trace)
	}
	t.mu.RUnlock()

	out := make([]Trace, 0, len(pending))
	for _, trace := range pending {
		out = append(out, trace.Clone())
	}
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

	t.mu.RLock()
	active, ok := t.active[id]
	t.mu.RUnlock()
	if !ok {
		return Trace{}, false
	}
	return active.Clone(), true
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
		if !t.Enabled() || ignoredPath(t.opts, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !t.sampler.Sample(r) {
			t.countUnsampled(r.Host)
			next.ServeHTTP(w, r)
			return
		}
		serveTraced(t, t.opts, next, w, r)
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

// memoryLimit returns the smallest memory limit the process is subject to, or
// zero when none can be determined.
func memoryLimit() uint64 {
	var limits []uint64
	if limit := debug.SetMemoryLimit(-1); limit > 0 && limit != math.MaxInt64 {
		limits = append(limits, uint64(limit))
	}
	for _, path := range []string{
		"/sys/fs/cgroup/memory.max",
		"/sys/fs/cgroup/memory/memory.limit_in_bytes",
	} {
		value, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(value)) == "max" {
			continue
		}
		if limit, err := strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64); err == nil && limit > 0 {
			limits = append(limits, limit)
		}
	}
	if value, err := os.ReadFile("/proc/meminfo"); err == nil {
		for line := range strings.SplitSeq(string(value), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				if kib, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					limits = append(limits, kib*1024)
				}
				break
			}
		}
	}
	if len(limits) == 0 {
		return 0
	}
	return slices.Min(limits)
}
