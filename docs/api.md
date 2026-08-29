# Package oida

```go
import (
	"github.com/titpetric/oida"
}
```

Package oida records in-process telemetry: traces and spans held in a ring
buffer inside the process, with a server side rendered front end mounted at
/debug/oida.

Wire it into a service in three calls: configure the tracer, add the
middleware, mount the dashboard from github.com/titpetric/oida/frontend.
With the standard library ServeMux:

```go
opts := oida.NewOptions()
opts.ServiceName = "billing-api"

tracer, err := oida.Configure(opts)
if err != nil {
	return err
}
opts.Tracer = tracer

mux := http.NewServeMux()
if err := frontend.MountServeMux(mux, opts); err != nil {
	return err
}
handler := oida.TracingMiddleware(opts)(mux)
return http.ListenAndServe(":8080", handler)
```

With a chi router the middleware registers like any other, and the same
options carry over:

```go
r := chi.NewRouter()
r.Use(oida.TracingMiddleware(opts))
if err := frontend.Mount(r, opts); err != nil {
	return err
}
```

Instrument anything below the middleware:

```go
ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
defer span.End()
span.SetAttribute("limit", limit)
```

Every instrumentation call is nil safe, so instrumented code runs unchanged
in processes where oida is disabled, where the request was not sampled, or
where no trace is in the context.

The project is three packages. This one records: the tracer, the middleware,
the options and the storage. Package model holds the recorded data and
depends on nothing; the types it defines are aliased here, so instrumenting a
service needs this import alone. Package frontend serves the dashboard and is
the only one that renders.

Nothing in this package writes to stdout or stderr. Storage and rendering
failures are reported through Options.OnError.

## Types

<details>
<summary><code>type Options</code></summary>

```go
// Options configures telemetry behaviour, the debug front end and the
// middleware. Take NewOptions and override what you need, so fields added in
// later versions keep their defaults.
type Options struct {
	// Path is the mount path of the debug front end.
	Path string `yaml:"path"`

	// ServiceName is displayed in the front end and recorded on every trace.
	ServiceName string `yaml:"service_name"`

	// Enabled records traces. A disabled tracer passes requests through.
	Enabled bool `yaml:"enabled"`

	// RingBufferSize is the number of completed traces retained.
	RingBufferSize int `yaml:"ring_buffer_size"`

	// TopRequests is the maximum number of groups in rolling statistics.
	TopRequests int `yaml:"top_requests"`

	// MaxSpansPerTrace bounds the spans recorded in a single trace. Excess
	// spans are counted in Trace.DroppedSpans. Zero means unlimited.
	MaxSpansPerTrace int `yaml:"max_spans_per_trace"`

	// SampleRate is the percentage of requests traced, between 0 and 100. It
	// is ignored when Sampler is set.
	SampleRate float64 `yaml:"sample_rate"`

	// TrackMemoryUse records process-wide allocation changes for each trace.
	TrackMemoryUse bool `yaml:"track_memory_use"`

	// TrustRequestID reuses a client supplied Request-Id header. Only enable
	// this behind a trusted proxy.
	TrustRequestID bool `yaml:"trust_request_id"`

	// IgnorePaths lists request paths that are never traced. Entries ending in
	// "/*" match by prefix.
	IgnorePaths []string `yaml:"ignore_paths"`

	// RefreshInterval is the fallback auto refresh interval of the live view in
	// seconds, used when the browser cannot stream. Zero disables it.
	RefreshInterval int `yaml:"refresh_interval"`

	// LiveStream serves the live view over server sent events, so recorded
	// traces appear as they happen instead of on a timer.
	LiveStream bool `yaml:"live_stream"`

	// Sampler decides whether a request is traced. It replaces SampleRate.
	Sampler Sampler `yaml:"-"`

	// Storage retains completed traces. Defaults to StorageMemory sized by
	// RingBufferSize; StorageDisk retains them across restarts.
	Storage Storage `yaml:"-"`

	// RouteFunc returns the routed pattern of a request, so statistics group
	// /users/1 and /users/2 into GET /users/{id}. With chi:
	//
	//	opts.RouteFunc = func(r *http.Request) string {
	//		return chi.RouteContext(r.Context()).RoutePattern()
	//	}
	//
	// The function decides on its own: returning an empty string means the
	// request has no route worth grouping by, and it groups by path instead.
	// A nil function falls back to the pattern the router recorded on the
	// request, which is what http.ServeMux and chi both set.
	RouteFunc func(r *http.Request) string `yaml:"-"`

	// OnError receives storage and recording errors. The package never writes
	// to stdout or stderr, so this is the only way to observe them.
	OnError func(error) `yaml:"-"`

	// Authorize gates access to the debug front end. A nil function allows
	// every request.
	Authorize func(r *http.Request) bool `yaml:"-"`

	// Clock is the time source of the tracer. Defaults to time.Now.
	Clock func() time.Time `yaml:"-"`

	// Tracer is the recorder used by TracingMiddleware, Handler and Mount. A
	// nil tracer resolves the process default.
	Tracer *Tracer `yaml:"-"`

	initialized bool
}
```

