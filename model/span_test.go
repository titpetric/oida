package model

import (
	"context"
	"errors"
	"testing"
	"time"
)

// spanTestTrace returns a trace with a deterministic clock the test can move.
func spanTestTrace() (*Trace, *time.Time) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: func() time.Time { return now }})
	return trace, &now
}

func TestSpansFind(t *testing.T) {
	trace, _ := spanTestTrace()
	_, first := trace.StartSpan(context.Background(), "first")
	_, second := trace.StartSpan(context.Background(), "second")

	if got := trace.Spans.Find(second.ID); got != second {
		t.Errorf("Find(%d) = %v, want the second span", second.ID, got)
	}
	if got := trace.Spans.Find(first.ID); got != first {
		t.Errorf("Find(%d) = %v, want the first span", first.ID, got)
	}
	if got := trace.Spans.Find(0); got != nil {
		t.Errorf("Find(0) = %v, want nil for an entry outside any span", got)
	}
	if got := append(Spans{nil}, trace.Spans...).Find(first.ID); got != first {
		t.Error("Find does not skip a nil span")
	}
	if got := Spans(nil).Find(1); got != nil {
		t.Errorf("Find on a nil slice = %v, want nil", got)
	}
}

func TestSpanEndWithError(t *testing.T) {
	trace, now := spanTestTrace()
	_, span := trace.StartSpan(context.Background(), "SELECT users")

	*now = now.Add(5 * time.Millisecond)
	failure := errors.New("connection reset")
	span.EndWithError(failure)

	if !span.Ended() {
		t.Fatal("the span did not end")
	}
	if span.Duration != 5*time.Millisecond {
		t.Errorf("duration is %v, want 5ms", span.Duration)
	}
	if !errors.Is(span.Err(), failure) {
		t.Errorf("Err() = %v, want the recorded error", span.Err())
	}
	if trace.Err() == nil {
		t.Error("the trace did not record the failure")
	}

	// End is idempotent: a later End does not stretch the duration.
	*now = now.Add(time.Second)
	span.End()
	if span.Duration != 5*time.Millisecond {
		t.Errorf("duration moved to %v after a second End", span.Duration)
	}
}

func TestSpanErrDecoded(t *testing.T) {
	// A span decoded from JSON kept the message and not the value.
	span := &Span{ErrorText: "user 0 does not exist"}
	if err := span.Err(); err == nil || err.Error() != "user 0 does not exist" {
		t.Errorf("Err() = %v, want the decoded message", err)
	}
	if err := (&Span{}).Err(); err != nil {
		t.Errorf("Err() on a clean span = %v, want nil", err)
	}
	if err := (*Span)(nil).Err(); err != nil {
		t.Errorf("Err() on a nil span = %v, want nil", err)
	}
}

func TestSpanInert(t *testing.T) {
	trace, _ := spanTestTrace()
	_, span := trace.StartSpan(context.Background(), "render")
	span.SetAttribute("template", "index.html")

	inert := span.Inert()
	if inert.Trace() != nil {
		t.Error("the inert copy still reaches the trace")
	}
	if inert.Name != "render" || inert.Attributes["template"] != "index.html" {
		t.Errorf("the inert copy lost data: %+v", inert)
	}
	if got := (*Span)(nil).Inert(); got.ID != 0 {
		t.Errorf("Inert on a nil span = %+v, want a zero span", got)
	}
}

func TestSpanElapsed(t *testing.T) {
	trace, now := spanTestTrace()
	_, span := trace.StartSpan(context.Background(), "work")

	*now = now.Add(3 * time.Millisecond)
	if got := span.Elapsed(); got != 3*time.Millisecond {
		t.Errorf("Elapsed while open = %v, want 3ms", got)
	}
	span.End()
	*now = now.Add(time.Second)
	if got := span.Elapsed(); got != 3*time.Millisecond {
		t.Errorf("Elapsed after End = %v, want the recorded 3ms", got)
	}
	if got := (*Span)(nil).Elapsed(); got != 0 {
		t.Errorf("Elapsed on a nil span = %v, want 0", got)
	}
}
