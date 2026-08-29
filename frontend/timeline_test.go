package frontend_test

import (
	"testing"
	"time"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
)

// timelineTrace builds a trace with nested spans at fixed offsets.
func timelineTrace() oida.Trace {
	start := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	span := func(id, parent, depth int, name string, kind oida.Kind, offset, duration time.Duration) *oida.Span {
		return &oida.Span{
			ID:        id,
			ParentID:  parent,
			TraceID:   "TRACE",
			Name:      name,
			Kind:      kind,
			Depth:     depth,
			StartedAt: start.Add(offset),
			Duration:  duration,
		}
	}

	return oida.Trace{
		ID:        "TRACE",
		Name:      "GET /users/{id}",
		StartedAt: start,
		Duration:  100 * time.Millisecond,
		Spans: []*oida.Span{
			span(1, 0, 0, "GET /users/1", oida.KindHTTP, 0, 100*time.Millisecond),
			span(2, 1, 1, "handler", oida.KindInternal, 10*time.Millisecond, 70*time.Millisecond),
			span(3, 2, 2, "SELECT users", oida.KindDatabase, 20*time.Millisecond, 30*time.Millisecond),
			span(4, 1, 1, "render", oida.KindTemplate, 80*time.Millisecond, 20*time.Millisecond),
		},
	}
}

func TestTimelineAttributesInnermostSpan(t *testing.T) {
	segments := frontend.Timeline(timelineTrace())

	want := []struct {
		kind     oida.Kind
		offset   time.Duration
		duration time.Duration
	}{
		{oida.KindInternal, 10 * time.Millisecond, 10 * time.Millisecond},
		{oida.KindDatabase, 20 * time.Millisecond, 30 * time.Millisecond},
		{oida.KindInternal, 50 * time.Millisecond, 30 * time.Millisecond},
		{oida.KindTemplate, 80 * time.Millisecond, 20 * time.Millisecond},
	}
	if len(segments) != len(want) {
		t.Fatalf("produced %d segments, want %d: %+v", len(segments), len(want), segments)
	}
	for i, expected := range want {
		got := segments[i]
		if got.Kind != expected.kind || got.Offset != expected.offset || got.Duration != expected.duration {
			t.Errorf("segment %d is %s at %s for %s, want %s at %s for %s",
				i, got.Kind, got.Offset, got.Duration, expected.kind, expected.offset, expected.duration)
		}
	}
	if segments[1].Share != 30 || segments[1].OffsetShare != 20 {
		t.Errorf("database share is %.2f%% at %.2f%%, want 30%% at 20%%", segments[1].Share, segments[1].OffsetShare)
	}
}

func TestTimelineWithoutDurationIsEmpty(t *testing.T) {
	trace := timelineTrace()
	trace.Duration = 0
	if segments := frontend.Timeline(trace); segments != nil {
		t.Fatalf("produced %d segments for an unfinished trace", len(segments))
	}
}

func TestRowsAreDepthFirst(t *testing.T) {
	rows := frontend.Rows(timelineTrace())

	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	want := []string{"GET /users/1", "handler", "SELECT users", "render"}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("rows are %v, want %v", names, want)
		}
	}

	if rows[2].Depth != 2 || rows[2].OffsetShare != 20 || rows[2].Share != 30 {
		t.Errorf("database row: depth %d, offset %.2f%%, share %.2f%%", rows[2].Depth, rows[2].OffsetShare, rows[2].Share)
	}
	if !rows[3].Last || rows[1].Last {
		t.Errorf("last-child flags are wrong: handler=%v render=%v", rows[1].Last, rows[3].Last)
	}
}
