# Architecture

## 1. Origin and prior art

`oida` is an extraction and generalisation of an in-process status page that
lived inside a scripting runtime: a request recorder plus a
`/debug/server-status` HTTP surface, backed by a telemetry model of requests and
timestamped spans. That implementation is referred to below as *the original*.

### 1.1 What the original did

| Concern | Implementation in the original |
| --- | --- |
| Request identity | 26-char Crockford base32 ULID per request, mirrored into the `Request-Id` request and response header |
| Active requests | `map[string]*Request` guarded by an `sync.RWMutex` |
| Completed requests | A slice truncated to `Options.RingBufferSize` on every completion |
| Span recording | `StartSpan(ctx, message, flags...)` appended to the request stored in the context |
| Span nesting | `open` / `close` marker flags; nesting depth reconstructed at render time by scanning for the matching open span of the same type |
| Lifecycle states | Scoreboard-style single-character states (`_ s R P W K C E`) with cumulative per-state time |
| Memory accounting | `runtime.ReadMemStats` before and after each request, deltas stored on the request |
| Pool estimate | Average allocation per request measured against `NextGC` and the process memory limit |
| Statistics | Requests grouped by host and request over the ring buffer: count, share, average duration/bytes/allocations |
| Timeline | A boundary sweep attributing each segment of the request to the innermost active span type |
| Rendering | One `html/template` with a large `FuncMap`, plus JSON and plain-text renderings selected by `Accept` and the user agent |

### 1.2 What carried over

- The ring buffer of completed units of work, sized by options.
- ULID identity, echoed in a response header.
- The kind taxonomy for spans (`database`, `internal`, `external`, `template`,
  `cache`, `http`) with a stable colour per kind in the UI.
- Rolling statistics over the ring buffer window, capped at `TopRequests`.
- The timeline sweep that turns overlapping spans into non-overlapping segments
  attributed to the innermost span.
- Per-trace memory delta accounting, gated by `TrackMemoryUse`.
- Content negotiation: HTML by default, JSON for JSON `Accept`, plain text for
  `curl` and `text/plain`.
- Scoreboard states for in-flight work.

### 1.3 What changed, and why

| Change | Rationale |
| --- | --- |
| Open/close marker flags replaced by real parent/child spans | The original was driven by a scripting language without `defer`, so it emitted paired open/close events and rebuilt nesting by scanning. Go callers write `ctx, span := oida.Start(ctx, ...)` / `defer span.End()`, so the parent is known at creation time. Depth becomes a field, not a reconstruction. |
| `Request` became `Trace` | oida records HTTP requests, background jobs, cron ticks and startup work. `Trace` is the neutral name; HTTP-specific fields move into a nested `HTTPInfo`. |
| `Span` is a struct, not an interface | The interface existed so foreign-language bindings could hold an opaque handle. Go callers benefit from a concrete `*Span` with nil-safe methods. Substitutability is retained through the `Recorder` interface. |
| Spans are appended under a per-trace mutex | The original appended from a single-threaded interpreter. Go handlers fan out into goroutines, so `Trace` appends are mutex-guarded and `Span.End` is idempotent. |
| Sampling added | The original recorded every request. oida targets production processes, so `SampleRate`/`Sampler` gate trace creation before any allocation happens. |
| `MaxSpansPerTrace` added | An unbounded loop calling `oida.Start` could otherwise retain unbounded memory inside one trace. Excess spans are counted in `Trace.DroppedSpans` and dropped. |
| `html/template` replaced by templ | Compile-time checked components, no `FuncMap` string indirection, and rendering errors surface as Go errors. |
| Path is configurable | The original hardcoded its route. oida takes `Options.Path`, default `/debug/oida`. |
| Authorization hook added | `/debug/oida` exposes URIs, user agents and span attributes. `Options.Authorize` gates access. |
| Retention behind a `Storage` interface | The original inlined a slice truncation. oida separates recording from retention, so the same tracer can hold traces in memory (`StorageMemory`) or write them to disk (`StorageDisk`) and keep them across a restart. |
| Failures are returned, never printed | The original could only drop errors on the floor. oida reports storage and rendering failures through `Options.OnError`; the package writes to neither stdout nor stderr. |

## 2. Package layout

