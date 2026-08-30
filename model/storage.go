package model

import (
	"context"
	"time"
)

// Storage retains completed traces. Implementations must be safe for concurrent
// use: the tracer writes from request goroutines and reads from the debug front
// end at the same time.
//
// Two implementations ship in the storage package: a bounded ring buffer, and
// a bounded folder of JSON documents.
type Storage interface {
	// Save retains a completed trace.
	Save(ctx context.Context, trace Trace) error

	// Load returns a retained trace, or ErrTraceNotFound.
	Load(ctx context.Context, id string) (Trace, error)

	// List returns retained traces newest first, at most limit of them. A limit
	// of zero or less returns everything retained.
	List(ctx context.Context, limit int) ([]Trace, error)

	// Len returns the number of retained traces.
	Len(ctx context.Context) (int, error)

	// Cap returns the retention limit, or zero when unbounded.
	Cap() int

	// Reset drops every retained trace.
	Reset(ctx context.Context) error

	// Prune drops retained traces older than maxAge. A driver with nothing
	// to prune returns nil.
	Prune(ctx context.Context, maxAge time.Duration) error

	// Restore fills the read path from what the driver persisted, so a new
	// process can list what an earlier one recorded. A driver holding nothing
	// of its own returns nil.
	Restore(ctx context.Context) error
}
