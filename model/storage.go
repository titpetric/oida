package model

import "context"

// Storage retains completed traces. Implementations must be safe for concurrent
// use: the tracer writes from request goroutines and reads from the debug front
// end at the same time.
//
// Two implementations ship with the root package: StorageMemory, a bounded ring
// buffer, and StorageDisk, a bounded folder of JSON documents.
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
}
