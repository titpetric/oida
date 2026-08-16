# Specification: data model

All data models live in `model.go`, except `Span` (`span.go`) and `Trace`
(`trace.go`), which carry behaviour and therefore get their own file under the
one-struct-per-file rule described in [conventions.md](conventions.md).

Every model is JSON-encodable; the JSON rendering of `/debug/oida` is the
serialization of these types. Durations are encoded as integer nanoseconds with
an `_ns` suffix on the field name.

## 1. Kind

```go
type Kind string

const (
    KindInternal Kind = "internal"
    KindHTTP     Kind = "http"
    KindDatabase Kind = "database"
    KindExternal Kind = "external"
    KindTemplate Kind = "template"
    KindCache    Kind = "cache"
    KindQueue    Kind = "queue"
)
```

`Kind` is an open enum: an unrecognised value is valid, renders with the
fallback colour, and groups on its own in the timeline. The zero value is
normalised to `KindInternal` when a span is created.

Colours are stable per kind so the timeline, the kind badge and the legend agree:

| Kind | Colour |
| --- | --- |
| `internal` | `#6b7280` |
| `http` | `#16a34a` |
| `database` | `#2563eb` |
| `external` | `#7c3aed` |
| `template` | `#db2777` |
| `cache` | `#0891b2` |
| `queue` | `#ca8a04` |
| *(other)* | `#d97706` |

## 2. State

Scoreboard states for in-flight work, following the server scoreboard
convention used by lighttpd and Apache:

```go
type State string

const (
    StateWaiting    State = "_"
    StateStarting   State = "s"
    StateReading    State = "R"
    StateProcessing State = "P"
    StateWriting    State = "W"
    StateKeepalive  State = "K"
    StateClosing    State = "C"
    StateError      State = "E"
)
```

`State.Label()` returns the human label (`Waiting`, `Starting`, …). The tracer
accumulates lifetime time-in-state per state; a trace transitions
`Reading → Processing → Writing` under the middleware, or to `Error` when a
handler panics or `Trace.Fail` is called.

## 3. Span

```go
type Span struct {
    ID         int            `json:"id"`
    ParentID   int            `json:"parent_id,omitempty"`
    TraceID    string         `json:"trace_id"`
    Name       string         `json:"name"`
    Kind       Kind           `json:"kind"`
    StartedAt  time.Time      `json:"started_at"`
    Duration   time.Duration  `json:"duration_ns,omitempty"`
    Depth      int            `json:"depth"`
    Filename   string         `json:"filename,omitempty"`
    Line       int            `json:"line,omitempty"`
    Attributes map[string]any `json:"attributes,omitempty"`
    Error      string         `json:"error,omitempty"`
    // unexported: trace back-reference, mutex, ended flag
}
```

Rules:

- `ID` is a per-trace sequence starting at 1. `ParentID` is 0 for the root span.
- `Depth` is `parent.Depth + 1`, fixed at creation; it is never recomputed.
- `StartedAt` uses the tracer clock (`Options.Clock`).
- `Duration` is set once by `End`. A span that never ends keeps `Duration == 0`
  and renders as *(open)*.
- `Attributes` is lazily allocated on the first `SetAttribute`.
- `Error` holds `err.Error()` of the last non-nil error recorded, and sets the
  span's error flag used for highlighting in the UI.

### 3.1 Methods

| Method | Behaviour |
| --- | --- |
| `End()` | Records `Duration = clock() - StartedAt`. Idempotent; second and later calls are ignored. Nil-safe. |
| `EndWithError(err error)` | `RecordError(err)` then `End()`. |
| `SetAttribute(key string, value any)` | Stores an attribute. Nil-safe. Values are rendered with `%v`. |
| `SetAttributes(Attributes)` | Bulk variant. |
| `RecordError(err error)` | Stores `err.Error()`, marks the span and its trace as failed. No-op on nil error. |
| `SetSource(filename string, line int)` | Records the source location shown in the span table. |
| `Failed() bool` | Reports whether an error was recorded. |
| `Elapsed() time.Duration` | `Duration` if ended, otherwise time since `StartedAt`. |
| `Context(ctx) context.Context` | Returns a context with this span as the active parent. |

