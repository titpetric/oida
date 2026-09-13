package model

import (
	"sync"
)

// traceBox is the one allocation a pooled trace lives in: the trace, the
// mutex it locks with, the HTTP info of the request and slots for the first
// spans. AcquireTrace takes a box from the pool and Release returns it, so a
// recorded request reuses the memory of an earlier one instead of allocating
// its own.
type traceBox struct {
	trace Trace
	mu    sync.Mutex
	http  HTTPInfo

	// spans are the inline slots appendSpan hands out before touching the
	// heap; ptrs seeds the Spans slice with matching capacity. The array is
	// fixed size because a handed out *Span must never move.
	spans [4]spanBox
	ptrs  [4]*Span
	used  int
}

// tracePool recycles trace boxes across requests. Release zeroes a box
// before putting it back, so a box from the pool is as clean as a new one.
var tracePool = sync.Pool{
	New: func() any { return new(traceBox) },
}

// AcquireTrace returns a trace the way NewTrace does, backed by a pooled
// allocation: the trace, its mutex, its HTTP info and the first spans share
// one reused block. The recorder returns it with Release once the trace is
// finished and cloned; a trace that is never released is collected like any
// other allocation.
func AcquireTrace(id, name string, opts TraceOptions) *Trace {
	box := tracePool.Get().(*traceBox)
	now := opts.now()
	trace := &box.trace
	*trace = Trace{
		ID:        id,
		Name:      name,
		Service:   opts.Service,
		State:     StateStarting,
		StartedAt: now,
		UpdatedAt: now,
		mu:        &box.mu,
		clock:     opts.Clock,
		maxSpans:  opts.MaxSpans,
		changedAt: now,

		captureLogs: opts.CaptureLogs,
		box:         box,
	}
	trace.Spans = box.ptrs[:0:len(box.ptrs)]
	return trace
}

// Release returns the trace to the pool it was acquired from. After Release
// the trace and every span it recorded are recycled memory: a *Trace or
// *Span kept past this point is invalid, the way an http.ResponseWriter is
// invalid after the handler returns. It is safe on a nil trace and a no-op
// on a trace that did not come from AcquireTrace.
func (t *Trace) Release() {
	if t == nil || t.box == nil {
		return
	}
	box := t.box
	*box = traceBox{}
	tracePool.Put(box)
}
