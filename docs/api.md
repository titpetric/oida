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

tracer, err := oida.New(opts)
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

Mount registers the front end on either router, adding the subtree patterns
each one understands:

```go
if err := oida.Mount(mux, tracer); err != nil {
	return err
}
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

The project is four packages. This one records and serves: the tracer, the
middleware and the options. Package model holds the recorded data and the
configuration and depends on nothing; the types it defines are aliased here,
so instrumenting a service needs this import alone. Package storage holds
the retention drivers, which New builds from the environment.
Package frontend renders the dashboard, reads the model alone, and is
imported here so the tracer can serve it.

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
<summary><code>type LogEntry</code></summary>

```go
// LogEntry is one log line recorded on a trace by Trace.Info, Trace.Error and
// their Span counterparts. No context is involved: a trace attributes the
// entry to its innermost open span, and a span uses its own id.
type LogEntry = model.LogEntry
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
<summary><code>type Router</code></summary>

```go
// Router is the one method chi and the standard library share: chi.Router,
// *chi.Mux and *http.ServeMux all register handlers with Handle, so one
// interface mounts the front end on either, and oida depends on neither.
type Router interface {
	Handle(pattern string, h http.Handler)
}
```

</details>

<details>
<summary><code>type RouterFunc</code></summary>

```go
// RouterFunc adapts a registration function to Router, for a router whose own
// Handle does not fit:
//
//	oida.Mount(oida.RouterFunc(func(pattern string, h http.Handler) {
//		r.PathPrefix(pattern).Handler(h)
//	}), tracer)
//
// gorilla returns a *mux.Route from Handle and matches its paths exactly, so
// its dashboard is registered by prefix.
type RouterFunc func(pattern string, h http.Handler)
```

</details>

<details>
<summary><code>type Sampler</code></summary>

```go
// Sampler decides whether a request is traced. The decision is taken before a
// trace is allocated, so rejecting a request costs one interface call. The
// definition is a copy of the model's; interfaces are structural, so a sampler
// written against either spelling works everywhere one is accepted.
//
// Options.SampleRate covers the common case. Set Options.Sampler to decide per
// request on something the rate cannot express, such as a header or a route.
type Sampler interface {
	Sample(r *http.Request) bool
}
```

</details>

<details>
<summary><code>type Storage</code></summary>

```go
// Storage is what a storage driver implements to retain completed traces.
// Implementations must be safe for concurrent use: the tracer writes from
// request goroutines and reads from the debug front end at the same time.
//
// Two drivers ship with the package, a bounded memory ring buffer and a
// bounded folder of JSON documents, and Configure builds either one from the
// environment. Set this field to retain traces somewhere else.
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

	// Prune drops retained traces older than maxAge. A driver with nothing
	// to prune returns nil.
	Prune(ctx context.Context, maxAge time.Duration) error

	// Restore fills the read path from what the driver persisted, so a new
	// process can list what an earlier one recorded. A driver holding nothing
	// of its own returns nil.
	Restore(ctx context.Context) error
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
```

</details>

