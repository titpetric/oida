package model

import (
	"context"
	"testing"
)

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
