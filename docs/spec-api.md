# Public API

One import path: `github.com/titpetric/oida`. The root package is the public API and [api.md](api.md) is its generated reference. The `model`, `frontend` and `storage` packages serve it and carry no compatibility promise of their own, so an integration never has to name one.

Most services use `NewOptions`, `New`, `Mount`, `Start` and the tracer's own `Middleware`.

## Setup

```go
func NewOptions(serviceName string) Options
func New(opts Options) (*Tracer, error)

func (t *Tracer) Middleware(next http.Handler) http.Handler

func (t *Tracer) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

`*Tracer` implements `http.Handler` and serves the dashboard, so mounting it needs no second import. `Mount` registers it under the patterns a router serves a subtree with:

```go
func Mount(r Router, t *Tracer) error
```

It wraps the `frontend` package, which renders the dashboard and exposes `HandlerFor` for the root package to build the handler with.

`New` is the only constructor. There is no process wide tracer: the tracer it returns is what the middleware and the dashboard are wired to, by hand rather than through the options.

```go
opts := oida.NewOptions("billing-api")
tracer, err := oida.New(opts)
if err != nil {
	return err
}

r.Use(tracer.Middleware)
oida.Mount(r, tracer)
```

`Options.ReadEnv`, which `NewOptions` turns on, applies the `OIDA_*` environment to the options inside `New`. Options built as a literal leave it off, which is what a library or a test wants.

`Mount` accepts routers with this method, the one `chi.Router` and `*http.ServeMux` share:

```go
type Router interface {
	Handle(pattern string, h http.Handler)
}
```

Three patterns are registered: the bare path, the trailing slash form that is the subtree on a ServeMux, and the `/*` wildcard that is the subtree on chi; each router uses the ones it understands. Mounting the tracer itself is the shortest path to a dashboard, and takes its configuration from the tracer.

The middleware skips disabled, ignored, and unsampled requests. Traced HTTP requests receive a `Request-Id` header. When `TrustRequestID` is true, a valid client-supplied trace ID is reused. Panics are recorded and then passed on to the application's recovery middleware.

## Instrumentation

```go
func Start(ctx context.Context, name string, kind ...Kind) (context.Context, *Span)
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span
func StartAuto(ctx context.Context, symbol any, kind ...Kind) (context.Context, *Span)
func StartRequest(r *http.Request, name string, kind ...Kind) (*http.Request, *Span)
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error
func RecordError(ctx context.Context, err error)

func TraceFromContext(ctx context.Context) *Trace
func SpanFromContext(ctx context.Context) *Span
func TraceID(ctx context.Context) string
func WithTrace(ctx context.Context, t *Trace) context.Context
```

`Start` returns a context that should be passed to nested work:

```go
ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
defer span.End()

rows, err := db.QueryContext(ctx, query)
```

`StartSpan` is for leaf work that does not create child spans. `Do` records the returned error, ends the span, and returns the same error.

`StartAuto` reads the span name from a symbol, so `storage.UserStorage.GetUsers` does not have to be spelled out:

```go
ctx, span := oida.StartAuto(ctx, s.GetUsers)
defer span.End()
```

The name comes from reflection and the runtime symbol table. It does not survive a stripped binary and reads oddly for anonymous functions, so use `Start` where either matters.

`StartRequest` is `Start` for a handler holding an `*http.Request`:

```go
r, span := oida.StartRequest(r, "user.Handler")
defer span.End()
```

`RecordError` records an error on the innermost span in a context, without holding the span:

```go
if err := store.Save(ctx, u); err != nil {
	oida.RecordError(ctx, err)
	return err
}
```

When the context has no trace, these functions return nil spans, return the request unchanged, and record nothing. Span and trace methods are nil-safe.

### Span methods

```go
func (s *Span) End()
func (s *Span) EndWithError(err error)
func (s *Span) RecordError(err error)
func (s *Span) Info(message string, args ...any)
func (s *Span) Error(message string, args ...any)
func (s *Span) SetAttribute(key string, value any)
func (s *Span) SetAttributes(attributes Attributes)
func (s *Span) SetName(name string)
func (s *Span) SetSource(filename string, line int)
func (s *Span) Err() error
func (s *Span) Ended() bool
func (s *Span) Elapsed() time.Duration
func (s *Span) Context(ctx context.Context) context.Context
func (s *Span) Trace() *Trace
```

`End` is idempotent. `RecordError` marks both the span and trace as failed. `Info` and `Error` record log entries on the trace of the span, linked to the span; see [the instrumentation guide](guide-instrumentation.md).

### Trace methods

The trace API matches the span API where the two mean the same thing:

```go
func (t *Trace) RecordError(err error)
func (t *Trace) Err() error
func (t *Trace) Info(message string, args ...any)
func (t *Trace) Error(message string, args ...any)
func (t *Trace) Current() *Span
func (t *Trace) Root() *Span
func (t *Trace) SetAttribute(key string, value any)
func (t *Trace) SetAttributes(attributes Attributes)
func (t *Trace) Attribute(key string) (any, bool)
func (t *Trace) SetName(name string)
func (t *Trace) SetState(state State)
func (t *Trace) SpanCount() int
func (t *Trace) Elapsed() time.Duration
```

`RecordError` records the error and moves the trace to `StateError`; a nil error is ignored. `Err` returns the error that was recorded, the value itself, so `errors.Is` and `errors.As` work on it:

```go
if err := trace.Err(); errors.Is(err, context.Canceled) {
	// the caller went away before the work finished
}
```

`Span.RecordError` records on the span and then on its trace, so an error recorded anywhere in a transaction is readable from both. A trace or span decoded from JSON kept the message and not the value, and reports an error carrying that message.

`Info` and `Error` record log entries linked to the innermost open span, which is what `Current` returns; `Root` returns the first recorded span. With `Options.CaptureLogs` off, `Info` does nothing and `Error` records through `RecordError` instead.

Trace attributes are for what holds for the whole transaction, such as the memory limit it ran under. What holds for one operation belongs on the span that measured it. See [the data model](spec-model.md#attributes) for the keys the front end knows.

## Background work

```go
func (t *Tracer) StartTrace(ctx context.Context, name string) (context.Context, *Trace, error)
func (t *Tracer) Finish(trace *Trace)
func (t *Tracer) Observe(ctx context.Context, name string, fn func(context.Context) error) error
```

Use `Observe` for a job or cron tick:

```go
err := tracer.Observe(ctx, "refresh search index", func(ctx context.Context) error {
	return index.Refresh(ctx)
})
```

Use `StartTrace` when the caller needs control over completion, and always call `Finish`.

## Reading and controlling a tracer

```go
func (t *Tracer) Options() Options
func (t *Tracer) Enabled() bool
func (t *Tracer) SetEnabled(enabled bool)
func (t *Tracer) Snapshot() Snapshot
func (t *Tracer) Traces() []Trace
func (t *Tracer) Trace(id string) (Trace, bool)
func (t *Tracer) Live() []Trace
func (t *Tracer) Reset()
func (t *Tracer) Subscribe() (<-chan struct{}, func())
```

`Traces` and `Live` return newest first. `Trace` searches retained and active traces. `Reset` clears retained traces and counters but leaves active traces running. `Subscribe` can notify another view when trace activity changes.

## Sampling

```go
type Sampler interface {
	Sample(r *http.Request) bool
}

type SamplerFunc func(r *http.Request) bool

func NewRateSampler(rate float64) Sampler
```

Set `Options.SampleRate` for rate sampling or `Options.Sampler` for custom rules. A rate of zero records no HTTP requests; a rate of one records all of them.

## Retention

```go
type Storage interface {
	Save(ctx context.Context, trace Trace) error
	Load(ctx context.Context, id string) (Trace, error)
	List(ctx context.Context, limit int) ([]Trace, error)
	Len(ctx context.Context) (int, error)
	Cap() int
	Reset(ctx context.Context) error
	Prune(ctx context.Context, maxAge time.Duration) error
	Restore(ctx context.Context) error
}
```

The two drivers live in the storage package and are not constructed by hand: `New` builds one from the environment and assigns it to `Options.Storage`. Memory storage retains traces in a ring buffer and is the default, sized by `RingBufferSize`; a size of zero retains none.

Disk storage is a write-through overlay of memory storage: a save goes to a JSON document and to the ring, and every read comes from the ring, so listing costs no disk access. `Load` falls back to the document folder for a trace the ring no longer holds. The ring holds what the running process recorded, so the dashboard lists its own traces, while the folder outlives the process: its documents stay reachable by ID and are aged out by `Prune`. `Prune` ages the archive out and leaves the ring alone: the ring is the recent window with its own size policy. `Restore` is the other direction, reading the newest documents back into the ring so a run opens on what earlier ones recorded. The memory driver has nothing to prune and nothing of its own to restore, and returns nil to both. A driver of your own implements the interface, returning nil from the methods it has nothing to do; embedding `oida.Storage` in the struct keeps it compiling when the interface grows, at the cost of a panic if something calls a method it never wrote.

`OIDA_STORAGE_DRIVER` chooses between them, and the `OIDA_STORAGE_MEMORY_` and `OIDA_STORAGE_DISK_` variables carry their settings. A path that cannot be created, a driver name that is not one of the two, and settings addressed to the driver that was not chosen all fail `New` rather than being dropped. The [configuration guide](guide-configuration.md) lists the variables.

## Errors

| Error                  | Meaning                                          |
|------------------------|--------------------------------------------------|
| `ErrNilRouter`         | `Mount` received a nil router                    |
| `ErrInvalidOptions`    | Base error for invalid configuration             |
| `ErrInvalidPath`       | `Options.Path` is not absolute                   |
| `ErrInvalidSampleRate` | `Options.SampleRate` is outside `[0,100]` or NaN |
| `ErrTraceNotFound`     | A storage lookup did not find the trace          |
| `ErrDisabled`          | `StartTrace` was called on a disabled tracer     |

Negative `RingBufferSize`, `TopRequests`, `MaxSpansPerTrace`, or `RefreshInterval` values wrap `ErrInvalidOptions`.

Storage and dashboard errors are sent to `Options.OnError` when it is set. `Observe` still runs its function without tracing when the tracer is disabled.

## Test server

The `github.com/titpetric/oida/tests` package provides an instrumented HTTP server with sample success, failure, cache, database, and external-call routes.

```go
func NewServer(t testing.TB) http.Handler
func NewServerWithTracer(t testing.TB) (http.Handler, *oida.Tracer)
func NewHTTPServer(t testing.TB) *httptest.Server

const Path = "/debug/oida"
```

```go
func TestTraces(t *testing.T) {
	server := tests.NewHTTPServer(t)

	response, err := server.Client().Get(server.URL + "/users/42")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
}
```
