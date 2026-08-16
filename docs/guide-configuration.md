# Configuration

`oida.Options` is the single configuration struct. `oida.NewOptions()` returns
the defaults; take it and override what you need, so new fields keep their
sensible values when you upgrade.

```go
opts := oida.NewOptions()
opts.ServiceName = "billing-api"
opts.SampleRate = 0.25
```

## 1. Reference

```go
type Options struct {
	Path             string                        `yaml:"path"`
	ServiceName      string                        `yaml:"service_name"`
	Enabled          bool                          `yaml:"enabled"`
	RingBufferSize   int                           `yaml:"ring_buffer_size"`
	TopRequests      int                           `yaml:"top_requests"`
	MaxSpansPerTrace int                           `yaml:"max_spans_per_trace"`
	SampleRate       float64                       `yaml:"sample_rate"`
	TrackMemoryUse   bool                          `yaml:"track_memory_use"`
	TrustRequestID   bool                          `yaml:"trust_request_id"`
	IgnorePaths      []string                      `yaml:"ignore_paths"`
	RefreshInterval  int                           `yaml:"refresh_interval"`
	LiveStream       bool                          `yaml:"live_stream"`

	Sampler   Sampler                       `yaml:"-"`
	Storage   Storage                       `yaml:"-"`
	RouteFunc func(*http.Request) string    `yaml:"-"`
	OnError   func(error)                   `yaml:"-"`
	Authorize func(*http.Request) bool      `yaml:"-"`
	Clock     func() time.Time              `yaml:"-"`
	Tracer    *Tracer                       `yaml:"-"`
}
```

| Field | Default | Meaning |
| --- | --- | --- |
| `Path` | `/debug/oida` | Mount path of the UI. Must be absolute; trailing slashes are trimmed. Also implicitly added to `IgnorePaths`. |
| `ServiceName` | `""` | Shown in the header and stored on every trace. |
| `Enabled` | `true` | When false, the middleware passes through and the handler serves an empty snapshot. Flip at runtime with `Tracer.SetEnabled`. |
| `RingBufferSize` | `200` | Completed traces retained. 0 disables retention (live view still works). |
| `TopRequests` | `20` | Rows in the statistics view. |
| `MaxSpansPerTrace` | `1000` | Spans recorded per trace; further spans are counted in `DroppedSpans` and dropped. 0 means unlimited. |
| `SampleRate` | `1` | Fraction of requests traced, `[0,1]`. |
| `TrackMemoryUse` | `true` | Read `runtime.MemStats` around each trace. |
| `TrustRequestID` | `false` | Reuse a client-supplied `Request-Id` header instead of generating one. Only enable behind a trusted proxy. |
| `IgnorePaths` | `["/healthz", "/readyz", "/metrics", "/favicon.ico"]` | Exact paths and `/prefix/*` patterns never traced. |
| `RefreshInterval` | `5` | Fallback refresh of the live view, in seconds, used when the browser cannot stream. 0 disables it. |
| `LiveStream` | `true` | Serve the live view over server sent events, so traces appear as they are recorded. False falls back to the meta refresh and 404s the stream route. |
| `Sampler` | nil | Overrides `SampleRate` entirely when set. |
| `Storage` | nil | Retention backend. Nil builds `NewStorageMemory(RingBufferSize)`. |
| `RouteFunc` | nil | Returns the routed pattern of a request, so statistics group by route. Nil falls back to `http.Request.Pattern`. |
| `OnError` | nil | Receives storage and rendering failures. Nil discards them — the package never prints. |
| `Authorize` | nil | Access check for the UI. Nil means "allow" — set it before exposing the route on a public listener. |
| `Clock` | `time.Now` | Time source. Tests inject a deterministic clock. |
| `Tracer` | nil | Explicit tracer for `TracingMiddleware`, `Handler` and `Mount`. Nil resolves the process default. |

### 1.1 Route patterns

Statistics group by `HTTP.Route` when the router provides one. The standard
library sets `http.Request.Pattern`, which oida reads with no configuration.
chi keeps the pattern in its route context, so hand it over explicitly:

```go
opts.RouteFunc = func(r *http.Request) string {
	if route := chi.RouteContext(r.Context()); route != nil {
		return route.RoutePattern()
	}
	return ""
}
```

Without it, `/users/1` and `/users/2` are two separate rows in the statistics
table instead of one `GET /users/{id}` row.

### 1.2 Errors

```go
opts.OnError = func(err error) {
	slog.Error("oida", "err", err)
}
```

This is the only channel for asynchronous failures — a disk write that failed, a
component that failed to render. In tests, point it at `t.Errorf`.

## 2. Sizing the ring buffer

Every retained trace holds its spans, so memory is roughly:

```
RingBufferSize × (trace overhead ≈ 400B + spans × (span overhead ≈ 200B + attributes))
```

200 traces × 30 spans ≈ 1.5 MB. A busy service with 100 spans per trace and a
1000-trace buffer is closer to 25 MB — measurable, so size it deliberately.
`MaxSpansPerTrace` is the guard rail against one pathological request pinning
memory until it rotates out.

## 2.1 Storage

Retention is a pluggable interface. The default keeps traces in memory:

```go
opts.Storage = oida.NewStorageMemory(500)   // implicit when Storage is nil
```

Disk storage survives a restart, which is what you want when the interesting
trace is the one that happened right before the process died:

```go
storage, err := oida.NewStorageDisk(5000, "/var/lib/myservice/traces")
if err != nil {
	return err
}
opts.Storage = storage
```

`NewStorageDisk` creates the folder, verifies it is writable, and prunes the
oldest documents past the limit on every save. With no path it uses a folder
under `os.TempDir()`. Documents are JSON, one per trace, named after the trace
ID — readable with `jq`, and cheap to ship elsewhere if you want to:

```bash
jq '.spans[] | select(.kind == "database") | {name, duration_ns}' /var/lib/myservice/traces/*.json
```

Add `StorageDisk.Prune(ctx, maxAge)` to a ticker if you want an age bound as
well as a count bound. Writes are `O(1)`; reads scan the folder, so keep the
limit within a few thousand documents unless you enjoy `readdir`.

Anything satisfying `oida.Storage` works — the interface is six methods, and
the tracer never holds a lock while calling it.

## 3. Sampling

### 3.1 Rate sampling

`SampleRate` uses a deterministic counter, not randomness: at 0.25 every fourth
request is traced. That makes tests reproducible and avoids clustering.

```go
opts.SampleRate = 0.1    // 1 in 10
opts.SampleRate = 1      // everything (default)
opts.SampleRate = 0      // nothing; the UI still serves, the log stays empty
```

### 3.2 Custom samplers

```go
type Sampler interface {
	Sample(r *http.Request) bool
}
```

Anything satisfying it replaces the rate sampler. `SamplerFunc` adapts a plain
function:

```go
opts.Sampler = oida.SamplerFunc(func(r *http.Request) bool {
	if r.Header.Get("X-Debug") == "1" {
		return true            // always trace explicit debug requests
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true            // always trace the API
	}
	return false
})
```

Combining a rate with an override:

```go
rate := oida.NewRateSampler(0.05)
opts.Sampler = oida.SamplerFunc(func(r *http.Request) bool {
	return r.Header.Get("X-Debug") == "1" || rate.Sample(r)
})
```

Sampling is decided *before* the trace is allocated, so an unsampled request
costs one interface call and nothing else. `oida.Start` inside an unsampled
request returns a nil span.

## 4. Multiple tracers

One process can run several tracers — an HTTP tracer with a small buffer and a
job tracer with a large one:

```go
httpOpts := oida.NewOptions()
httpOpts.Path = "/debug/oida"
httpOpts.RingBufferSize = 200
httpTracer, err := oida.New(httpOpts)
if err != nil {
	return err
}
httpOpts.Tracer = httpTracer

jobOpts := oida.NewOptions()
jobOpts.Path = "/debug/oida/jobs"
jobOpts.ServiceName = "billing-jobs"
jobOpts.RingBufferSize = 2000
jobTracer, err := oida.New(jobOpts)
if err != nil {
	return err
}
jobOpts.Tracer = jobTracer

if err := oida.Mount(r, httpOpts); err != nil {
	return err
}
if err := oida.Mount(r, jobOpts); err != nil {
	return err
}
```