```
oida/
  doc.go              package documentation
  model.go            data models: Kind, State, View, Attributes, HTTPInfo,
                      MemoryUse, Memory, PoolEstimate, StateDuration, Segment,
                      SpanRow, Statistic, Stats, Snapshot
  span.go             Span
  trace.go            Trace
  options.go          Options
  tracer.go           Tracer
  recorder.go         Recorder interface
  storage.go          Storage interface
  storage_memory.go   StorageMemory
  storage_disk.go     StorageDisk
  ring.go             ring buffer backing StorageMemory
  broker.go           change notification fan-out for the live stream
  sampler.go          Sampler interface + rateSampler
  context.go          context keys and accessors (no struct)
  id.go               ULID generation and validation (no struct)
  errors.go           sentinel errors (no struct)
  default.go          process-wide default tracer (no struct)
  middleware.go       TracingMiddleware (no struct)
  responsewriter.go   responseWriter capturing status and bytes
  handler.go          handler struct, Handler(Options)
  mount.go            Router interface, Mount, MountServeMux
  page.go             Page view model handed to templ components
  stats.go            statistics over the retention window (no struct)
  timeline.go         Timeline and Rows (no struct)
  format.go           formatting helpers used by components (no struct)
  text.go             content negotiation and plain text rendering (no struct)
  assets.go           the embedded public tree (no struct)
  public/assets/      oida.css, oida.js: embedded with //go:embed all:public
                      and served at {Path}/assets/
  view_layout.templ   Layout, page chrome, nav, metric tiles, state bar
  view_list.templ     List: recorded traces and the filter bar
  view_live.templ     Live: traces in flight
  view_stats.templ    Statistics: rolling statistics
  view_detail.templ   Detail: one trace, timeline, span tree, attributes
  view_*_templ.go     generated components, committed
  docs/               this documentation
  cmd/oida/           runnable chi/v5 demo service, the Docker image payload
  tests/              tests.NewServer: chi + oida wired for tests
  atkins.yml          CI/CD pipeline
  compose.yml         demo service behind the ingress network
  docker/Dockerfile   alpine image over bin/oida-linux-amd64
```

## 3. Concurrency model

Three levels of locking, deliberately kept independent:

1. **`Tracer.mu` (`sync.RWMutex`)** guards the active trace map, the lifetime
   counters and the cumulative state durations. Held only for map manipulation,
   never while rendering and never while calling into `Storage` — storage
   implementations do their own locking, so a slow disk write cannot block a
   request that is only starting a span.
2. **`Trace.mu` (`sync.Mutex`)** guards the span slice and the mutable trace
   fields. A handler that fans out into goroutines can call `oida.Start`
   concurrently on the same trace.
3. **`Span`** uses a mutex for attribute writes and an `ended` flag so `End` is
   idempotent — a `defer span.End()` plus an explicit `span.End()` on an error
   path records one duration, not two.

`Tracer.Snapshot` copies everything it needs under `RLock` and releases before
computing statistics, the timeline, or rendering. No template code ever holds a
tracer lock.

### 3.1 Nil and no-op behaviour

Every exported method on `*Span` and `*Trace` tolerates a nil receiver, and
`oida.Start` on a context without a trace returns a nil `*Span` plus the
unchanged context. Instrumented libraries therefore work unchanged in processes
that never configured oida:

```go
ctx, span := oida.Start(ctx, "cache.get", oida.KindCache)
defer span.End()          // nil-safe
span.SetAttribute("k", k) // nil-safe
```

## 4. Data flow

```
http.Request
  │
  ├─ TracingMiddleware
  │    ├─ Sampler.Sample(r) ── false ──► next.ServeHTTP (no allocation)
  │    └─ true
  │         ├─ NewTrace(id, http metadata), read MemStats
  │         ├─ ctx = WithTrace(ctx, trace); root span (KindHTTP)
  │         ├─ Tracer.active[id] = trace
  │         ├─ next.ServeHTTP(responseWriter, r)
  │         └─ defer: root span End, read MemStats, compute deltas,
  │                   delete from active, Storage.Save(trace.Clone())
  │
  └─ application code
       └─ oida.Start(ctx, name, kind) ──► trace append ──► *Span
                                          └─ span.End() records duration
```

The read path is completely separate:

```
GET /debug/oida[/live|/stats|/trace/{id}]
  │
  ├─ Options.Authorize(r) ── false ──► 404
  ├─ Tracer.Snapshot()   (copy under RLock)
  ├─ statistics / timeline computed on the copy
  └─ negotiate: templ component | JSON | plain text
```

## 5. Non-goals

- **No OpenTelemetry wire compatibility.** oida is deliberately in-process. A
  bridge can be written outside this package against the `Recorder` interface.
- **No database.** `StorageDisk` writes JSON documents to a folder and prunes
  them; it is not an index, and it is not queryable beyond "newest first".
- **Almost no client-side JavaScript.** The live view loads a 30-line script
  that opens an `EventSource` on `{Path}/live/events` and assigns the pushed
  markup to `innerHTML`. The server still renders every byte; there is no
  framework, no bundler, and no client state. Everything else is a plain link,
  and without scripting the page falls back to a meta refresh.
- **No metrics or log pipeline.** oida records traces. Counters and logs belong
  elsewhere.
