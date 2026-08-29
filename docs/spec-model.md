# Data returned by oida

Request JSON with `?format=json` or `Accept: application/json`. Durations are integer nanoseconds and use field names ending in `_ns`.

The Go types below are defined in `github.com/titpetric/oida/model` and aliased into `github.com/titpetric/oida`, so `oida.Trace` and `model.Trace` are the same type and either import works.

## Span kinds

```go
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

Custom `Kind` values are accepted. An omitted kind is recorded as `internal`.

## Span

```go
type Span struct {
	ID         int           `json:"id"`
	ParentID   int           `json:"parent_id,omitempty"`
	TraceID    string        `json:"trace_id"`
	Name       string        `json:"name"`
	Kind       Kind          `json:"kind"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration_ns,omitempty"`
	Depth      int           `json:"depth"`
	Filename   string        `json:"filename,omitempty"`
	Line       int           `json:"line,omitempty"`
	Attributes Attributes    `json:"attributes,omitempty"`
	Error      string        `json:"error,omitempty"`
}
```

Span IDs start at 1 within each trace. `ParentID` is zero for a root span. For active traces, `Duration` is the elapsed time when the trace was read. `Error` contains the latest error recorded on the span.

## Trace

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
	InFlight  bool          `json:"in_flight,omitempty"`

	HTTP       *HTTPInfo  `json:"http,omitempty"`
	Memory     MemoryUse  `json:"memory"`
	Attributes Attributes `json:"attributes,omitempty"`

	Spans        []*Span `json:"spans,omitempty"`
	DroppedSpans int     `json:"dropped_spans,omitempty"`
}
```

For HTTP traces, `Name` is the method and routed pattern when available, such as `GET /users/{id}`. Background traces use the name passed to `StartTrace` or `Observe`. `InFlight` is true when the trace was still running when read.

### Attributes

`Attributes` is a `map[string]any`, recorded on a trace with `Trace.SetAttribute` and on a span with `Span.SetAttribute`. What holds for the whole transaction belongs on the trace, what holds for one operation belongs on the span that measured it.

The set of keys is open. These are the keys the front end understands, and renders as sizes rather than as bare integers:

```go
const (
	AttrMemoryLimit = "memory_limit" // bytes, on the trace
	AttrMemoryUsage = "memory_usage" // bytes, on the trace and on its spans
)
```

`memory_limit` is the ceiling the transaction ran under. `memory_usage` is the memory in use when a span or a trace finished, so the readings across the spans of a trace are the memory curve of the request, and the span where the curve steps is the span that allocated. A runtime that charges allocations to the request, such as an interpreter, reports both; the Go runtime reports neither, because there the heap belongs to the process. `MemoryUse` covers what Go can say.

`Attributes.Int64(key)` reads a numeric value as an integer whatever type it arrived as, so a number that came back through JSON as a float, or over a wire as a decimal string, still reads as one. It reports false for a missing key and for a value that is not a number. `IsBytes(key)` reports whether a key holds a size in bytes.

### HTTP fields

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

`Route` is the routed pattern supplied by `Options.RouteFunc` or `http.Request.Pattern`. Statistics prefer it over the raw URI.

`RemoteAddress` is the resolved client IP, without a port. The middleware reads the `Forwarded` (RFC 7239) header, or `X-Forwarded-For` when `Forwarded` names no address, and walks the hops right to left. Hops in the trusted ranges are infrastructure, so the first address outside them is the client. The trusted ranges are:

- 10.0.0.0/8
- 172.16.0.0/12
- 192.168.0.0/16
- 127.0.0.0/8
- 169.254.0.0/16
- ::1/128
- fc00::/7
- fe80::/10

Entries that hold no address, such as `unknown` or an obfuscated RFC 7239 identifier, are skipped.

When every hop is inside those ranges, as with all-LAN traffic, the leftmost valid address wins. With no usable header the connection's own address is recorded, with the port stripped when it parses. There is nothing to configure; the ranges are fixed.

### Per-trace memory fields

```go
type MemoryUse struct {
	HeapDelta      int64         `json:"heap_delta_bytes"`
	AllocatedBytes uint64        `json:"allocated_bytes"`
	Allocations    uint64        `json:"allocations"`
	GCCycles       uint32        `json:"gc_cycles"`
	GCPause        time.Duration `json:"gc_pause_ns"`
}
```

These fields are populated when `Options.TrackMemoryUse` is true. They describe process-wide changes observed while the trace ran, so concurrent traces can overlap.

## Snapshot

`Tracer.Snapshot()` returns process information, counters, active traces, retained traces, and statistics:

```go
type Snapshot struct {
	Service    string        `json:"service"`
	StartedAt  time.Time     `json:"started_at"`
	Uptime     time.Duration `json:"uptime_ns"`
	PID        int           `json:"pid"`
	GoVersion  string        `json:"go_version"`
	GOMAXPROCS int           `json:"gomaxprocs"`
	Goroutines int           `json:"goroutines"`

	Total   uint64  `json:"total_requests"`
	Sampled uint64  `json:"sampled_traces"`
	Dropped uint64  `json:"dropped_traces"`
	Active  int     `json:"active_traces"`
	Errors  uint64  `json:"failed_traces"`
	SLA     float64 `json:"sla_percent"`

	StateTime []StateDuration `json:"state_time"`
	Memory    Memory          `json:"memory"`
	Pool      PoolEstimate    `json:"pool_estimate"`

	Live       []Trace `json:"live"`
	Log        []Trace `json:"log"`
	Statistics Stats   `json:"statistics"`
}
```

- `Total` counts sampled traces and HTTP requests rejected by the sampler.
- `Sampled` counts traces that were created.
- `Dropped` counts unsampled requests and sampled traces no longer retained.
- `Errors` counts recorded traces that failed.
- `SLA` is the percentage of recorded traces that did not fail.
- `Live` contains active traces, newest first.
- `Log` contains retained traces, newest first.

### Process memory

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
```

## Statistics

```go
type Stats struct {
	WindowSize  int         `json:"window_size"`
	WindowLimit int         `json:"window_limit"`
	TopLimit    int         `json:"top_limit"`
	Top         []Statistic `json:"top"`
	Hosts       []HostStat  `json:"hosts"`
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

type HostStat struct {
	Host            string        `json:"host"`
	Requests        uint64        `json:"requests"`
	Traces          uint64        `json:"traces"`
	Errors          uint64        `json:"errors"`
	Share           float64       `json:"share_percent"`
	Routes          int           `json:"routes"`
	AverageDuration time.Duration `json:"average_duration_ns"`
	MaxDuration     time.Duration `json:"max_duration_ns"`
	AverageSpans    float64       `json:"average_spans"`
}
```

`Top` groups retained traces by host and routed name. `Hosts` combines lifetime request counts with statistics from the retained trace window.