</details>

<details>
<summary><code>type Recorder</code></summary>

```go
// Recorder is the substitutable subset of Tracer. Code that only needs to
// record and read back traces can depend on this interface instead of the
// concrete tracer.
type Recorder interface {
	// StartTrace begins a trace the caller must complete with Finish.
	StartTrace(ctx context.Context, name string) (context.Context, *Trace, error)

	// Finish completes a trace and retains it.
	Finish(t *Trace)

	// Snapshot returns a race free copy of the recorded state.
	Snapshot() Snapshot
}
```

</details>

<details>
<summary><code>type Sampler</code></summary>

```go
// Sampler decides whether a request is traced. The decision is taken before a
// trace is allocated, so rejecting a request costs one interface call.
type Sampler interface {
	Sample(r *http.Request) bool
}
```

</details>

<details>
<summary><code>type SamplerFunc</code></summary>

```go
// SamplerFunc adapts a function to the Sampler interface.
type SamplerFunc func(r *http.Request) bool
```

</details>

<details>
<summary><code>type Storage</code></summary>

```go
// Storage retains completed traces. Implementations must be safe for concurrent
// use: the tracer writes from request goroutines and reads from the debug front
// end at the same time.
//
// Two implementations ship with the package: StorageMemory, a bounded ring
// buffer, and StorageDisk, a bounded folder of JSON documents.
type Storage interface {
	// Save retains a completed trace.
	Save(ctx context.Context, trace Trace) error

	// Load returns a retained trace, or ErrTraceNotFound.
	Load(ctx context.Context, id string) (Trace, error)

	// List returns retained traces newest first, at most limit of them. A limit
	// of zero or less returns everything retained.
	List(ctx context.Context, limit int) ([]Trace, error)

	// Len returns the number of retained traces.
	Len(ctx context.Context) (int, error)

	// Cap returns the retention limit, or zero when unbounded.
	Cap() int

	// Reset drops every retained trace.
	Reset(ctx context.Context) error
}
```

</details>

<details>
<summary><code>type StorageDisk</code></summary>

```go
// StorageDisk retains completed traces as JSON documents in a folder, so traces
// survive a process restart. Trace IDs are lexicographically sortable, so the
// folder listing is chronological and pruning drops the oldest documents.
type StorageDisk struct {
	mu    sync.Mutex
	path  string
	limit int
}
```

</details>

<details>
<summary><code>type StorageMemory</code></summary>

```go
// StorageMemory retains completed traces in a bounded ring buffer. It is the
// default storage: nothing leaves the process and memory use is bounded by the
// configured size.
type StorageMemory struct {
	mu  sync.RWMutex
	log *ring
}
```

</details>

<details>
<summary><code>type Tracer</code></summary>

```go
// Tracer records traces into a ring buffer and serves the debug front end. The
// zero value is not usable; construct one with New.
type Tracer struct {
	opts    Options
	sampler Sampler
	storage Storage
	events  *broker
	started time.Time
	enabled atomic.Bool

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
```

</details>

<details>
<summary><code>type Trace, Span, Attributes, Kind, State, HTTPInfo, MemoryUse, Memory, PoolEstimate, StateDuration, Statistic, HostStat, Stats, Snapshot</code></summary>

