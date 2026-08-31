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

// RequestIDHeader carries the trace identifier on the request and the
// response.
const RequestIDHeader = model.RequestIDHeader

// LogEntry is one log line recorded on a trace by Trace.Info, Trace.Error and
// their Span counterparts. No context is involved: a trace attributes the
// entry to its innermost open span, and a span uses its own id.
type LogEntry = model.LogEntry

// Log levels recorded by Trace.Info and Trace.Error.
const (
	LevelInfo  = model.LevelInfo
	LevelError = model.LevelError
)
