package model

import (
	"context"
	"testing"
	"time"
)

// debugTrace returns a trace that drops debug spans under a millisecond, with
// a clock the test moves: standing still, every span ends one nanosecond long
// and the filter takes it.
func debugTrace(maxSpans int) (*Trace, *time.Time) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	trace := NewTrace("t1", "GET /runs", TraceOptions{
		Clock:                func() time.Time { return now },
		MaxSpans:             maxSpans,
		DebugSpanMinDuration: time.Millisecond,
	})
	return trace, &now
}

func TestDebugSpanShortIsDropped(t *testing.T) {
	trace, _ := debugTrace(0)
	span := StartDebugSpan(WithTrace(context.Background(), trace), "Row::text")
	span.End()

	if got := trace.SpanCount(); got != 0 {
		t.Errorf("SpanCount = %d, want 0: a debug span under the minimum leaves the trace", got)
	}
	if trace.DroppedSpans != 1 {
		t.Errorf("DroppedSpans = %d, want 1", trace.DroppedSpans)
	}
	// The span value itself is untouched: a caller holding it still reads
	// what it measured, and ending it twice counts one drop.
	if span.Duration <= 0 || !span.Ended() {
		t.Errorf("Duration = %v, ended = %v, want a measured, ended span", span.Duration, span.Ended())
	}
	span.End()
	if trace.DroppedSpans != 1 {
		t.Errorf("DroppedSpans = %d after a second End, want 1", trace.DroppedSpans)
	}
}

func TestDebugSpanSlowIsKept(t *testing.T) {
	trace, now := debugTrace(0)
	_, span := trace.StartSpan(context.Background(), "mod.serve")
	span.SetDebug()
	*now = now.Add(2 * time.Millisecond)
	span.End()

	if got := trace.SpanCount(); got != 1 {
		t.Errorf("SpanCount = %d, want 1: a debug span over the minimum stays", got)
	}
	if trace.DroppedSpans != 0 {
		t.Errorf("DroppedSpans = %d, want 0", trace.DroppedSpans)
	}
}

// TestDebugSpanNormalIsKept holds the line the filter must not cross: a span
// nobody marked is recorded whatever it cost.
func TestDebugSpanNormalIsKept(t *testing.T) {
	trace, _ := debugTrace(0)
	_, span := trace.StartSpan(context.Background(), "SELECT 1", KindDatabase)
	span.End()

	if got := trace.SpanCount(); got != 1 {
		t.Errorf("SpanCount = %d, want 1: the filter is for debug spans alone", got)
	}
	if trace.DroppedSpans != 0 {
		t.Errorf("DroppedSpans = %d, want 0", trace.DroppedSpans)
	}
}

func TestDebugSpanNoMinimumKeepsEverything(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: func() time.Time { return now }})
	span := StartDebugSpan(WithTrace(context.Background(), trace), "Row::text")
	span.End()

	if got := trace.SpanCount(); got != 1 {
		t.Errorf("SpanCount = %d, want 1: a zero minimum filters nothing", got)
	}
}

// TestDebugSpanFreesTheLimit is why the filter drops instead of marking: the
// span limit is what a trace of an instrumented interpreter runs out of, and
// a dropped span gives its slot back.
func TestDebugSpanFreesTheLimit(t *testing.T) {
	trace, _ := debugTrace(2)
	for range 10 {
		span := StartDebugSpan(WithTrace(context.Background(), trace), "Row::text")
		span.End()
	}
	_, kept := trace.StartSpan(context.Background(), "SELECT runs", KindDatabase)

	if kept == nil {
		t.Fatal("the trace hit its span limit on spans it had already dropped")
	}
	if got := trace.SpanCount(); got != 1 {
		t.Errorf("SpanCount = %d, want 1", got)
	}
	if trace.DroppedSpans != 10 {
		t.Errorf("DroppedSpans = %d, want 10", trace.DroppedSpans)
	}
}

// TestDebugSpanKeepsChildren drops a span with a recorded child, which is the
// one case the removal shifts the list for. The child stays, orphaned the way
// the span limit already orphans one, and the front end renders it.
func TestDebugSpanKeepsChildren(t *testing.T) {
	trace, _ := debugTrace(0)
	ctx, parent := trace.StartSpan(context.Background(), "page.draw")
	parent.SetDebug()
	_, child := trace.StartSpan(ctx, "SELECT runs", KindDatabase)
	child.End()
	parent.End()

	if got := trace.SpanCount(); got != 1 {
		t.Fatalf("SpanCount = %d, want 1: the child outlives the frame that was dropped", got)
	}
	if got := trace.Spans[0]; got != child {
		t.Errorf("Spans[0] = %v, want the child", got.Name)
	}
	if child.ParentID != parent.ID {
		t.Errorf("ParentID = %d, want %d: the child still names where it ran", child.ParentID, parent.ID)
	}
}

// TestDebugSpanSurvivesFinish states what a finished trace does: Finish ends
// every span still open while it walks the list, so nothing is removed from
// under it and an unclosed debug span is reported rather than dropped.
func TestDebugSpanSurvivesFinish(t *testing.T) {
	trace, _ := debugTrace(0)
	span := StartDebugSpan(WithTrace(context.Background(), trace), "mod.serve")
	trace.Finish()

	if got := trace.SpanCount(); got != 1 {
		t.Errorf("SpanCount = %d, want 1: a finished trace keeps what it holds", got)
	}
	if !span.Ended() {
		t.Error("Finish did not end the span")
	}
}

// TestSetDebugNilSpan holds the package rule that every span method tolerates
// a nil receiver: a producer marks what it starts without asking whether the
// request was traced at all.
func TestSetDebugNilSpan(t *testing.T) {
	var span *Span
	span.SetDebug()
	span.End()
}

func TestStartDebugMarks(t *testing.T) {
	trace, _ := debugTrace(0)
	ctx, span := StartDebug(WithTrace(context.Background(), trace), "loader.load")
	if !span.Debug {
		t.Error("StartDebug did not mark the span")
	}
	if SpanFromContext(ctx) != span {
		t.Error("StartDebug did not derive a context carrying the span")
	}
	if StartDebugSpan(context.Background(), "no trace") != nil {
		t.Error("a context with no trace recorded a span")
	}
}