```go
// The recorded data lives in the model package, so the front end can read it
// without depending on the recorder. These aliases keep it spelled the way the
// rest of the API is: one import is enough to instrument a service.
type (
	// Trace is one recorded unit of work.
	Trace = model.Trace

	// Span is one timed operation within a trace.
	Span = model.Span

	// Attributes is a set of key/value pairs recorded on a trace or a span.
	Attributes = model.Attributes

	// Kind classifies the work a span measured.
	Kind = model.Kind

	// State is the scoreboard state of an in-flight trace.
	State = model.State

	// HTTPInfo describes the request a trace was created for.
	HTTPInfo = model.HTTPInfo

	// MemoryUse holds the allocation deltas observed while a trace ran.
	MemoryUse = model.MemoryUse

	// Memory describes current process memory and GC pressure.
	Memory = model.Memory

	// PoolEstimate is a heuristic concurrency estimate.
	PoolEstimate = model.PoolEstimate

	// StateDuration is the lifetime trace time observed in one state.
	StateDuration = model.StateDuration

	// Statistic aggregates one group of traces in the rolling window.
	Statistic = model.Statistic

	// HostStat aggregates the traffic of one host.
	HostStat = model.HostStat

	// Stats contains the most frequent trace groups in the rolling window.
	Stats = model.Stats

	// Snapshot is the complete read model of a tracer at one point in time.
	Snapshot = model.Snapshot
)
```

</details>

## Consts

<details>
<summary><code>const BackgroundHost</code></summary>

```go
// BackgroundHost is the host label of traces that did not arrive over the
// network: cron ticks, queue consumers, startup work.
const BackgroundHost = model.BackgroundHost
```

</details>

<details>
<summary><code>const DefaultPath</code></summary>

```go
// DefaultPath is the default mount path of the debug front end.
const DefaultPath = "/debug/oida"
```

</details>

<details>
<summary><code>const RequestIDHeader</code></summary>

```go
// RequestIDHeader carries the trace identifier on the request and the response.
const RequestIDHeader = "Request-Id"
```

</details>

<details>
<summary><code>const KindInternal, KindHTTP, KindDatabase, KindExternal, KindTemplate, KindCache, KindQueue</code></summary>

```go
// Span kinds. The set is open: an unrecognized value is valid.
const (
	KindInternal = model.KindInternal
	KindHTTP     = model.KindHTTP
	KindDatabase = model.KindDatabase
	KindExternal = model.KindExternal
	KindTemplate = model.KindTemplate
	KindCache    = model.KindCache
	KindQueue    = model.KindQueue
)
```

</details>

<details>
<summary><code>const AttrMemoryLimit, AttrMemoryUsage</code></summary>

```go
// Well known attribute keys. The set is open; these are the ones the front end
// renders as sizes.
const (
	AttrMemoryLimit = model.AttrMemoryLimit
	AttrMemoryUsage = model.AttrMemoryUsage
)
```

</details>

<details>
<summary><code>const StateWaiting, StateStarting, StateReading, StateProcessing, StateWriting, StateKeepalive, StateClosing, StateError</code></summary>

```go
// Scoreboard states of a trace in flight.
const (
	StateWaiting    = model.StateWaiting
	StateStarting   = model.StateStarting
	StateReading    = model.StateReading
	StateProcessing = model.StateProcessing
	StateWriting    = model.StateWriting
	StateKeepalive  = model.StateKeepalive
	StateClosing    = model.StateClosing
	StateError      = model.StateError
)
```

</details>

## Vars

<details>
<summary><code>var ErrNilRouter, ErrInvalidOptions, ErrInvalidPath, ErrInvalidSampleRate, ErrTraceNotFound, ErrDisabled</code></summary>

```go
// The errors this package returns. Every configuration failure wraps
// ErrInvalidOptions, so a caller can test for the class or for the case.
var (
	// ErrNilRouter is returned when Mount is called without a router.
	ErrNilRouter = errors.New("oida: router is nil")

	// ErrInvalidOptions is the base error for every configuration failure.
	ErrInvalidOptions = errors.New("oida: invalid options")

	// ErrInvalidPath is returned when Options.Path is not an absolute path.
	ErrInvalidPath = fmt.Errorf("%w: path must be an absolute path", ErrInvalidOptions)

	// ErrInvalidSampleRate is returned when Options.SampleRate is outside
	// [0,100].
	ErrInvalidSampleRate = fmt.Errorf("%w: sample rate must be between 0 and 100", ErrInvalidOptions)

	// ErrTraceNotFound is returned when a trace ID is not in the ring buffer.
	ErrTraceNotFound = errors.New("oida: trace not found")

	// ErrDisabled is returned when a trace is requested from a disabled tracer.
	ErrDisabled = errors.New("oida: tracer is disabled")
)
```

</details>

## Function symbols

