package frontend

import (
	"slices"
	"time"

	"github.com/titpetric/oida"
)

// Timeline converts the spans of a trace into non overlapping segments, each
// attributed to the innermost span that was active during it. Shares are
// percentages of the trace duration, so segments render directly as CSS
// offsets and widths.
func Timeline(trace oida.Trace) []Segment {
	total := trace.Duration
	if total <= 0 || len(trace.Spans) == 0 {
		return nil
	}
	end := trace.StartedAt.Add(total)

	type interval struct {
		kind  oida.Kind
		depth int
		start time.Time
		stop  time.Time
	}

	intervals := make([]interval, 0, len(trace.Spans))
	boundaries := make([]time.Time, 0, len(trace.Spans)*2)
	for _, span := range trace.Spans {
		if span == nil || span.Duration <= 0 {
			continue
		}
		if span.Kind == oida.KindHTTP && span.Depth == 0 {
			continue
		}
		start, stop := span.StartedAt, span.StartedAt.Add(span.Duration)
		if start.Before(trace.StartedAt) {
			start = trace.StartedAt
		}
		if stop.After(end) {
			stop = end
		}
		if !stop.After(start) {
			continue
		}
		intervals = append(intervals, interval{
			kind:  span.Kind,
			depth: span.Depth,
			start: start,
			stop:  stop,
		})
		boundaries = append(boundaries, start, stop)
	}
	if len(intervals) == 0 {
		return nil
	}

	slices.SortFunc(boundaries, func(a, b time.Time) int { return a.Compare(b) })
	boundaries = slices.Compact(boundaries)

	result := make([]Segment, 0, len(boundaries))
	for i := 0; i+1 < len(boundaries); i++ {
		start, stop := boundaries[i], boundaries[i+1]
		active := -1
		for j, candidate := range intervals {
			if candidate.start.After(start) || !candidate.stop.After(start) {
				continue
			}
			if active < 0 || candidate.depth > intervals[active].depth ||
				candidate.depth == intervals[active].depth && candidate.start.After(intervals[active].start) {
				active = j
			}
		}
		if active < 0 {
			continue
		}

		kind, duration := intervals[active].kind, stop.Sub(start)
		offset := start.Sub(trace.StartedAt)
		if len(result) > 0 {
			previous := &result[len(result)-1]
			if previous.Kind == kind && previous.Offset+previous.Duration == offset {
				previous.Duration += duration
				previous.Share = share(previous.Duration, total)
				continue
			}
		}
		result = append(result, Segment{
			Kind:        kind,
			Offset:      offset,
			Duration:    duration,
			OffsetShare: share(offset, total),
			Share:       share(duration, total),
		})
	}
	return result
}

// Rows flattens the spans of a trace into depth first render rows.
func Rows(trace oida.Trace) []SpanRow {
	if len(trace.Spans) == 0 {
		return nil
	}

	children := make(map[int][]*oida.Span, len(trace.Spans))
	for _, span := range trace.Spans {
		if span != nil {
			children[span.ParentID] = append(children[span.ParentID], span)
		}
	}

	total := trace.Duration
	rows := make([]SpanRow, 0, len(trace.Spans))

	var walk func(parent int)
	walk = func(parent int) {
		siblings := children[parent]
		for i, span := range siblings {
			row := SpanRow{
				Span:   span.Inert(),
				Offset: span.StartedAt.Sub(trace.StartedAt),
				Open:   span.Duration == 0,
				Last:   i == len(siblings)-1,
			}
			row.Memory, row.HasMemory = span.Attributes.Int64(oida.AttrMemoryUsage)
			if total > 0 {
				row.OffsetShare = share(row.Offset, total)
				row.Share = share(span.Duration, total)
			}
			rows = append(rows, row)
			walk(span.ID)
		}
	}
	walk(0)

	// Spans whose parent was dropped by the span limit are still rendered.
	if len(rows) < len(trace.Spans) {
		seen := make(map[int]struct{}, len(rows))
		for _, row := range rows {
			seen[row.ID] = struct{}{}
		}
		for _, span := range trace.Spans {
			if span == nil {
				continue
			}
			if _, ok := seen[span.ID]; ok {
				continue
			}
			row := SpanRow{
				Span:   span.Inert(),
				Offset: span.StartedAt.Sub(trace.StartedAt),
				Open:   span.Duration == 0,
				Last:   true,
			}
			row.Memory, row.HasMemory = span.Attributes.Int64(oida.AttrMemoryUsage)
			if total > 0 {
				row.OffsetShare = share(row.Offset, total)
				row.Share = share(span.Duration, total)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// share returns part as a percentage of total.
func share(part, total time.Duration) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) * 100 / float64(total)
}
