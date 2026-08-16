# Specification: public API

Import path: `github.com/titpetric/oida`.

The API has four layers. Most services only use layer 1 and 2.

1. **Wiring** — `Mount`, `TracingMiddleware`, `Handler`, `Options`.
2. **Instrumentation** — `Start`, `Span`, `Trace`, context accessors.
3. **Explicit tracers** — `New`, `Tracer`, `Recorder`, for processes that want
   more than one recorder or full control over lifetime.
4. **Retention** — `Storage`, `StorageMemory`, `StorageDisk`.

## 1. Wiring

```go
func New(opts Options) (*Tracer, error)
func Configure(opts Options) (*Tracer, error)
func Default() *Tracer

func TracingMiddleware(opts Options) func(http.Handler) http.Handler
func Handler(opts Options) http.Handler
func Mount(r Router, opts Options) error
```

### 1.1 Tracer resolution

`TracingMiddleware`, `Handler` and `Mount` all need to talk to the *same*
tracer, otherwise the UI shows an empty ring buffer. Resolution order:

1. `opts.Tracer != nil` → use it. This is the explicit, recommended form for
   libraries, tests and multi-tracer processes.
2. Otherwise → the process-wide default tracer. The first call that resolves the
   default configures it from `opts`; later calls reuse it and ignore their
   options for tracer construction (they still use `opts.Path`, `opts.Authorize`
   and the other request-scoped fields).

`Configure(opts)` replaces the default tracer explicitly and returns it, so the
usual pattern is:

```go
tracer, err := oida.Configure(opts)   // once, at startup
if err != nil {
    return err
}
opts.Tracer = tracer                  // optional, makes the wiring explicit
```

### 1.2 `Mount`

```go
type Router interface {
    Mount(pattern string, h http.Handler)
}

func Mount(r Router, opts Options) error
```

`Router` is a structural interface satisfied by `chi.Router` (and by
`*chi.Mux`), so oida does not depend on chi. `Mount`:

1. Applies defaults to `opts` and validates them.
2. Resolves the tracer.
3. Calls `r.Mount(opts.Path, Handler(opts))`.

Errors:

| Condition | Error |
| --- | --- |
| `r == nil` | `ErrNilRouter` |
| `opts.Path` is not an absolute path | `ErrInvalidPath` |
| `opts.RingBufferSize < 0`, `TopRequests < 0`, `MaxSpansPerTrace < 0` | `ErrInvalidOptions` |
| `opts.SampleRate` outside `[0,1]` | `ErrInvalidSampleRate` |

All errors wrap `ErrInvalidOptions` except `ErrNilRouter`, so callers can test
with `errors.Is(err, oida.ErrInvalidOptions)`.

`net/http`'s `*http.ServeMux` does not satisfy `Router` (its method is `Handle`,
not `Mount`), so a `MountServeMux` helper is provided:

```go
func MountServeMux(mux *http.ServeMux, opts Options) error
```

It registers `opts.Path` and `opts.Path + "/"`.

### 1.3 `Handler`

```go
func Handler(opts Options) http.Handler
```

Returns the `/debug/oida` handler. It is mount-point agnostic: it strips
`opts.Path` from the incoming path itself and also honours the prefix stripped
by `chi.Mount`, so the same handler works under `r.Mount`, `mux.Handle` and a
direct `http.Handle`. Invalid options degrade to defaults rather than panicking;
use `Mount` or `New` when you want the error.

### 1.4 `TracingMiddleware`

```go
func TracingMiddleware(opts Options) func(http.Handler) http.Handler
```

Compatible with `chi`'s `r.Use`, `alice`, and any `func(http.Handler)
http.Handler` chain. Per request it:

1. Skips instrumentation entirely when the tracer is disabled, when the path
   matches `opts.Path` or `opts.IgnorePaths`, or when the sampler rejects it.
2. Generates a ULID, sets it on the `Request-Id` request and response headers
   (a client-supplied `Request-Id` is preserved when `opts.TrustRequestID` is
   set).
3. Creates the trace, stores it in the request context, opens the root
   `KindHTTP` span.
4. Wraps the `http.ResponseWriter` to capture status and byte count while
   preserving `http.Flusher`, `http.Hijacker` and `io.ReaderFrom` through
   `Unwrap`.
5. Recovers panics: records the panic on the trace, marks it `StateError`, and
   re-panics so the outer recovery middleware still sees it.
6. Completes the trace and pushes it into the ring buffer.

## 2. Instrumentation

```go
func Start(ctx context.Context, name string, kind ...Kind) (context.Context, *Span)
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error

func TraceFromContext(ctx context.Context) *Trace
func SpanFromContext(ctx context.Context) *Span
func TraceID(ctx context.Context) string
func WithTrace(ctx context.Context, t *Trace) context.Context
```

- `Start` is the primary API. The returned context carries the new span as the
  parent for nested `Start` calls.
- `StartSpan` is the convenience form for leaf spans that will not nest.
- `Do` runs `fn` in a child span, records the returned error on it and ends it —
  the error is returned unchanged.
- All of these are no-ops returning a nil `*Span` when `ctx` has no trace.

`Tracer` provides the same operations bound to an explicit recorder, for
non-HTTP work:

```go
func (t *Tracer) StartTrace(ctx context.Context, name string) (context.Context, *Trace, error)
func (t *Tracer) Observe(ctx context.Context, name string, fn func(context.Context) error) error
```

`StartTrace` returns a trace that the caller must complete with
`t.Finish(trace)`; `Observe` does both around `fn` and is what background jobs
should use.

## 3. Tracer

```go
type Tracer struct { /* unexported */ }

