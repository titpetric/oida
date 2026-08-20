package frontend

import (
	"sort"

	"github.com/titpetric/oida"
)

// SortKey names a column the trace list can be ordered by.
type SortKey string

// The columns the trace list can be ordered by.
const (
	SortAge       SortKey = "age"
	SortDuration  SortKey = "duration"
	SortSpans     SortKey = "spans"
	SortAllocated SortKey = "allocated"
)

// sortKeys lists the sortable columns, so an unknown value in a URL falls back
// to the default rather than sorting by nothing.
var sortKeys = []SortKey{SortAge, SortDuration, SortSpans, SortAllocated}

// sortTraces orders traces by key. Ascending means "smallest first" for numbers
// and "oldest first" for age, which is what the arrow in the header claims.
func sortTraces(traces []oida.Trace, key SortKey, ascending bool) {
	less := func(a, b oida.Trace) bool { return a.StartedAt.Before(b.StartedAt) }
	switch key {
	case SortDuration:
		less = func(a, b oida.Trace) bool { return a.Duration < b.Duration }
	case SortSpans:
		less = func(a, b oida.Trace) bool { return len(a.Spans) < len(b.Spans) }
	case SortAllocated:
		less = func(a, b oida.Trace) bool {
			return a.Memory.AllocatedBytes < b.Memory.AllocatedBytes
		}
	}

	sort.SliceStable(traces, func(i, j int) bool {
		if ascending {
			return less(traces[i], traces[j])
		}
		return less(traces[j], traces[i])
	})
}
