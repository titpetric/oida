package oida

import (
	"time"
)

// Kind classifies the work a span measured. The set is open: an unrecognized
// value is valid, renders with the fallback color and groups on its own in the
// timeline.
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

// Color returns the stable UI color of the kind. This is a categorical scale,
// not a theme: the hues are spread and saturated so two segments of a timeline
// can be told apart at a glance, which matters more here than restraint. Text
// on top of these is the dark ink, so every one of them carries a label.
func (k Kind) Color() string {
	switch k {
	case KindInternal:
		// The catch-all bucket stays grey, so the named kinds stand out.
		return "#6b7785"
	case KindHTTP:
		return "#2ee08a"
	case KindDatabase:
		return "#3d7bff"
	case KindExternal:
		// Orange, not violet: external work most often sits beside database
		// work on a timeline, and two adjacent blues read as one band.
		return "#ff9f2e"
	case KindTemplate:
		return "#ff4d94"
	case KindCache:
		return "#12dcdc"
	case KindQueue:
		return "#b06bff"
	default:
		return "#ffd93b"
	}
}

// String implements fmt.Stringer.
func (k Kind) String() string {
	if k == "" {
		return string(KindInternal)
	}
	return string(k)
}

// State is the scoreboard state of an in-flight trace. The one-character values
// follow the convention used by servers such as lighttpd.
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

// states lists every known state in display order.
var states = []State{
	StateWaiting,
	StateStarting,
	StateReading,
	StateProcessing,
	StateWriting,
	StateKeepalive,
	StateClosing,
	StateError,
}

// Label returns the human readable name of the state.
func (s State) Label() string {
	switch s {
	case StateWaiting:
		return "Waiting"
	case StateStarting:
		return "Starting"
	case StateReading:
		return "Reading"
	case StateProcessing:
		return "Processing"
	case StateWriting:
		return "Writing"
	case StateKeepalive:
		return "Keepalive"
	case StateClosing:
		return "Closing"
	case StateError:
		return "Error"
	default:
		return string(s)
	}
}

// View identifies one rendered page of the debug front end.
type View string

const (
	// ViewHosts is the landing page: which domains this process serves, and
	// how much traffic each one carries. Everything else is a drill down.
	ViewHosts  View = "hosts"
	ViewList   View = "list"
	ViewLive   View = "live"
	ViewStats  View = "stats"
	ViewDetail View = "detail"
)

// listKinds are the kinds offered in the front end filter, in display order.
var listKinds = []Kind{
	KindHTTP,
	KindDatabase,
	KindExternal,
	KindTemplate,
	KindCache,
	KindQueue,
	KindInternal,
}

// Attributes is a set of key/value pairs recorded on a span.
type Attributes map[string]any

// HTTPInfo describes the request a trace was created for.
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

// MemoryUse holds the process-wide allocation deltas observed while a trace
// ran. Concurrent traces overlap, so the values are indicative, not exact.
type MemoryUse struct {
	HeapDelta      int64         `json:"heap_delta_bytes"`
	AllocatedBytes uint64        `json:"allocated_bytes"`
	Allocations    uint64        `json:"allocations"`
	GCCycles       uint32        `json:"gc_cycles"`
	GCPause        time.Duration `json:"gc_pause_ns"`
}

// Memory describes current process memory and GC pressure.
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

// PoolEstimate is a heuristic concurrency estimate derived from observed
// per-trace allocations.
type PoolEstimate struct {
	Samples               uint64 `json:"samples"`
	AverageAllocatedBytes uint64 `json:"average_allocated_bytes"`
	BeforeNextGC          uint64 `json:"traces_before_next_gc,omitempty"`
	WithinMemoryLimit     uint64 `json:"traces_within_memory_limit,omitempty"`
}

// StateDuration is the lifetime trace time observed in one scoreboard state.
type StateDuration struct {
	State    State         `json:"state"`
	Label    string        `json:"label"`
	Duration time.Duration `json:"duration_ns"`
	Share    float64       `json:"share_percent"`
}

// Segment is one point-in-time region of a trace where a span kind was the
// innermost active span.
type Segment struct {
	Kind        Kind          `json:"kind"`
	Offset      time.Duration `json:"offset_ns"`
	Duration    time.Duration `json:"duration_ns"`
	OffsetShare float64       `json:"offset_percent"`
	Share       float64       `json:"share_percent"`
}

// SelectOption is one choice in a SelectField.
type SelectOption struct {
	Value string `json:"value"`
	Label string `json:"label"`

	// Note is the secondary text on the right of the option, such as a count.
	Note string `json:"note,omitempty"`
}

