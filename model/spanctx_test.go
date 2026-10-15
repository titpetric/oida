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
		t.Error("parent cancellation does not pass through the span context")
	}
}

// TestSpanContextOutlivesTheBox covers a context a caller keeps past the span.
// A box returns to tracePool on Release and zeroes on the next acquire, so a
// context living inside the box reads a nil parent once another trace draws it,
// and spanCtx promotes Done, Err and Deadline to that parent. A `database/sql`
// driver watching Done() in a goroutine holds the context exactly this way.
func TestSpanContextOutlivesTheBox(t *testing.T) {
	type parentKey struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), parentKey{}, "kept"))
	defer cancel()

	trace, _ := spanTestTrace()
	ctx, span := trace.StartSpan(parent, "query")
	span.End()

	// The recorder's own sequence: the box goes back, and the next trace
	// takes it and clears it.
	trace.Release()
	reused := NewTrace("t2", "GET /next", TraceOptions{})
	if _, next := reused.StartSpan(context.Background(), "other"); next == nil {
		t.Fatal("the reused box started no span")
	}

	// Every method the embedded parent answers, on the context the first
	// request still holds.
	if ctx.Done() == nil {
		t.Error("Done() reads nil, so a driver watching it faults")
	}
	if err := ctx.Err(); err != nil {
		t.Errorf("Err() = %v, want nil while the parent lives", err)
	}
	if _, ok := ctx.Deadline(); ok {
		t.Error("Deadline() reports one the parent does not carry")
	}
	if got := ctx.Value(parentKey{}); got != "kept" {
		t.Errorf("parent value = %v, want kept", got)
	}
	if got := SpanFromContext(ctx); got != span {
		t.Errorf("SpanFromContext = %v, want the span the context names", got)
	}

	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Error("the retained context does not observe its own parent's cancellation")
	}
}

// TestSpanContextSurvivesABoxReset covers the window a box reset opens. reset
// zeroes every spanBox, so a context inside one embeds a nil parent until the
// next derivation writes another. Done() on a nil parent faults, and the
// goroutine a `database/sql` driver parks on Done() evaluates it on its own
// schedule.
func TestSpanContextSurvivesABoxReset(t *testing.T) {
	trace, _ := spanTestTrace()
	ctx, _ := trace.StartSpan(context.Background(), "query")

	// The box goes back and the next trace clears it, with no span started
	// to write a parent back in.
	trace.Release()
	NewTrace("t2", "GET /next", TraceOptions{})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("reading the retained context panics: %v", r)
		}
	}()
	ctx.Done()
	ctx.Err()
	ctx.Deadline()
	ctx.Value(struct{}{})
}

// TestSpanContextPerCall covers two derivations of one span. Each call answers
// a context of its own, so the second leaves the first reading the parent it
// carries.
func TestSpanContextPerCall(t *testing.T) {
	type key struct{}
	trace, _ := spanTestTrace()
	_, span := trace.StartSpan(context.Background(), "work")

	first := span.Context(context.WithValue(context.Background(), key{}, "first"))
	second := span.Context(context.WithValue(context.Background(), key{}, "second"))

	if first == second {
		t.Fatal("two derivations answer one context, so the second rewrites the first")
	}
	if got := first.Value(key{}); got != "first" {
		t.Errorf("the first context reads %v, want first", got)
	}
	if got := second.Value(key{}); got != "second" {
		t.Errorf("the second context reads %v, want second", got)
	}
	if got := SpanFromContext(first); got != span {
		t.Errorf("SpanFromContext(first) = %v, want the span", got)
	}
	if got := SpanFromContext(second); got != span {
		t.Errorf("SpanFromContext(second) = %v, want the span", got)
	}
}