All methods are safe on a nil `*Span`, which is what `Start` returns when the
context has no trace or the trace was not sampled.

## 4. Trace

```go
type Trace struct {
    ID        string        `json:"id"`
    Name      string        `json:"name"`
    Service   string        `json:"service,omitempty"`
    State     State         `json:"state"`
    StartedAt time.Time     `json:"started_at"`
    UpdatedAt time.Time     `json:"updated_at"`
    Duration  time.Duration `json:"duration_ns"`
    Error     string        `json:"error,omitempty"`

    HTTP   *HTTPInfo `json:"http,omitempty"`
    Memory MemoryUse `json:"memory"`

    Spans        []*Span `json:"spans,omitempty"`
    DroppedSpans int     `json:"dropped_spans,omitempty"`
    // unexported: mutex, sequence counter, state change timestamp, memstats
}
```

`Name` is `"GET /users/{id}"` for HTTP traces (method plus routed pattern when
the router exposes one, raw path otherwise), or the caller-supplied name for
background traces.

### 4.1 HTTPInfo

```go
type HTTPInfo struct {
    Method        string `json:"method"`
    URI           string `json:"uri"`
    Route         string `json:"route,omitempty"`
    Host          string `json:"host"`
    Protocol      string `json:"protocol"`
    RemoteAddress string `json:"remote_address"`
    UserAgent     string `json:"user_agent,omitempty"`
    Status        int    `json:"status,omitempty"`
    ResponseBytes int64  `json:"response_bytes"`
}
```

`Route` is populated from `chi.RouteContext(r.Context()).RoutePattern()` when
available, and is what statistics group on — so `/users/1` and `/users/2`
aggregate into `/users/{id}`.

### 4.2 MemoryUse

Per-trace deltas, recorded only when `Options.TrackMemoryUse` is set. Values are
process-wide and overlap when requests run concurrently; the UI says so.

```go
type MemoryUse struct {
    HeapDelta      int64         `json:"heap_delta_bytes"`
    AllocatedBytes uint64        `json:"allocated_bytes"`
    Allocations    uint64        `json:"allocations"`
    GCCycles       uint32        `json:"gc_cycles"`
    GCPause        time.Duration `json:"gc_pause_ns"`
}
```

### 4.3 Methods

| Method | Behaviour |
| --- | --- |
| `StartSpan(ctx, name, kind...) (context.Context, *Span)` | Appends a span whose parent is the active span in `ctx`. Enforces `MaxSpansPerTrace`. |
| `Root() *Span` | First span, or nil. |
| `SetState(State)` | Transitions state, accumulating time in the previous state. |
| `Fail(err error)` | Records a trace-level error and sets `StateError`. |
| `Clone() Trace` | Deep copy with copied span pointers, used by `Snapshot`. |
| `SpanCount() int` | Recorded spans, excluding dropped ones. |

## 5. Snapshot

`Snapshot` is the whole read model, produced by `Tracer.Snapshot()`:

```go
type Snapshot struct {
    Service    string        `json:"service"`
    StartedAt  time.Time     `json:"started_at"`
    Uptime     time.Duration `json:"uptime_ns"`
    PID        int           `json:"pid"`
    GoVersion  string        `json:"go_version"`
    GOMAXPROCS int           `json:"gomaxprocs"`
    Goroutines int           `json:"goroutines"`

    Total    uint64 `json:"total_traces"`
    Sampled  uint64 `json:"sampled_traces"`
    Dropped  uint64 `json:"dropped_traces"`
    Active   int    `json:"active_traces"`

    StateTime []StateDuration `json:"state_time"`
    Memory    Memory          `json:"memory"`
    Pool      PoolEstimate    `json:"pool_estimate"`

    Live       []Trace `json:"live"`
    Log        []Trace `json:"log"`
    Statistics Stats   `json:"statistics"`
}
```

- `Live` is sorted newest-first and contains in-flight traces with `Duration`
  filled in as elapsed time.
- `Log` is the ring buffer, newest-first.
- `Total` counts every request seen by the middleware, `Sampled` those that
  produced a trace, `Dropped` the difference plus traces evicted from the ring.

