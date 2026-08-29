package frontend_test

import (
	"testing"
	"time"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
)

// memoryTrace builds a trace whose spans report memory_usage. The spans are
// laid out so that depth first row order and finish order disagree: the last
// child of the root finishes before the deeper branch does.
func memoryTrace() oida.Trace {
	start := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	span := func(id, parent, depth int, name string, offset, duration time.Duration, memory int64) *oida.Span {
		s := &oida.Span{
			ID:        id,
			ParentID:  parent,
			TraceID:   "TRACE",
			Name:      name,
			Kind:      oida.KindInternal,
			Depth:     depth,
			StartedAt: start.Add(offset),
			Duration:  duration,
		}
		if memory > 0 {
			s.Attributes = oida.Attributes{oida.AttrMemoryUsage: memory}
		}
		return s
	}

	return oida.Trace{
		ID:        "TRACE",
		Name:      "GET /users/{id}",
		StartedAt: start,
		Duration:  100 * time.Millisecond,
		Spans: []*oida.Span{
			span(1, 0, 0, "GET /users/1", 0, 100*time.Millisecond, 0),
			span(2, 1, 1, "handler", 10*time.Millisecond, 20*time.Millisecond, 46<<10),
			span(3, 2, 2, "SELECT users", 40*time.Millisecond, 40*time.Millisecond, 92<<10),
			span(4, 1, 1, "render", 15*time.Millisecond, 5*time.Millisecond, 23<<10),
		},
	}
}

// memoryPage runs a trace through the same pipeline the handler uses.
func memoryPage(trace oida.Trace) frontend.Page {
	rows := frontend.Rows(trace)
	return frontend.Page{
		Trace:  &trace,
		Rows:   rows,
		Memory: frontend.TraceMemory(trace, rows),
	}
}

func TestMemorySeriesOrdersByFinish(t *testing.T) {
	series := memoryPage(memoryTrace()).MemorySeries()

	want := []struct {
		name   string
		offset time.Duration
		bytes  int64
		x, y   float64
	}{
		// render walks last in row order but finishes first.
		{"render", 20 * time.Millisecond, 23 << 10, 20, 77},
		{"handler", 30 * time.Millisecond, 46 << 10, 30, 54},
		{"SELECT users", 80 * time.Millisecond, 92 << 10, 80, 8},
	}
	if len(series.Points) != len(want) {
		t.Fatalf("produced %d points, want %d: %+v", len(series.Points), len(want), series.Points)
	}
	for i, expected := range want {
		got := series.Points[i]
		if got.Name != expected.name || got.Offset != expected.offset || got.Bytes != expected.bytes {
			t.Errorf("point %d is %s (%s, %d B), want %s (%s, %d B)",
				i, got.Name, got.Offset, got.Bytes, expected.name, expected.offset, expected.bytes)
		}
		if got.X != expected.x || got.Y != expected.y {
			t.Errorf("point %d sits at %.2f,%.2f, want %.2f,%.2f", i, got.X, got.Y, expected.x, expected.y)
		}
	}

	if series.Ceiling != 92<<10 {
		t.Errorf("ceiling is %d, want the %d peak", series.Ceiling, 92<<10)
	}
	if series.Limited {
		t.Error("drew a limit line with no limit recorded")
	}
	if last := series.Last(); last.Bytes != 92<<10 {
		t.Errorf("last reading is %d bytes, want %d", last.Bytes, 92<<10)
	}
}

func TestMemorySeriesScalesToNearLimit(t *testing.T) {
	trace := memoryTrace()
	trace.Attributes = oida.Attributes{oida.AttrMemoryLimit: int64(184 << 10)}
	series := memoryPage(trace).MemorySeries()

	if !series.Limited || series.Ceiling != 184<<10 {
		t.Fatalf("limited=%v ceiling=%d, want the %d limit drawn", series.Limited, series.Ceiling, 184<<10)
	}
	if series.LimitY != 8 {
		t.Errorf("limit line sits at %.2f, want 8 from the top", series.LimitY)
	}
	// The peak reading is half the limit, so it sits halfway up the scale.
	if last := series.Last(); last.Y != 54 {
		t.Errorf("peak reading sits at %.2f, want 54", last.Y)
	}
}

func TestMemorySeriesIgnoresFarLimit(t *testing.T) {
	trace := memoryTrace()
	trace.Attributes = oida.Attributes{oida.AttrMemoryLimit: int64(1 << 20)}
	series := memoryPage(trace).MemorySeries()

	if series.Limited {
		t.Error("drew a limit line more than twice the peak away")
	}
	if series.Ceiling != 92<<10 {
		t.Errorf("ceiling is %d, want the %d peak", series.Ceiling, 92<<10)
	}
}

func TestMemorySeriesWithoutReadingsIsEmpty(t *testing.T) {
	trace := timelineTrace()
	series := memoryPage(trace).MemorySeries()
	if len(series.Points) != 0 {
		t.Fatalf("produced %d points for a trace with no readings", len(series.Points))
	}
}