<details>
<summary><code>type Trace, Span, Attributes, Kind, State, Snapshot</code></summary>

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

	// Snapshot is the complete read model of a tracer at one point in time,
	// which is what the dashboard renders and what Tracer.Snapshot returns.
	Snapshot = model.Snapshot
)
```

</details>

## Consts

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
const RequestIDHeader = model.RequestIDHeader
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

<details>
<summary><code>const LevelInfo, LevelError</code></summary>

```go
// Log levels recorded by Trace.Info and Trace.Error.
const (
	LevelInfo  = model.LevelInfo
	LevelError = model.LevelError
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
<summary><code>var ErrNilRouter, ErrNoTracer, ErrInvalidOptions, ErrInvalidPath, ErrInvalidSampleRate, ErrTraceNotFound, ErrDisabled</code></summary>

```go
// The errors this package returns. Every configuration failure wraps
// ErrInvalidOptions, so a caller can test for the class or for the case. The
// values live in the model package so the front end can return them too; these
// are the same error values, so errors.Is works with either spelling.
var (
	// ErrNilRouter is returned when Mount is called without a router.
	ErrNilRouter = model.ErrNilRouter

	// ErrNoTracer is returned when Mount is called without a tracer, which is
	// a dashboard with nothing to show.
	ErrNoTracer = model.ErrNoTracer

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

- `func Do (ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error`
- `func Mount (r Router, t *Tracer) error`
- `func New (opts Options) (*Tracer, error)`
- `func NewAuth (opts Options) (*Auth, error)`
- `func NewOptions (serviceName string) Options`
- `func RecordError (ctx context.Context, err error)`
- `func SpanFromContext (ctx context.Context) *Span`
- `func Start (ctx context.Context, name string, kind ...Kind) (context.Context, *Span)`
- `func StartAuto (ctx context.Context, symbol any, kind ...Kind) (context.Context, *Span)`
- `func StartRequest (r *http.Request, name string, kind ...Kind) (*http.Request, *Span)`
- `func StartSpan (ctx context.Context, name string, kind ...Kind) *Span`
- `func TraceFromContext (ctx context.Context) *Trace`
- `func TraceID (ctx context.Context) string`
- `func WithTrace (ctx context.Context, t *Trace) context.Context`
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
- `func (*Tracer) Subscribe () (<-chan struct{}, func())`
- `func (*Tracer) Trace (id string) (Trace, bool)`
- `func (*Tracer) Traces () []Trace`
- `func (RouterFunc) Handle (pattern string, h http.Handler)`

### Do

Do runs fn inside a span, records the returned error on it and ends it. The
error is returned unchanged.

```go
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error
```

### Mount

Mount registers the debug front end of t on r, under the path t was
configured with. Mounting the tracer itself, r.Handle(path, tracer), is
equivalent; this call adds the patterns each router uses to serve a subtree.

Three patterns are registered: the bare path, the trailing slash form that
is the subtree on a ServeMux, and the /* wildcard that is the subtree on
chi. Each router uses the ones it understands.

It returns an error when r or t is nil.

```go
func Mount(r Router, t *Tracer) error
```

### New

New returns a tracer built from opts. Nothing is stored in a package level
variable: the tracer a request records into is the one in its context, and
the tracer an entry point uses is the one handed to it.

With Options.ReadEnv set, which is what NewOptions returns, the OIDA_*
environment is applied to opts first. A variable applies only where the code
left the field at its default, so options set in code win over the
environment, and a variable set to nothing leaves the default alone. The
configuration guide lists them.

```go
func New(opts Options) (*Tracer, error)
```

### NewAuth

NewAuth builds the authentication state out of the options, or nil when no
authentication option is set: no allow list, no users and no signing secret
leaves the front end open. Session mints a token from it, which is how a
deployment issues one for a job that reads the dashboard API.

```go
func NewAuth(opts Options) (*Auth, error)
```

### NewOptions

NewOptions returns the default options for the named service.

```go
func NewOptions(serviceName string) Options
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
gives names like billing.UserStore.GetUsers without spelling them out.

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

### TraceID

TraceID returns the identifier of the trace in ctx, or an empty string. It is
the value of the Request-Id header for HTTP traces, which makes it the
cheapest correlation key for logs.

```go
func TraceID(ctx context.Context) string
```

### WithTrace

WithTrace returns a context carrying the trace. Spans started from the
returned context, or any context derived from it, are recorded on it.

```go
func WithTrace(ctx context.Context, t *Trace) context.Context
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

Middleware records every sampled request handled by next. It is compatible
with chi's Use, with alice, and with any func(http.Handler) http.Handler
chain:

```go
r.Use(tracer.Middleware)
```

A nil tracer passes every request through, so instrumented wiring runs
unchanged in a process that built none.

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

Options returns the options the tracer was built with, as a copy the caller
owns. The retention driver is left out and the list and map are cloned: a
reader of the configuration has no business reaching the storage behind it
or rewriting what the tracer runs on.

```go
func (*Tracer) Options() Options
```

### ReportError

ReportError forwards a failure to Options.OnError, which is where the front
end reports its render failures too. Nothing is written to stdout or stderr.

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

Trace returns the retained or in flight trace with the given ID. A retained
trace is read only, the way Traces returns them; an in flight one is a copy
the caller owns.

```go
func (*Tracer) Trace(id string) (Trace, bool)
```

### Traces

Traces returns the retained traces, newest first. The result is read only:
its spans are the ones the front end renders, so recording into them is not
a caller's to do.

```go
func (*Tracer) Traces() []Trace
```

### Handle

Handle implements Router.

```go
func (RouterFunc) Handle(pattern string, h http.Handler)
```