- `func Configure (opts Options) (*Tracer, error)`
- `func Default () *Tracer`
- `func Do (ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error`
- `func IsBytes (key string) bool`
- `func MustResolve (opts Options) *Tracer`
- `func New (opts Options) (*Tracer, error)`
- `func NewOptions () Options`
- `func NewRateSampler (rate float64) Sampler`
- `func NewStorageDisk (limit int, paths ...string) (*StorageDisk, error)`
- `func NewStorageMemory (size int) *StorageMemory`
- `func RecordError (ctx context.Context, err error)`
- `func Resolve (opts Options) (*Tracer, error)`
- `func SpanFromContext (ctx context.Context) *Span`
- `func Start (ctx context.Context, name string, kind ...Kind) (context.Context, *Span)`
- `func StartAuto (ctx context.Context, symbol any, kind ...Kind) (context.Context, *Span)`
- `func StartRequest (r *http.Request, name string, kind ...Kind) (*http.Request, *Span)`
- `func StartSpan (ctx context.Context, name string, kind ...Kind) *Span`
- `func TraceFromContext (ctx context.Context) *Trace`
- `func TraceHost (trace Trace) string`
- `func TraceID (ctx context.Context) string`
- `func TracingMiddleware (opts Options) func(http.Handler) http.Handler`
- `func ValidID (id string) bool`
- `func WithTrace (ctx context.Context, t *Trace) context.Context`
- `func (*StorageDisk) Cap () int`
- `func (*StorageDisk) Len (ctx context.Context) (int, error)`
- `func (*StorageDisk) List (ctx context.Context, limit int) ([]Trace, error)`
- `func (*StorageDisk) Load (ctx context.Context, id string) (Trace, error)`
- `func (*StorageDisk) Path () string`
- `func (*StorageDisk) Prune (ctx context.Context, maxAge time.Duration) error`
- `func (*StorageDisk) Reset (ctx context.Context) error`
- `func (*StorageDisk) Save (ctx context.Context, trace Trace) error`
- `func (*StorageMemory) Cap () int`
- `func (*StorageMemory) Len (ctx context.Context) (int, error)`
- `func (*StorageMemory) List (ctx context.Context, limit int) ([]Trace, error)`
- `func (*StorageMemory) Load (ctx context.Context, id string) (Trace, error)`
- `func (*StorageMemory) Reset (ctx context.Context) error`
- `func (*StorageMemory) Save (ctx context.Context, trace Trace) error`
- `func (*Tracer) Enabled () bool`
- `func (*Tracer) Finish (trace *Trace)`
- `func (*Tracer) Live () []Trace`
- `func (*Tracer) Middleware (next http.Handler) http.Handler`
- `func (*Tracer) Observe (ctx context.Context, name string, fn func(context.Context) error) error`
- `func (*Tracer) Options () Options`
- `func (*Tracer) ReportError (err error)`
- `func (*Tracer) Reset ()`
- `func (*Tracer) SetEnabled (enabled bool)`
- `func (*Tracer) Snapshot () Snapshot`
- `func (*Tracer) StartTrace (ctx context.Context, name string) (context.Context, *Trace, error)`
- `func (*Tracer) Storage () Storage`
- `func (*Tracer) Subscribe () (<-chan struct{}, func())`
- `func (*Tracer) Trace (id string) (Trace, bool)`
- `func (*Tracer) Traces () []Trace`
- `func (Options) Authorized (r *http.Request) bool`
- `func (Options) Validate () error`
- `func (Options) WithDefaults () Options`
- `func (SamplerFunc) Sample (r *http.Request) bool`

### Configure

Configure replaces the process wide tracer with one built from opts and
returns it. Call it once during startup, before wiring the middleware.

```go
func Configure(opts Options) (*Tracer, error)
```

### Default

Default returns the process wide tracer, creating it with the default options
on first use. Prefer an explicit tracer from New in libraries and tests.

```go
func Default() *Tracer
```

### Do

Do runs fn inside a span, records the returned error on it and ends it. The
error is returned unchanged.

```go
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error
```

### IsBytes

IsBytes reports whether an attribute key holds a size in bytes.

```go
func IsBytes(key string) bool
```

### MustResolve

MustResolve returns the tracer for opts, falling back to the default tracer
when the options are invalid. It backs the entry points that cannot report an
error.

```go
func MustResolve(opts Options) *Tracer
```

### New

New returns a tracer configured with opts.

```go
func New(opts Options) (*Tracer, error)
```

### NewOptions

NewOptions returns the default options.

```go
func NewOptions() Options
```

### NewRateSampler

NewRateSampler returns a sampler tracing the given percentage of requests. A
rate of 100 or more traces everything, a rate of 0 or less traces nothing.

```go
func NewRateSampler(rate float64) Sampler
```