// SelectField is a dropdown on the filter bar. The control is rendered rather
// than delegated to a native select, because a native select cannot be styled
// consistently across platforms, least of all its open menu.
type SelectField struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Label   string         `json:"label"`
	Value   string         `json:"value"`
	Options []SelectOption `json:"options"`
}

// Selected returns the label of the chosen option, falling back to the first
// one, so the closed control always names its own state.
func (f SelectField) Selected() string {
	for _, option := range f.Options {
		if option.Value == f.Value {
			return option.Label
		}
	}
	if len(f.Options) > 0 {
		return f.Options[0].Label
	}
	return ""
}

// IsSelected reports whether an option is the chosen one.
func (f SelectField) IsSelected(option SelectOption) bool {
	return option.Value == f.Value
}

// WaveSpan is one span in the shape the drawing wants: where it ran as a
// fraction of the trace, how deep it sat, and what colour it draws in.
//
// Every span is handed over, parents and root included, because the drawing is
// of the stack: a span that wraps the whole trace is the reason there is a
// floor under everything else. The legend answers the other question, where the
// time went, and answers it from the sweep instead.
type WaveSpan struct {
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
	Color  string  `json:"color"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Depth  int     `json:"depth"`
	Failed bool    `json:"failed,omitempty"`
}

// WaveTrace is the payload behind the drawing: the spans, how long the trace
// took, and how deep it nested. The duration lets the drawing work in time
// rather than in pixels; the depth lets it scale itself to the trace it has
// rather than to a number someone guessed.
type WaveTrace struct {
	Milliseconds float64    `json:"ms"`
	Depth        int        `json:"depth"`
	Spans        []WaveSpan `json:"spans"`
}

// AxisTick is one labelled step of the time axis under a timeline.
type AxisTick struct {
	Share float64 `json:"share_percent"`
	Label string  `json:"label"`
}

// SpanRow is the flattened render model of one span within a trace.
type SpanRow struct {
	Span

	Offset      time.Duration `json:"offset_ns"`
	OffsetShare float64       `json:"offset_percent"`
	Share       float64       `json:"share_percent"`
	Open        bool          `json:"open,omitempty"`
	Last        bool          `json:"-"`
}

// Statistic aggregates one group of traces in the rolling window.
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

	totalDuration      time.Duration
	totalResponseBytes uint64
	totalAllocated     uint64
	totalSpans         uint64
}

// HostStat aggregates the traffic of one host.
type HostStat struct {
	Host string `json:"host"`

	// Requests counts every request the host was seen in, sampled or not, for
	// the lifetime of the process.
	Requests uint64 `json:"requests"`

	// Traces counts the traces of this host retained in the rolling window.
	Traces uint64 `json:"traces"`

	Errors          uint64        `json:"errors"`
	Share           float64       `json:"share_percent"`
	Routes          int           `json:"routes"`
	AverageDuration time.Duration `json:"average_duration_ns"`
	MaxDuration     time.Duration `json:"max_duration_ns"`
	AverageSpans    float64       `json:"average_spans"`

	totalDuration time.Duration
	totalSpans    uint64
}

// Stats contains the most frequent trace groups in the rolling window.
type Stats struct {
	WindowSize  int         `json:"window_size"`
	WindowLimit int         `json:"window_limit"`
	TopLimit    int         `json:"top_limit"`
	Top         []Statistic `json:"top"`
	Hosts       []HostStat  `json:"hosts"`
}

// Snapshot is the complete read model of a tracer at one point in time.
type Snapshot struct {
	Service    string        `json:"service"`
	StartedAt  time.Time     `json:"started_at"`
	Uptime     time.Duration `json:"uptime_ns"`
	PID        int           `json:"pid"`
	GoVersion  string        `json:"go_version"`
	GOMAXPROCS int           `json:"gomaxprocs"`
	Goroutines int           `json:"goroutines"`

	Total   uint64 `json:"total_requests"`
	Sampled uint64 `json:"sampled_traces"`
	Dropped uint64 `json:"dropped_traces"`
	Active  int    `json:"active_traces"`

	// Errors counts recorded traces that failed, and SLA is the share of
	// recorded traces that did not, as a percentage. Requests the sampler
	// rejected have unknown outcomes, so they are not in the denominator.
	Errors uint64  `json:"failed_traces"`
	SLA    float64 `json:"sla_percent"`

	StateTime []StateDuration `json:"state_time"`
	Memory    Memory          `json:"memory"`
	Pool      PoolEstimate    `json:"pool_estimate"`

	Live       []Trace `json:"live"`
	Log        []Trace `json:"log"`
	Statistics Stats   `json:"statistics"`
}
