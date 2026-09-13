package model

import (
	"context"
	"testing"
	"time"
)

func acquireTestTrace() (*Trace, *time.Time) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	trace := AcquireTrace("t1", "GET /", TraceOptions{Clock: func() time.Time { return now }})
	return trace, &now
}

func TestAcquireTraceInlineSpans(t *testing.T) {
	trace, _ := acquireTestTrace()
	box := trace.box
	if box == nil {
		t.Fatal("an acquired trace carries no box")
	}

	ctx := context.Background()
	for i := range len(box.spans) {
		_, span := trace.StartSpan(ctx, "inline")
		if span != &box.spans[i].span {
			t.Errorf("span %d not recorded in the box slot", i+1)
		}
	}
	_, overflow := trace.StartSpan(ctx, "overflow")
	if overflow == nil {
		t.Fatal("the span past the inline capacity was not recorded")
	}
	for i := range box.spans {
		if overflow == &box.spans[i].span {
			t.Error("the overflow span landed in an inline slot")
		}
	}
	if got := trace.SpanCount(); got != len(box.spans)+1 {
		t.Errorf("SpanCount() = %d, want %d", got, len(box.spans)+1)
	}
}

func TestAcquireTraceRelease(t *testing.T) {
	trace, _ := acquireTestTrace()
	box := trace.box

	trace.SetHTTPInfo(&HTTPInfo{Method: "GET", Host: "acme.example"})
	if trace.HTTP != &box.http {
		t.Error("SetHTTPInfo on a pooled trace did not use the box")
	}
	_, span := trace.StartSpan(context.Background(), "SELECT users", KindDatabase)
	span.End()
	trace.Finish()

	clone := trace.Clone()
	trace.Release()

	if box.trace.ID != "" || box.http.Host != "" || box.used != 0 {
		t.Error("Release left data in the box")
	}
	if clone.box != nil {
		t.Error("the clone kept a box reference")
	}
	if clone.ID != "t1" || clone.HTTP == nil || clone.HTTP.Host != "acme.example" {
		t.Errorf("the clone lost trace data: %+v", clone)
	}
	if len(clone.Spans) != 1 || clone.Spans[0].Name != "SELECT users" {
		t.Errorf("the clone lost span data: %+v", clone.Spans)
	}
}

func TestReleaseUnpooled(t *testing.T) {
	(*Trace)(nil).Release()

	trace, _ := spanTestTrace()
	trace.StartSpan(context.Background(), "work")
	trace.Finish()
	trace.Release()
	if got := trace.SpanCount(); got != 1 {
		t.Errorf("Release on an unpooled trace dropped spans: SpanCount() = %d", got)
	}

	clone := trace.Clone()
	clone.Release()
	if clone.ID != trace.ID {
		t.Error("Release on a clone touched its data")
	}
}