### NewStorageDisk

NewStorageDisk creates the storage folder, verifies that it is writable, and
retains at most limit traces. A limit of zero or less is unbounded. With no
path it uses a folder in the operating system temporary directory.

```go
func NewStorageDisk(limit int, paths ...string) (*StorageDisk, error)
```

### NewStorageMemory

NewStorageMemory returns in-memory storage retaining size traces. A size of
zero or less retains nothing, which is useful when only the live view and the
lifetime counters are wanted.

```go
func NewStorageMemory(size int) *StorageMemory
```

### RecordError

RecordError records err on the innermost span in ctx and on its trace.

```go
if err := store.Save(ctx, u); err != nil {
	oida.RecordError(ctx, err)
	return err
}
```

It is Span.RecordError for code that holds a context rather than the span. A
nil error, a context without a trace and an unsampled request are no-ops.

```go
func RecordError(ctx context.Context, err error)
```

### Resolve

Resolve returns the tracer the options point at: the explicit one when set,
the process default otherwise. The first resolution of the default configures
it from opts. The front end resolves the tracer it serves this way.

```go
func Resolve(opts Options) (*Tracer, error)
```

### SpanFromContext

SpanFromContext returns the innermost span in ctx, or nil.

```go
func SpanFromContext(ctx context.Context) *Span
```

### Start

Start records a span in the trace carried by ctx and returns a context
carrying it. When ctx has no trace, or the trace was not sampled, it returns
ctx unchanged and a nil span: every span method tolerates that.

```go
ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
defer span.End()
```

The kind is optional and defaults to KindInternal.

```go
func Start(ctx context.Context, name string, kind ...Kind) (context.Context, *Span)
```

### StartAuto

StartAuto is Start with the span name read from a symbol. Pass a function or
a value and the package, type and function names are joined with a dot, which
gives names like storage.UserStorage.GetUsers without spelling them out.

```go
ctx, span := oida.StartAuto(ctx, s.GetUsers)
defer span.End()
```

The name comes from reflection and the runtime symbol table, so it does not
survive a stripped binary and reads oddly for anonymous functions. Use Start
where either matters, or where the call is hot enough for the reflection to
show up.

```go
func StartAuto(ctx context.Context, symbol any, kind ...Kind) (context.Context, *Span)
```

### StartRequest

StartRequest is Start for code holding an *http.Request rather than a
context. It returns a request carrying the span, so spans started from the
returned request nest below this one.

```go
r, span := oida.StartRequest(r, "user.Handler")
defer span.End()
```

When the request carries no trace it is returned unchanged along with a nil
span, so the unsampled path allocates nothing.

```go
func StartRequest(r *http.Request, name string, kind ...Kind) (*http.Request, *Span)
```

### StartSpan

StartSpan records a span without deriving a context. Use it for leaf spans
that will not nest.

```go
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span
```

### TraceFromContext

TraceFromContext returns the trace in ctx, or nil.

```go
func TraceFromContext(ctx context.Context) *Trace
```

### TraceHost

TraceHost returns the host a trace belongs to. Background traces have none,
so they group under BackgroundHost rather than an empty string.

```go
func TraceHost(trace Trace) string
```

### TraceID

TraceID returns the identifier of the trace in ctx, or an empty string. It is
the value of the Request-Id header for HTTP traces, which makes it the
cheapest correlation key for logs.

```go
func TraceID(ctx context.Context) string
```

### TracingMiddleware

TracingMiddleware returns middleware recording every sampled request into the
tracer resolved from opts. It is compatible with chi's Use, with alice, and
with any func(http.Handler) http.Handler chain.

```go
func TracingMiddleware(opts Options) func(http.Handler) http.Handler
```

### ValidID

ValidID reports whether id looks like a trace identifier this package
records. It keeps hostile input out of lookups and out of rendered links.

```go
func ValidID(id string) bool
```

### WithTrace

WithTrace returns a context carrying the trace. Spans started from the
returned context, or any context derived from it, are recorded on it.

```go
func WithTrace(ctx context.Context, t *Trace) context.Context
```

### Cap

Cap returns the retention limit.

```go
func (*StorageDisk) Cap() int
```

### Len

Len returns the number of stored trace documents.

```go
func (*StorageDisk) Len(ctx context.Context) (int, error)
```

### List

List reads stored traces, newest first.

```go
func (*StorageDisk) List(ctx context.Context, limit int) ([]Trace, error)
```

### Load

Load reads a stored trace document.