func (t *Tracer) Options() Options
func (t *Tracer) Enabled() bool
func (t *Tracer) SetEnabled(bool)
func (t *Tracer) Middleware(next http.Handler) http.Handler
func (t *Tracer) Handler() http.Handler
func (t *Tracer) Mount(r Router) error
func (t *Tracer) Storage() Storage
func (t *Tracer) Subscribe() (<-chan struct{}, func())
func (t *Tracer) Snapshot() Snapshot
func (t *Tracer) Traces() []Trace
func (t *Tracer) Trace(id string) (Trace, bool)
func (t *Tracer) Live() []Trace
func (t *Tracer) Reset()
func (t *Tracer) Finish(tr *Trace)
```

`Recorder` is the substitutable subset:

```go
type Recorder interface {
    StartTrace(ctx context.Context, name string) (context.Context, *Trace, error)
    Finish(t *Trace)
    Snapshot() Snapshot
}
```

`Subscribe` returns a channel woken whenever a trace starts or completes, plus
an idempotent release function. Sends are non-blocking on a one-slot buffer, so
a slow consumer coalesces updates instead of slowing down recording. The live
view's event stream is built on it, and so can anything else:

```go
events, cancel := tracer.Subscribe()
defer cancel()
for range events {
	render(tracer.Live())
}
```

Counter semantics on `Snapshot`:

| Field | Meaning |
| --- | --- |
| `Total` | Units of work seen: every sampled trace plus every request the sampler rejected |
| `Sampled` | Traces actually created, HTTP and background alike |
| `Dropped` | Rejected by the sampler, plus traces rotated out of storage |
| `Active` | Traces in flight right now |

## 4. Storage

```go
type Storage interface {
    Save(ctx context.Context, trace Trace) error
    Load(ctx context.Context, id string) (Trace, error)   // ErrTraceNotFound
    List(ctx context.Context, limit int) ([]Trace, error) // newest first, 0 = all
    Len(ctx context.Context) (int, error)
    Cap() int
    Reset(ctx context.Context) error
}

func NewStorageMemory(size int) *StorageMemory
func NewStorageDisk(limit int, paths ...string) (*StorageDisk, error)
func (s *StorageDisk) Path() string
func (s *StorageDisk) Prune(ctx context.Context, maxAge time.Duration) error
```

- `StorageMemory` is the default, built from `Options.RingBufferSize` when
  `Options.Storage` is nil. A size of zero retains nothing.
- `StorageDisk` writes one JSON document per trace, named after the trace ID,
  and prunes the oldest documents past `limit`. Trace IDs are ULIDs, so the
  folder sorts chronologically. IDs are validated before they touch the
  filesystem, so a hostile ID cannot escape the folder.
- Implementations must be safe for concurrent use. The tracer never holds its
  own lock while calling into storage.
- `Save` receives a completed, inert `Trace`: cloning happens in the tracer, so
  an implementation may retain the value.

## 5. Errors

```go
var (
    ErrNilRouter         = errors.New("oida: router is nil")
    ErrInvalidOptions    = errors.New("oida: invalid options")
    ErrInvalidPath       = fmt.Errorf("%w: path must be an absolute path", ErrInvalidOptions)
    ErrInvalidSampleRate = fmt.Errorf("%w: sample rate must be between 0 and 1", ErrInvalidOptions)
    ErrTraceNotFound     = errors.New("oida: trace not found")
    ErrDisabled          = errors.New("oida: tracer is disabled")
)
```

Contract:

- Constructors and mounting return errors; instrumentation never does.
- Errors that surface asynchronously — a storage write that failed, a component
  that failed to render — go to `Options.OnError`. The package writes to neither
  stdout nor stderr, so a nil `OnError` discards them.
- `Tracer.StartTrace` returns `ErrDisabled` when the tracer is off;
  `Tracer.Observe` swallows that and simply runs the function untraced.
- `Start`, `Span` methods and `Trace` methods never panic and never return
  errors — they degrade to no-ops.
- The HTTP handler never returns 5xx for missing data: an unknown trace ID is a
  404, an unauthorized request is a 404 (not a 401, so the endpoint's existence
  is not advertised).

## 6. Test helper

`github.com/titpetric/oida/tests` wires a complete instrumented service for use
in tests and examples. It is the only package in the module that depends on chi.

```go
func NewServer(t testing.TB) http.Handler
func NewServerWithTracer(t testing.TB) (http.Handler, *oida.Tracer)
func NewHTTPServer(t testing.TB) *httptest.Server
const Path = "/debug/oida"
```

The server uses `oida.NewStorageMemory(64)`, samples everything, resolves chi
route patterns through `Options.RouteFunc`, fails the test on any `OnError`
callback, and serves `/`, `/users/{id}`, `/slow` and `/fail` — routes that
record cache, database, external, template and error spans.

```go
func TestSomething(t *testing.T) {
	server := tests.NewHTTPServer(t)

	response, err := server.Client().Get(server.URL + "/users/42")
	...
}
```

## 7. Stability

Exported names in this document are the supported surface. Everything else is
unexported. Struct fields of the data models may gain new fields; existing JSON
field names will not be renamed within a major version.
