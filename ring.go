package oida

// ring is a fixed size ring buffer of completed traces. It is not safe for
// concurrent use; the tracer holds its lock around every call.
type ring struct {
	items []*Trace
	next  int
	full  bool
}

// newRing returns a ring buffer retaining size traces. A size of zero retains
// nothing.
func newRing(size int) *ring {
	if size < 0 {
		size = 0
	}
	return &ring{items: make([]*Trace, size)}
}

// push stores a trace, overwriting the oldest one when the buffer is full. It
// reports whether a trace was evicted.
func (r *ring) push(t *Trace) bool {
	if r == nil || len(r.items) == 0 || t == nil {
		return t != nil
	}
	evicted := r.items[r.next] != nil
	r.items[r.next] = t
	r.next = (r.next + 1) % len(r.items)
	if r.next == 0 {
		r.full = true
	}
	return evicted
}

// len returns the number of retained traces.
func (r *ring) len() int {
	if r == nil {
		return 0
	}
	if r.full {
		return len(r.items)
	}
	return r.next
}

// cap returns the retention limit.
func (r *ring) cap() int {
	if r == nil {
		return 0
	}
	return len(r.items)
}

// list returns the retained traces, newest first.
func (r *ring) list() []*Trace {
	if r == nil || len(r.items) == 0 {
		return nil
	}
	out := make([]*Trace, 0, r.len())
	for i := range r.len() {
		index := (r.next - 1 - i + len(r.items)*2) % len(r.items)
		if trace := r.items[index]; trace != nil {
			out = append(out, trace)
		}
	}
	return out
}

// find returns the retained trace with the given ID.
func (r *ring) find(id string) (*Trace, bool) {
	if r == nil || id == "" {
		return nil, false
	}
	for _, trace := range r.list() {
		if trace.ID == id {
			return trace, true
		}
	}
	return nil, false
}

// reset drops every retained trace.
func (r *ring) reset() {
	if r == nil {
		return
	}
	clear(r.items)
	r.next = 0
	r.full = false
}