### 5.1 Memory, PoolEstimate, StateDuration

```go
type Memory struct {
    HeapAlloc     uint64  `json:"heap_alloc_bytes"`
    HeapInuse     uint64  `json:"heap_inuse_bytes"`
    HeapObjects   uint64  `json:"heap_objects"`
    StackInuse    uint64  `json:"stack_inuse_bytes"`
    System        uint64  `json:"system_bytes"`
    NextGC        uint64  `json:"next_gc_bytes"`
    NumGC         uint32  `json:"gc_cycles"`
    GCPauseTotal  uint64  `json:"gc_pause_total_ns"`
    GCCPUFraction float64 `json:"gc_cpu_fraction"`
    Limit         uint64  `json:"memory_limit_bytes,omitempty"`
}

type PoolEstimate struct {
    Samples               uint64 `json:"samples"`
    AverageAllocatedBytes uint64 `json:"average_allocated_bytes"`
    BeforeNextGC          uint64 `json:"traces_before_next_gc,omitempty"`
    WithinMemoryLimit     uint64 `json:"traces_within_memory_limit,omitempty"`
}

type StateDuration struct {
    State    State         `json:"state"`
    Label    string        `json:"label"`
    Duration time.Duration `json:"duration_ns"`
    Share    float64       `json:"share_percent"`
}
```

`Memory.Limit` is the minimum of `debug.SetMemoryLimit(-1)`, the cgroup v2
`memory.max`, the cgroup v1 `memory.limit_in_bytes`, and `MemTotal` from
`/proc/meminfo`. Unreadable sources are skipped; if none resolve, the limit is 0
and the dependent pool estimates are omitted.

## 6. Statistics

```go
type Stats struct {
    WindowSize  int         `json:"window_size"`
    WindowLimit int         `json:"window_limit"`
    TopLimit    int         `json:"top_limit"`
    Top         []Statistic `json:"top"`
}

type Statistic struct {
    Name                  string        `json:"name"`
    Host                  string        `json:"host,omitempty"`
    Count                 uint64        `json:"count"`
    Errors                uint64        `json:"errors"`
    Share                 float64       `json:"share_percent"`
    AverageDuration       time.Duration `json:"average_duration_ns"`
    MaxDuration           time.Duration `json:"max_duration_ns"`
    AverageResponseBytes  uint64        `json:"average_response_bytes"`
    AverageAllocatedBytes uint64        `json:"average_allocated_bytes"`
    AverageSpans          float64       `json:"average_spans"`
}
```

Grouping key is `host + "\x00" + name`, where `name` prefers `HTTP.Route` over
the raw URI. Sort order: count descending, then total duration descending, then
name and host ascending — a total order, so the table is stable between
refreshes. The result is truncated to `Options.TopRequests`.

## 7. Timeline

```go
type Segment struct {
    Kind        Kind          `json:"kind"`
    Offset      time.Duration `json:"offset_ns"`
    Duration    time.Duration `json:"duration_ns"`
    OffsetShare float64       `json:"offset_percent"`
    Share       float64       `json:"share_percent"`
}
```

`Timeline(Trace) []Segment` converts overlapping spans into a non-overlapping
sequence:

1. Collect every ended span with a positive duration, clamped to the trace
   window; skip the root HTTP span so it does not shadow everything.
2. Collect and sort all interval boundaries, deduplicated.
3. For each adjacent boundary pair, attribute the segment to the innermost
   active interval (latest start, then earliest end).
4. Merge adjacent segments of the same kind.

Shares are percentages of `Trace.Duration`, so the segments render directly as
CSS `left`/`width`.

## 8. Invariants

1. A `*Span` returned by `Start` is never used after the trace is completed;
   ending it afterwards still only mutates the span.
2. `Trace.Duration > 0` for every trace in the ring buffer.
3. `len(Trace.Spans) <= Options.MaxSpansPerTrace` when the limit is positive.
4. Span IDs within a trace are unique and monotonic; `ParentID < ID` always, so
   the span tree can be built in one pass.
5. Snapshots never alias live tracer state; mutating a snapshot cannot affect
   the tracer.
