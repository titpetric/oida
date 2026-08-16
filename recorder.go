package oida

import "context"

// Recorder is the substitutable subset of Tracer. Code that only needs to
// record and read back traces can depend on this interface instead of the
// concrete tracer.
type Recorder interface {
	// StartTrace begins a trace the caller must complete with Finish.
	StartTrace(ctx context.Context, name string) (context.Context, *Trace, error)

	// Finish completes a trace and retains it.
	Finish(t *Trace)

	// Snapshot returns a race free copy of the recorded state.
	Snapshot() Snapshot
}
