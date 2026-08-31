package ring

import "github.com/titpetric/oida/model"

// Ring is a fixed size ring buffer of completed traces. It is not safe for
// concurrent use; the tracer holds its lock around every call.
type Ring struct {
	items []*model.Trace
	next  int
	full  bool
}

// New returns a ring buffer retaining size traces. A size of zero retains
// nothing.
func New(size int) *Ring {
	if size < 0 {
		size = 0
	}
	return &Ring{items: make([]*model.Trace, size)}
}

// Push stores a trace, overwriting the oldest one when the buffer is full. It
// reports whether a trace was evicted.
func (r *Ring) Push(t *model.Trace) bool {
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

// Len returns the number of retained traces.
func (r *Ring) Len() int {
	if r == nil {
		return 0
	}
	if r.full {
		return len(r.items)
	}
	return r.next
}

// Cap returns the retention limit.
func (r *Ring) Cap() int {
	if r == nil {
		return 0
	}
	return len(r.items)
}

// List returns the retained traces, newest first.
func (r *Ring) List() []*model.Trace {
	if r == nil || len(r.items) == 0 {
		return nil
	}
	out := make([]*model.Trace, 0, r.Len())
	for i := range r.Len() {
		index := (r.next - 1 - i + len(r.items)*2) % len(r.items)
		if trace := r.items[index]; trace != nil {
			out = append(out, trace)
		}
	}
	return out
}

// Find returns the retained trace with the given ID.
func (r *Ring) Find(id string) (*model.Trace, bool) {
	if r == nil || id == "" {
		return nil, false
	}
	for _, trace := range r.List() {
		if trace.ID == id {
			return trace, true
		}
	}
	return nil, false
}

// Reset drops every retained trace.
func (r *Ring) Reset() {
	if r == nil {
		return
	}
	clear(r.items)
	r.next = 0
	r.full = false
}
