package model

import (
	"sync/atomic"
	"time"
)

// clockStep is the least a recorded clock reading advances past the one
// before it. Windows ticks at half a millisecond and more, so a span can
// start and end on the same reading; issue #14 and docs/errata.md.
const clockStep = time.Nanosecond

// time returns a recorded reading of the trace clock, strictly after every
// earlier one of this trace: a reading equal to or behind the last recorded
// one advances by clockStep, so the shortest span a coarse clock records is
// one step long and sibling spans never tie on their start. The returned
// time carries no monotonic reading; within a trace, order and duration
// come from these values. Safe with or without the trace lock.
func (t *Trace) time() time.Time {
	now := t.raw()
	if t.box == nil {
		return now
	}
	nanos := now.UnixNano()
	for {
		last := atomic.LoadInt64(&t.box.lastNanos)
		if nanos <= last {
			nanos = last + int64(clockStep)
		}
		if atomic.CompareAndSwapInt64(&t.box.lastNanos, last, nanos) {
			return time.Unix(0, nanos)
		}
	}
}

// peek returns the trace clock without recording it: the reading for an
// observation, Elapsed and the snapshot copies, which measure without
// moving what the next recording reads. It never runs behind the last
// recorded reading.
func (t *Trace) peek() time.Time {
	now := t.raw()
	if t.box == nil {
		return now
	}
	if last := atomic.LoadInt64(&t.box.lastNanos); now.UnixNano() <= last {
		return time.Unix(0, last)
	}
	return now
}

// raw returns the configured clock's reading as it comes.
func (t *Trace) raw() time.Time {
	if t.clock == nil {
		return time.Now()
	}
	return t.clock()
}
