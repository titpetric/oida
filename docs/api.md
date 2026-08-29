# Package oida

```go
import (
	"github.com/titpetric/oida"
}
```

Package oida records in-process telemetry: traces and spans held in a ring
buffer inside the process, with a server side rendered front end mounted at
/debug/oida.

Wire it into a service in three calls: configure the tracer, mount it, add
the middleware. Recording is opt-in: enable it in code, or leave the field
alone and set OIDA_ENABLED=true in the environment. The tracer is an
http.Handler serving the debug front end, so it mounts like any other
handler and no second import is needed:

```go
opts := oida.NewOptions("billing-api")
opts.Enabled = true

tracer, err := oida.Configure(opts)
if err != nil {
	return err
}

mux := http.NewServeMux()
mux.Handle("/debug/oida/", tracer)
```

A chi router mounts the same tracer with its own call:

```go
r := chi.NewRouter()
r.Mount("/debug/oida", tracer)
```

The middleware records every sampled request into the tracer:

```go
handler := tracer.Middleware(mux)
return http.ListenAndServe(":8080", handler)
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

The project is three packages. This one records and serves: the tracer, the
middleware, the options and the storage. Package model holds the recorded
data and the configuration and depends on nothing; the types it defines are
aliased here, so instrumenting a service needs this import alone. Package
frontend renders the dashboard, reads the model alone, and is imported here
so the tracer can serve it.

Nothing in this package writes to stdout or stderr. Storage and rendering
failures are reported through Options.OnError.

## Types

<details>
<summary><code>type Auth</code></summary>

```go
// Auth evaluates the authentication options: the network allow list, the
// configured users, and the token verification behind the session cookie and
// the Authorization header. It lives in the model package so the front end
// can enforce it; the alias keeps it spelled oida.Auth.
type Auth = model.Auth
```

</details>

<details>
<summary><code>type Options</code></summary>

```go
// Options configures telemetry behaviour, the debug front end and the
// middleware. It lives in the model package so the front end can read it
// without depending on the recorder; the alias keeps it spelled oida.Options.
type Options = model.Options
```

</details>

<details>
<summary><code>type Recorder</code></summary>

```go
// Recorder is the substitutable surface of Tracer: the write side the
// instrumentation records through, and the read side the debug front end
// renders from. Code that only needs to record and read back traces can depend
// on this interface instead of the concrete tracer.
type Recorder = model.Recorder
```

</details>

<details>
<summary><code>type Sampler</code></summary>

```go
// Sampler decides whether a request is traced. The decision is taken before a
// trace is allocated, so rejecting a request costs one interface call.
type Sampler = model.Sampler
```

</details>

<details>
<summary><code>type SamplerFunc</code></summary>

```go
// SamplerFunc adapts a function to the Sampler interface.
type SamplerFunc = model.SamplerFunc
```

</details>

<details>
<summary><code>type Storage</code></summary>

```go
// Storage retains completed traces. Implementations must be safe for concurrent
// use: the tracer writes from request goroutines and reads from the debug front
// end at the same time.
//
// Two implementations ship with this package: StorageMemory, a bounded ring
// buffer, and StorageDisk, a bounded folder of JSON documents.
type Storage = model.Storage
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
const DefaultPath = model.DefaultPath
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
<summary><code>const SessionCookie</code></summary>

```go
// SessionCookie is the name of the front end session cookie.
const SessionCookie = model.SessionCookie
```

</details>

<details>
<summary><code>const SessionTTL</code></summary>

```go
// SessionTTL is how long an issued session token stays valid.
const SessionTTL = model.SessionTTL
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
<summary><code>var ErrInvalidCredentials, ErrInvalidToken</code></summary>

```go
// The errors authentication returns, aliased like the other error values so
// errors.Is works with either spelling.
var (
	// ErrInvalidCredentials is returned when a login does not match any
	// configured user.
	ErrInvalidCredentials = model.ErrInvalidCredentials

	// ErrInvalidToken is returned when a session cookie or bearer token does
	// not verify against the signing secret, or has expired.
	ErrInvalidToken = model.ErrInvalidToken
)
```

</details>

<details>
<summary><code>var ErrNilRouter, ErrInvalidOptions, ErrInvalidPath, ErrInvalidSampleRate, ErrTraceNotFound, ErrDisabled</code></summary>

```go
// The errors this package returns. Every configuration failure wraps
// ErrInvalidOptions, so a caller can test for the class or for the case. The
// values live in the model package so the front end can return them too; these
// are the same error values, so errors.Is works with either spelling.
var (
	// ErrNilRouter is returned when Mount is called without a router.
	ErrNilRouter = model.ErrNilRouter

	// ErrInvalidOptions is the base error for every configuration failure.
	ErrInvalidOptions = model.ErrInvalidOptions

	// ErrInvalidPath is returned when Options.Path is not an absolute path.
	ErrInvalidPath = model.ErrInvalidPath

	// ErrInvalidSampleRate is returned when Options.SampleRate is outside
	// [0,100].
	ErrInvalidSampleRate = model.ErrInvalidSampleRate

	// ErrTraceNotFound is returned when a trace ID is not in the ring buffer.
	ErrTraceNotFound = model.ErrTraceNotFound

	// ErrDisabled is returned when a trace is requested from a disabled tracer.
	ErrDisabled = model.ErrDisabled
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
- `func NewAuth (opts Options) (*Auth, error)`
- `func NewOptions (serviceName string) Options`
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
- `func (*Tracer) ServeHTTP (w http.ResponseWriter, r *http.Request)`
- `func (*Tracer) SetEnabled (enabled bool)`
- `func (*Tracer) Snapshot () Snapshot`
- `func (*Tracer) StartTrace (ctx context.Context, name string) (context.Context, *Trace, error)`
- `func (*Tracer) Storage () Storage`
- `func (*Tracer) Subscribe () (<-chan struct{}, func())`
- `func (*Tracer) Trace (id string) (Trace, bool)`
- `func (*Tracer) Traces () []Trace`

### Configure

Configure replaces the process wide tracer with one built from opts and
returns it. Call it once during startup, before wiring the middleware.

Configure also applies the environment: every OIDA_* variable listed on
optionsFromEnv, including the OIDA_AUTH=username:password sign in opt-in.
A variable applies only where the code left the field at its default, so
options set in code win over the environment.

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

### NewAuth

NewAuth builds the authentication state out of the options, or nil when no
authentication option is set. See model.NewAuth.

```go
func NewAuth(opts Options) (*Auth, error)
```

### NewOptions

NewOptions returns the default options for the named service.

```go
func NewOptions(serviceName string) Options
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

### ServeHTTP

ServeHTTP serves the debug front end of the tracer, so a tracer mounts like
any other handler:

```go
mux := http.NewServeMux()
mux.Handle("/debug/oida/", tracer)

r := chi.NewRouter()
r.Mount("/debug/oida", tracer)
```

A path that does not start with Options.Path is treated as already relative,
the shape http.StripPrefix delivers. A nil tracer serves 404, not a panic.

```go
func (*Tracer) ServeHTTP(w http.ResponseWriter, r *http.Request)
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
