package model

import (
	"context"
	"testing"
)

func TestNewTraceInlineSpans(t *testing.T) {
	trace, _ := spanTestTrace()
	box := trace.box
	if box == nil {
		t.Fatal("a new trace carries no box")
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

func TestTraceRelease(t *testing.T) {
	trace, _ := spanTestTrace()
	box := trace.box

	trace.SetHTTPInfo(&HTTPInfo{Method: "GET", Host: "acme.example"})
	if trace.HTTP != &box.http {
		t.Error("SetHTTPInfo did not use the box")
	}
	_, span := trace.StartSpan(context.Background(), "SELECT users", KindDatabase)
	span.End()
	trace.Finish()

	clone := trace.Clone()
	trace.Release()

	// The recorded values survive Release: a holder reads them until the
	// box is reused, which is what clears it.
	if trace.ID != "t1" || trace.HTTP == nil || trace.HTTP.Host != "acme.example" {
		t.Errorf("Release cleared the trace: %+v", trace)
	}
	if len(trace.Spans) != 1 || trace.Spans[0].Name != "SELECT users" {
		t.Errorf("Release cleared the spans: %+v", trace.Spans)
	}
	if trace.box != nil {
		t.Error("Release kept the box reference, a second Release would double free")
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

	box.trace = Trace{ID: "dirty"}
	box.used = 3
	if reused := NewTrace("t2", "GET /again", TraceOptions{}); reused.box == box {
		if reused.ID != "t2" || reused.SpanCount() != 0 || reused.box.used != 0 {
			t.Errorf("NewTrace reused a box without clearing it: %+v", reused)
		}
	}
}

func TestReleaseWithoutBox(t *testing.T) {
	(*Trace)(nil).Release()
	(*Trace)(nil).ReleaseOnCollect()

	trace, _ := spanTestTrace()
	trace.StartSpan(context.Background(), "work")
	trace.Finish()

	clone := trace.Clone()
	clone.Release()
	clone.ReleaseOnCollect()
	if clone.ID != trace.ID || len(clone.Spans) != 1 {
		t.Error("Release on a clone touched its data")
	}
}
