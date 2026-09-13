package model

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestTraceAttribute(t *testing.T) {
	trace, _ := spanTestTrace()
	trace.SetAttribute("tenant_id", "acme-eu")

	if value, ok := trace.Attribute("tenant_id"); !ok || value != "acme-eu" {
		t.Errorf("Attribute(tenant_id) = %v, %v", value, ok)
	}
	if _, ok := trace.Attribute("missing"); ok {
		t.Error("a missing attribute reports recorded")
	}
	if _, ok := (*Trace)(nil).Attribute("tenant_id"); ok {
		t.Error("a nil trace reports a recorded attribute")
	}
}

func TestTraceKinds(t *testing.T) {
	trace, _ := spanTestTrace()
	ctx := context.Background()
	trace.StartSpan(ctx, "SELECT users", KindDatabase)
	trace.StartSpan(ctx, "SELECT orders", KindDatabase)
	trace.StartSpan(ctx, "GET pricing", KindExternal)

	kinds := trace.Kinds()
	if len(kinds) != 2 || kinds[0] != KindDatabase || kinds[1] != KindExternal {
		t.Errorf("Kinds() = %v, want database then external in first use order", kinds)
	}

	if !trace.HasKind(KindDatabase) {
		t.Error("HasKind misses a recorded kind")
	}
	if trace.HasKind(KindCache) {
		t.Error("HasKind reports a kind no span recorded")
	}
	if (*Trace)(nil).HasKind(KindDatabase) {
		t.Error("a nil trace reports a kind")
	}
}

// TestTraceClockIncrementOnly drives a trace with the clock standing still,
// which is what sub-tick work on Windows looks like: recorded readings must
// strictly increase, so spans order and measure, while observations such as
// Elapsed read without advancing what the next recording sees.
func TestTraceClockIncrementOnly(t *testing.T) {
	trace, _ := spanTestTrace()
	ctx := context.Background()

	_, first := trace.StartSpan(ctx, "first")
	_, second := trace.StartSpan(ctx, "second")
	if !second.StartedAt.After(first.StartedAt) {
		t.Errorf("sibling starts tie: %v and %v", first.StartedAt, second.StartedAt)
	}
	second.End()
	if second.Duration <= 0 {
		t.Errorf("Duration = %v, want positive under a stalled clock", second.Duration)
	}

	// Observations do not advance the clock: repeated readings agree, and
	// the recording after them is not pushed forward by their count.
	before := trace.Elapsed()
	for range 100 {
		trace.Elapsed()
	}
	if got := trace.Elapsed(); got != before {
		t.Errorf("Elapsed moved from %v to %v under a stalled clock", before, got)
	}
	first.End()
	if first.Duration > 10*clockStep {
		t.Errorf("observations advanced the clock: Duration = %v", first.Duration)
	}
}

// TestTraceFinishCoarseClock finishes a trace with the clock standing still.
// A zero duration reads as still running, Elapsed falls back to the wall
// clock and Clone recomputes it, so a finished trace reports at least a
// nanosecond and stays finished as time passes.
func TestTraceFinishCoarseClock(t *testing.T) {
	trace, now := spanTestTrace()
	trace.Finish()

	if trace.Duration <= 0 {
		t.Errorf("Duration = %v, want at least a nanosecond so the trace reads as finished", trace.Duration)
	}
	*now = now.Add(time.Second)
	if got := trace.Elapsed(); got != trace.Duration {
		t.Errorf("Elapsed after Finish = %v, want the recorded %v", got, trace.Duration)
	}
	if clone := trace.Clone(); clone.Duration != trace.Duration || clone.InFlight {
		t.Errorf("Clone = {Duration: %v, InFlight: %v}, want the recorded duration, not in flight", clone.Duration, clone.InFlight)
	}
}

func TestTraceElapsed(t *testing.T) {
	trace, now := spanTestTrace()

	*now = now.Add(7 * time.Millisecond)
	if got := trace.Elapsed(); got != 7*time.Millisecond {
		t.Errorf("Elapsed in flight = %v, want 7ms", got)
	}

	trace.Finish()
	*now = now.Add(time.Second)
	if got := trace.Elapsed(); got != 7*time.Millisecond {
		t.Errorf("Elapsed after Finish = %v, want the recorded 7ms", got)
	}
	if got := (*Trace)(nil).Elapsed(); got != 0 {
		t.Errorf("Elapsed on a nil trace = %v, want 0", got)
	}
}

// TestSignedDelta covers a heap that shrank as well as one that grew, and both
// saturation points.
//
// The one caller reads runtime.MemStats twice and subtracts, so which branch a
// run lands on is the allocator's business rather than the test's: without
// this the coverage of the function is whatever the memory happened to do.
func TestSignedDelta(t *testing.T) {
	tests := []struct {
		after, before uint64
		want          int64
	}{
		{after: 0, before: 0, want: 0},
		{after: 300, before: 100, want: 200},
		{after: 100, before: 300, want: -200},
		// A growth wider than an int64 saturates rather than wrapping into a
		// shrink, and a shrink that wide saturates the other way.
		{after: math.MaxUint64, before: 0, want: math.MaxInt64},
		{after: 0, before: math.MaxUint64, want: math.MinInt64},
		{after: math.MaxInt64, before: 0, want: math.MaxInt64},
		{after: 0, before: math.MaxInt64, want: -math.MaxInt64},
	}

	for _, test := range tests {
		if got := signedDelta(test.after, test.before); got != test.want {
			t.Errorf("signedDelta(%d, %d) = %d, want %d", test.after, test.before, got, test.want)
		}
	}
}

// TestDelta covers the clamp, which the process only reaches when a counter
// that is meant to rise did not.
func TestDelta(t *testing.T) {
	tests := []struct {
		after, before uint64
		want          uint64
	}{
		{after: 300, before: 100, want: 200},
		{after: 100, before: 300, want: 0},
		{after: 0, before: 0, want: 0},
	}

	for _, test := range tests {
		if got := delta(test.after, test.before); got != test.want {
			t.Errorf("delta(%d, %d) = %d, want %d", test.after, test.before, got, test.want)
		}
	}
}
