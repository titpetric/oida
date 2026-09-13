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

// tracePool recycles trace boxes across requests. A box is cleared when it
// is acquired, not when it is released, so a released trace keeps its
// recorded values until the box is reused.
var tracePool = sync.Pool{
	New: func() any { return new(traceBox) },
}

// Release returns the trace's box to the pool. The recorded values stay in
// place until the box is reused, so a holder may still read the trace; once
// NewTrace picks the box up again, the memory belongs to the new trace. The
// recorder releases only the traces whose lifetime it owns end to end. It is
// safe on a nil trace and a no-op on a clone or a decoded trace, which have
// no box.
func (t *Trace) Release() {
	if t == nil || t.box == nil {
		return
	}
	box := t.box
	t.box = nil
	tracePool.Put(box)
}