```go
func (*StorageDisk) Load(ctx context.Context, id string) (Trace, error)
```

### Path

Path returns the folder traces are written to.

```go
func (*StorageDisk) Path() string
```

### Prune

Prune removes trace documents older than maxAge.

```go
func (*StorageDisk) Prune(ctx context.Context, maxAge time.Duration) error
```

### Reset

Reset removes every stored trace document.

```go
func (*StorageDisk) Reset(ctx context.Context) error
```

### Save

Save writes a trace document atomically and prunes the oldest documents over
the retention limit.

```go
func (*StorageDisk) Save(ctx context.Context, trace Trace) error
```

### Cap

Cap returns the retention limit.

```go
func (*StorageMemory) Cap() int
```

### Len

Len returns the number of retained traces.

```go
func (*StorageMemory) Len(ctx context.Context) (int, error)
```

### List

List returns retained traces, newest first.

```go
func (*StorageMemory) List(ctx context.Context, limit int) ([]Trace, error)
```

### Load

Load returns a retained trace.

```go
func (*StorageMemory) Load(ctx context.Context, id string) (Trace, error)
```

### Reset

Reset drops every retained trace.

```go
func (*StorageMemory) Reset(ctx context.Context) error
```

### Save

Save retains a completed trace, evicting the oldest one when full.

```go
func (*StorageMemory) Save(ctx context.Context, trace Trace) error
```

### Enabled

Enabled reports whether the tracer records traces.

```go
func (*Tracer) Enabled() bool
```

### Finish

Finish completes a trace and moves it into the ring buffer.

```go
func (*Tracer) Finish(trace *Trace)
```

### Live

Live returns the traces currently in flight, newest first.

```go
func (*Tracer) Live() []Trace
```

### Middleware

Middleware records requests handled by next.

```go
func (*Tracer) Middleware(next http.Handler) http.Handler
```

### Observe

Observe runs fn inside its own trace, records the returned error and
completes the trace. It is what background jobs and cron ticks should use.

```go
func (*Tracer) Observe(ctx context.Context, name string, fn func(context.Context) error) error
```

### Options

Options returns the options the tracer was built with.

```go
func (*Tracer) Options() Options
```

### ReportError

ReportError forwards a failure to Options.OnError. The front end reports
render failures through it, so every failure of one tracer arrives in one
place. Nothing is written to stdout or stderr.

```go
func (*Tracer) ReportError(err error)
```

### Reset

Reset drops every retained trace and the lifetime counters. Traces in flight
are left alone and are recorded when they complete.

```go
func (*Tracer) Reset()
```

### SetEnabled

SetEnabled turns recording on or off at runtime. Retained traces are kept.

```go
func (*Tracer) SetEnabled(enabled bool)
```

### Snapshot

Snapshot returns a race free copy of the tracer state. Nothing in the result
aliases live state.

```go
func (*Tracer) Snapshot() Snapshot
```

### StartTrace

StartTrace begins a trace for work that does not arrive over HTTP. The caller
must complete it with Finish.

```go
func (*Tracer) StartTrace(ctx context.Context, name string) (context.Context, *Trace, error)
```

### Storage

Storage returns the storage backing the tracer.

```go
func (*Tracer) Storage() Storage
```

### Subscribe

Subscribe returns a channel notified whenever a trace starts or completes,
and a function releasing it. The live view streams from this.

```go
events, cancel := tracer.Subscribe()
defer cancel()
for range events {
	render(tracer.Live())
}
```

Notifications are coalesced, so a slow consumer cannot slow down recording.

```go
func (*Tracer) Subscribe() (<-chan struct{}, func())
```

### Trace

Trace returns the retained or in flight trace with the given ID.

```go
func (*Tracer) Trace(id string) (Trace, bool)
```

### Traces

Traces returns the retained traces, newest first.

```go
func (*Tracer) Traces() []Trace
```

### Authorized

Authorized reports whether r may access the debug front end. The front end
asks before it serves anything, including its assets.

```go
func (Options) Authorized(r *http.Request) bool
```

### Validate

Validate reports whether the options are usable. Every failure wraps
ErrInvalidOptions.

```go
func (Options) Validate() error
```

### WithDefaults

WithDefaults returns a usable copy of the options. Options created by
NewOptions preserve explicit zero values; an uninitialized Options receives
the numeric defaults for backward compatibility.

```go
func (Options) WithDefaults() Options
```

### Sample

Sample implements Sampler.

```go
func (SamplerFunc) Sample(r *http.Request) bool
```
