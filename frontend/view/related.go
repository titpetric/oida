package view

import (
	"sort"

	"github.com/titpetric/oida/model"
)

// Related returns the other transactions of the same W3C trace, oldest first:
// the browser action above a request, the request above the run it started.
//
// The log is what one process retained, so a trace spanning two is read whole
// only where both record into the same storage.
func Related(trace model.Trace, log []model.Trace) []model.Trace {
	if trace.TraceID == "" {
		return nil
	}
	related := make([]model.Trace, 0, 4)
	for _, other := range log {
		if other.TraceID == trace.TraceID && other.ID != trace.ID {
			related = append(related, other)
		}
	}
	if len(related) == 0 {
		return nil
	}
	sort.Slice(related, func(i, j int) bool { return related[i].StartedAt.Before(related[j].StartedAt) })
	return related
}

// Caller returns the transaction whose span this one named as its parent, and
// whether the log holds it.
func Caller(trace model.Trace, related []model.Trace) (model.Trace, bool) {
	if trace.ParentSpanID == "" {
		return model.Trace{}, false
	}
	for _, other := range related {
		if other.SpanID == trace.ParentSpanID {
			return other, true
		}
	}
	return model.Trace{}, false
}
