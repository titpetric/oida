# Public API

Three import paths:

| Package    | Import path                          | Contents                                                                          |
|------------|--------------------------------------|-----------------------------------------------------------------------------------|
| `oida`     | `github.com/titpetric/oida`          | Tracer, middleware, options, storage, instrumentation                             |
| `frontend` | `github.com/titpetric/oida/frontend` | The dashboard: `Handler`, `Mount`, `MountServeMux`                                |
| `model`    | `github.com/titpetric/oida/model`    | Recorded data. Its types are aliased into `oida`, so this import is rarely needed |

Most services use `NewOptions`, `Configure`, `TracingMiddleware`, `frontend.Mount`, and `Start`.

## Setup

```go
func NewOptions() Options
func New(opts Options) (*Tracer, error)
func Configure(opts Options) (*Tracer, error)
func Default() *Tracer
func Resolve(opts Options) (*Tracer, error)
func MustResolve(opts Options) *Tracer

func TracingMiddleware(opts Options) func(http.Handler) http.Handler
```

The dashboard is served by the `frontend` package:

```go
package frontend

func Handler(opts oida.Options) http.Handler
func HandlerFor(tracer *oida.Tracer) http.Handler
func Mount(r Router, opts oida.Options) error
func MountServeMux(mux *http.ServeMux, opts oida.Options) error
```

`Configure` creates the process-wide tracer. Set `opts.Tracer` to the returned value before wiring middleware and the dashboard so every entry point uses the same tracer.

```go
opts := oida.NewOptions()
tracer, err := oida.Configure(opts)
if err != nil {
	return err
}
opts.Tracer = tracer
```

`frontend.Mount` accepts routers with this method:

```go
type Router interface {
	Mount(pattern string, h http.Handler)
}
```

Use `frontend.MountServeMux` with `http.ServeMux`. `HandlerFor` is the shortest path from a tracer to a dashboard, and takes its configuration from the tracer.

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
func WithTrace(ctx context.Context, trace *Trace) context.Context
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

`End` is idempotent. `RecordError` marks both the span and trace as failed.

### Trace methods

The trace API matches the span API where the two mean the same thing:

```go
func (t *Trace) RecordError(err error)
func (t *Trace) Err() error
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
func (t *Tracer) Storage() Storage
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
}

func NewStorageMemory(size int) *StorageMemory
func NewStorageDisk(limit int, paths ...string) (*StorageDisk, error)
func (s *StorageDisk) Path() string
func (s *StorageDisk) Prune(ctx context.Context, maxAge time.Duration) error
```

Memory storage retains up to `size` traces. A size of zero retains none. Disk storage retains traces across restarts; `limit` controls the maximum count and `Prune` applies an age limit.

## Errors

| Error                  | Meaning                                                            |
|------------------------|--------------------------------------------------------------------|
| `ErrNilRouter`         | `frontend.Mount` or `frontend.MountServeMux` received a nil router |
| `ErrInvalidOptions`    | Base error for invalid configuration                               |
| `ErrInvalidPath`       | `Options.Path` is not absolute                                     |
| `ErrInvalidSampleRate` | `Options.SampleRate` is outside `[0,100]` or NaN                   |
| `ErrTraceNotFound`     | A storage lookup did not find the trace                            |
| `ErrDisabled`          | `StartTrace` was called on a disabled tracer                       |

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