Traces belong to whichever tracer created them, so the two UIs stay separate.
`oida.Start` follows the trace in the context and never consults a global.

### 4.1 One dashboard per virtual host

The same property gives you virtual hosts: build a tracer per hostname, mount
each dashboard on that host's router, and the data never mixes. Both dashboards
can live at the same path, because they are reached through different hosts.

```go
// vhost wires one hostname: its own tracer, middleware and dashboard.
func vhost(name string) (http.Handler, error) {
	opts := oida.NewOptions()
	opts.ServiceName = name
	opts.RouteFunc = chiRoute

	tracer, err := oida.New(opts)
	if err != nil {
		return nil, err
	}
	opts.Tracer = tracer

	r := chi.NewRouter()
	r.Use(oida.TracingMiddleware(opts))
	if err := oida.Mount(r, opts); err != nil {
		return nil, err
	}

	r.Get("/", index)
	return r, nil
}

shop, err := vhost("shop.example")
if err != nil {
	return err
}
admin, err := vhost("admin.example")
if err != nil {
	return err
}

hosts := map[string]http.Handler{
	"shop.example":  shop,
	"admin.example": admin,
}

root := chi.NewRouter()
root.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
	handler, ok := hosts[stripPort(r.Host)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	handler.ServeHTTP(w, r)
})
```

`chi.HostRouter` style dispatch works the same way; all oida needs is that each
hostname's requests pass through that hostname's middleware.

Two things to keep in mind:

- Use `oida.New`, not `oida.Configure`. The process default tracer is a single
  instance; two hosts sharing it would share a ring buffer.
- Ring buffers are per host, so `RingBufferSize` is a per host budget. Ten hosts
  at 500 traces each retain 5000 traces.

A single tracer with the host filter is the other option: one dashboard, filter
by host, and per host counts on the statistics view. That shares one memory
budget across hosts and lets you compare them side by side. Separate tracers
give isolation; one tracer gives comparison.

## 5. Loading from YAML

Every configurable field has a `yaml` tag; the function fields do not and must
be set in code.

```yaml
oida:
  path: /debug/oida
  service_name: billing-api
  enabled: true
  ring_buffer_size: 500
  top_requests: 20
  max_spans_per_trace: 1000
  sample_rate: 0.25
  track_memory_use: true
  trust_request_id: false
  refresh_interval: 5
  live_stream: true
  ignore_paths:
    - /healthz
    - /readyz
    - /metrics
```

```go
type Config struct {
	Oida oida.Options `yaml:"oida"`
}

cfg := Config{Oida: oida.NewOptions()}     // defaults first
if err := yaml.Unmarshal(data, &cfg); err != nil {
	return err
}
cfg.Oida.Authorize = adminOnly
tracer, err := oida.Configure(cfg.Oida)
```

Note the ordering: unmarshal *into* the defaults, so keys absent from the file
keep their default rather than becoming zero.

## 6. Validation

`Options.Validate() error` is called by `New`, `Configure` and `Mount`:

| Condition | Error |
| --- | --- |
| `Path` empty or not starting with `/` | `ErrInvalidPath` |
| `SampleRate` outside `[0,1]` or NaN | `ErrInvalidSampleRate` |
| `RingBufferSize`, `TopRequests`, `MaxSpansPerTrace` or `RefreshInterval` negative | `ErrInvalidOptions` |

`Options.WithDefaults() Options` fills zero values and is applied before
validation, so a zero `Options{}` is valid and behaves like `NewOptions()`
except for `Enabled`, which is explicitly defaulted to true.

## 7. Turning it off

```go
opts.Enabled = false            // at construction
tracer.SetEnabled(false)        // at runtime
```

A disabled tracer stops recording immediately; existing traces stay in the ring
buffer until `Reset()` clears them. Nothing else in the process changes — spans
started against a disabled tracer are nil.
