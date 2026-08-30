package model

import "context"

// Recorder is the substitutable surface of a tracer: the write side the
// instrumentation records through, and the read side the debug front end
// renders from. The root package's *Tracer implements it; code that only needs
// to record and read back traces can depend on this interface instead of the
// concrete tracer.
type Recorder interface {
	// StartTrace begins a trace the caller must complete with Finish.
	StartTrace(ctx context.Context, name string) (context.Context, *Trace, error)

	// Finish completes a trace and retains it.
	Finish(t *Trace)

	// Snapshot returns a race free copy of the recorded state.
	Snapshot() Snapshot

	// Traces returns the retained traces, newest first.
	Traces() []Trace

	// Trace returns the retained or in flight trace with the given ID.
	Trace(id string) (Trace, bool)

	// Live returns the traces currently in flight, newest first.
	Live() []Trace

	// Subscribe returns a channel notified whenever a trace starts or
	// completes, and a function releasing it.
	Subscribe() (<-chan struct{}, func())

	// Options returns the options the recorder was built with.
	Options() Options

	// Enabled reports whether the recorder records traces.
	Enabled() bool

	// SetEnabled turns recording on or off at runtime.
	SetEnabled(enabled bool)

	// Reset drops every retained trace and the lifetime counters.
	Reset()

	// ReportError forwards a failure to Options.OnError.
	ReportError(err error)
}
