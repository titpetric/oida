package model

import (
	"context"
	"testing"
	"time"
)

// logClock returns a deterministic clock that steps one millisecond per call.
func logClock() func() time.Time {
	at := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		at = at.Add(time.Millisecond)
		return at
	}
}

func TestTraceCurrentReturnsInnermostOpenSpan(t *testing.T) {
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: logClock(), CaptureLogs: true})
	if trace.Current() != nil {
		t.Fatal("a trace without spans reports a current span")
	}

	ctx, outer := trace.StartSpan(context.Background(), "outer")
	if got := trace.Current(); got != outer {
		t.Fatalf("current is %v, want the outer span", got)
	}

	_, inner := trace.StartSpan(ctx, "inner")
	if got := trace.Current(); got != inner {
		t.Fatalf("current is %v, want the inner span", got)
	}

	inner.End()
	if got := trace.Current(); got != outer {
		t.Fatalf("after ending the inner span, current is %v, want the outer span", got)
	}

	outer.End()
	if trace.Current() != nil {
		t.Fatal("a trace with only ended spans reports a current span")
	}

	var nilTrace *Trace
	if nilTrace.Current() != nil {
		t.Fatal("a nil trace reports a current span")
	}
}

func TestTraceLogsLinkToTheActiveSpan(t *testing.T) {
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: logClock(), CaptureLogs: true})

	trace.Info("before any span")

	ctx, outer := trace.StartSpan(context.Background(), "outer")
	trace.Info("inside outer", "key", "value")

	_, inner := trace.StartSpan(ctx, "inner")
	trace.Error("inside inner")
	outer.Info("pinned to outer")

	inner.End()
	outer.End()
	trace.Info("after the spans")

	logs := trace.Logs
	if len(logs) != 5 {
		t.Fatalf("recorded %d entries, want 5", len(logs))
	}

	if logs[0].SpanID != 0 {
		t.Errorf("the entry before any span links to span %d, want 0", logs[0].SpanID)
	}
	if logs[1].SpanID != outer.ID || logs[1].Level != LevelInfo {
		t.Errorf("entry %+v, want info on span %d", logs[1], outer.ID)
	}
	if got, ok := logs[1].Attributes["key"]; !ok || got != "value" {
		t.Errorf("entry attributes %+v, want key=value", logs[1].Attributes)
	}
	if logs[2].SpanID != inner.ID || logs[2].Level != LevelError {
		t.Errorf("entry %+v, want error on span %d", logs[2], inner.ID)
	}
	if logs[3].SpanID != outer.ID {
		t.Errorf("the span entry links to span %d, want %d", logs[3].SpanID, outer.ID)
	}
	if logs[4].SpanID != 0 {
		t.Errorf("the entry after the spans links to span %d, want 0", logs[4].SpanID)
	}

	if !logs[0].Time.Before(logs[4].Time) {
		t.Error("entries do not carry the trace clock")
	}
}

func TestTraceLogArguments(t *testing.T) {
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: logClock(), CaptureLogs: true})

	trace.Info("plain")
	trace.Info("pairs", "a", 1, "b", "two")
	trace.Info("dangling", "a", 1, "oops")
	trace.Info("odd key", 42, "answer")

	logs := trace.Logs
	if logs[0].Attributes != nil {
		t.Errorf("a call without args carries attributes: %+v", logs[0].Attributes)
	}
	if got := logs[1].Attributes; got["a"] != 1 || got["b"] != "two" {
		t.Errorf("pairs read as %+v", got)
	}
	if got := logs[2].Attributes; got["a"] != 1 || got["!BADKEY"] != "oops" {
		t.Errorf("a dangling argument reads as %+v", got)
	}
	if got := logs[3].Attributes; got["42"] != "answer" {
		t.Errorf("a non-string key reads as %+v", got)
	}
}

func TestTraceLogCapAndDroppedCounter(t *testing.T) {
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: logClock(), CaptureLogs: true, MaxSpans: 2})

	trace.Info("one")
	trace.Info("two")
	trace.Info("three")
	trace.Error("four")

	if len(trace.Logs) != 2 {
		t.Fatalf("recorded %d entries, want 2", len(trace.Logs))
	}
	if trace.DroppedLogs != 2 {
		t.Fatalf("dropped %d entries, want 2", trace.DroppedLogs)
	}

	trace.Finish()
	trace.Info("after finish")
	if len(trace.Logs) != 2 || trace.DroppedLogs != 2 {
		t.Error("a finished trace still records log entries")
	}
}

func TestTraceLogsSurviveClone(t *testing.T) {
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: logClock(), CaptureLogs: true})
	_, span := trace.StartSpan(context.Background(), "work")
	span.Info("working", "step", 1)
	span.End()
	trace.Finish()

	copied := trace.Clone()
	if len(copied.Logs) != 1 {
		t.Fatalf("the clone carries %d entries, want 1", len(copied.Logs))
	}
	if copied.Logs[0].SpanID != span.ID || copied.Logs[0].Message != "working" {
		t.Fatalf("the clone carries %+v", copied.Logs[0])
	}

	// The copy is deep: mutating it cannot reach the recorded trace.
	copied.Logs[0].Attributes["step"] = 2
	if trace.Logs[0].Attributes["step"] != 1 {
		t.Error("the cloned attributes share storage with the trace")
	}
}

func TestTraceLogsCarryTheRequestID(t *testing.T) {
	trace := NewTrace("01TESTID", "GET /", TraceOptions{Clock: logClock(), CaptureLogs: true})
	trace.Info("one")
	if got := trace.Logs[0].RequestID; got != "01TESTID" {
		t.Fatalf("entry carries request id %q, want the trace id", got)
	}
}

func TestLogCaptureDisabled(t *testing.T) {
	trace := NewTrace("t1", "GET /", TraceOptions{Clock: logClock()})
	_, span := trace.StartSpan(context.Background(), "work")

	// Info does nothing at all.
	trace.Info("ignored")
	span.Info("ignored")
	if len(trace.Logs) != 0 || trace.DroppedLogs != 0 {
		t.Fatalf("a disabled trace recorded entries: %+v", trace.Logs)
	}

	// Error records its formatted text through RecordError on the active span.
	trace.Error("charge failed", "invoice", 42)
	if len(trace.Logs) != 0 {
		t.Fatalf("a disabled trace recorded entries: %+v", trace.Logs)
	}
	if span.ErrorText != "charge failed invoice=42" {
		t.Fatalf("the span records %q, want the formatted text", span.ErrorText)
	}
	if trace.State != StateError {
		t.Fatal("the disabled Error did not go through RecordError")
	}
	span.End()

	// With no span open, the error lands on the trace.
	other := NewTrace("t2", "GET /", TraceOptions{Clock: logClock()})
	other.Error("no rows")
	if other.ErrorText != "no rows" || other.State != StateError {
		t.Fatalf("the trace records %q in state %s, want the message in StateError", other.ErrorText, other.State)
	}
}

func TestLogMethodsAreNilSafe(t *testing.T) {
	var trace *Trace
	trace.Info("nothing")
	trace.Error("nothing")

	var span *Span
	span.Info("nothing")
	span.Error("nothing")

	// A span detached from any trace is also a no-op.
	detached := &Span{ID: 1}
	detached.Info("nothing")
	detached.Error("nothing")
}
