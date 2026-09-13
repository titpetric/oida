package model

import (
	"runtime"
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

	// armed mirrors the runtime's finalizer registration for the box,
	// which reuse does not clear: SetFinalizer panics on a double set, and
	// a finalizer that ran is gone. reset leaves it alone.
	armed bool
}

// reset assigns every allocation-holding field its zero value, in place:
// the storage is inline, so nothing is freed or recreated. Only the slots
// below used are cleared, which is complete: a slot past used was never
// written, and ptrs[i] is set only below used. It runs when the box is
// reused, not when it is released, so a released trace keeps its values
// until the memory changes owners. armed is not touched: it mirrors the
// runtime's finalizer registration, which reuse does not clear.
func (b *traceBox) reset() {
	b.trace = Trace{}
	b.http = HTTPInfo{}
	for i := range b.used {
		b.spans[i] = spanBox{}
		b.ptrs[i] = nil
	}
	b.used = 0
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

// ReleaseOnCollect arranges for the box to return to the pool once the
// collector finds the trace unreachable: Release for a trace whose lifetime
// the recorder does not own, such as one handed out by StartTrace. Any
// holder keeps the box reachable, so the memory moves only after the last
// reference is gone. It is safe on a nil trace and a no-op without a box.
func (t *Trace) ReleaseOnCollect() {
	if t == nil || t.box == nil || t.box.armed {
		return
	}
	t.box.armed = true
	runtime.SetFinalizer(t.box, releaseBox)
}

// releaseBox recycles a box the collector proved unreachable. The finalizer
// ran and is gone, so the box is disarmed and dies on a later pool eviction
// unless it is armed again.
func releaseBox(box *traceBox) {
	box.armed = false
	tracePool.Put(box)
}
