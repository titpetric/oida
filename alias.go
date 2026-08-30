package oida

import "github.com/titpetric/oida/model"

// The recorded data lives in the model package, so the front end can read it
// without depending on the recorder. These aliases keep it spelled the way the
// rest of the API is: one import is enough to instrument a service.
type (
	// Trace is one recorded unit of work.
	Trace = model.Trace

	// Span is one timed operation within a trace.
	Span = model.Span

	// Spans is the recorded span list of a trace, searchable with Find.
	Spans = model.Spans

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

// Well known attribute keys. The set is open; these are the ones the front end
// renders as sizes.
const (
	AttrMemoryLimit = model.AttrMemoryLimit
	AttrMemoryUsage = model.AttrMemoryUsage
)

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

// BackgroundHost is the host label of traces that did not arrive over the
// network: cron ticks, queue consumers, startup work.
const BackgroundHost = model.BackgroundHost

// TraceHost returns the host a trace belongs to. Background traces have none,
// so they group under BackgroundHost rather than an empty string.
func TraceHost(trace Trace) string {
	return model.TraceHost(trace)
}

// IsBytes reports whether an attribute key holds a size in bytes.
func IsBytes(key string) bool {
	return model.IsBytes(key)
}

// ValidID reports whether id looks like a trace identifier this package
// records. It keeps hostile input out of lookups and out of rendered links.
func ValidID(id string) bool {
	return model.ValidID(id)
}

// LogEntry is one log line recorded on a trace by Trace.Info, Trace.Error and
// their Span counterparts. No context is involved: a trace attributes the
// entry to its innermost open span, and a span uses its own id.
type LogEntry = model.LogEntry

// Log levels recorded by Trace.Info and Trace.Error.
const (
	LevelInfo  = model.LevelInfo
	LevelError = model.LevelError
)
