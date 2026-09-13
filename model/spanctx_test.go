package model

import (
	"context"
	"testing"
	"time"
)

func TestSpanContextResolves(t *testing.T) {
	trace, _ := spanTestTrace()
	type parentKey struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), parentKey{}, "kept"))
	defer cancel()

	ctx, span := trace.StartSpan(parent, "work")
	if got := SpanFromContext(ctx); got != span {
		t.Errorf("SpanFromContext = %v, want the started span", got)
	}
	if got := TraceFromContext(ctx); got != trace {
		t.Errorf("TraceFromContext = %v, want the trace", got)
	}
	if got := ctx.Value(parentKey{}); got != "kept" {
		t.Errorf("parent value = %v, want kept", got)
	}

	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("parent cancellation did not pass through the span context")
	}
}
