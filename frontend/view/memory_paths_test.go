package view

import (
	"testing"
	"time"

	"github.com/titpetric/oida/model"
)

// pathsTrace builds the memory reporting trace of memory_test.go with the
// model types this package reads, so the unexported path builders can be
// tested from inside the package without importing the root package.
func pathsTrace() model.Trace {
	start := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	span := func(id, parent, depth int, name string, offset, duration time.Duration, memory int64) *model.Span {
		s := &model.Span{
			ID:        id,
			ParentID:  parent,
			TraceID:   "TRACE",
			Name:      name,
			Kind:      model.KindInternal,
			Depth:     depth,
			StartedAt: start.Add(offset),
			Duration:  duration,
		}
		if memory > 0 {
			s.Attributes = model.Attributes{model.AttrMemoryUsage: memory}
		}
		return s
	}

	return model.Trace{
		ID:        "TRACE",
		Name:      "GET /users/{id}",
		StartedAt: start,
		Duration:  100 * time.Millisecond,
		Spans: []*model.Span{
			span(1, 0, 0, "GET /users/1", 0, 100*time.Millisecond, 0),
			span(2, 1, 1, "handler", 10*time.Millisecond, 20*time.Millisecond, 46<<10),
			span(3, 2, 2, "SELECT users", 40*time.Millisecond, 40*time.Millisecond, 92<<10),
			span(4, 1, 1, "render", 15*time.Millisecond, 5*time.Millisecond, 23<<10),
		},
	}
}

func TestMemoryPathsStepThroughReadings(t *testing.T) {
	trace := pathsTrace()
	rows := Rows(trace)
	series := Page{Trace: &trace, Rows: rows, Memory: TraceMemory(trace, rows)}.MemorySeries()

	// The line runs the whole trace: it opens at the first reading's level
	// and holds the last level to the end.
	if got, want := series.linePath(), "M 0 77.00 H 20.00 V 77.00 H 30.00 V 54.00 H 80.00 V 8.00 H 100"; got != want {
		t.Errorf("line path is %q, want %q", got, want)
	}
	if got, want := series.areaPath(), "M 0 100 V 77.00 H 20.00 V 77.00 H 30.00 V 54.00 H 80.00 V 8.00 H 100 V 100 Z"; got != want {
		t.Errorf("area path is %q, want %q", got, want)
	}

	// Hover targets tile the line: each reading owns the stretch behind it,
	// and the last owns the run-out to the end of the trace.
	if got := series.hitX(0); got != "0" {
		t.Errorf("first hit starts at %s, want 0", got)
	}
	if got := series.hitWidth(2); got != "70.00" {
		t.Errorf("last hit is %s wide, want 70.00", got)
	}

	// The axis reads the level under any share: the newest reading taken by
	// then, the first before any was, the last past the end.
	if got := series.at(0); got != series.Points[0].Bytes {
		t.Errorf("at(0) is %d, want the first reading %d", got, series.Points[0].Bytes)
	}
	if got := series.at(50); got != series.Points[1].Bytes {
		t.Errorf("at(50) is %d, want the second reading %d", got, series.Points[1].Bytes)
	}
	if got := series.at(100); got != series.Points[2].Bytes {
		t.Errorf("at(100) is %d, want the last reading %d", got, series.Points[2].Bytes)
	}
}
