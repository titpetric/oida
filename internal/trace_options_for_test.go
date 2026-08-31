package internal

import (
	"testing"
	"time"

	"github.com/titpetric/oida/model"
)

func TestTraceOptionsFor(t *testing.T) {
	clock := func() time.Time { return time.Unix(0, 0) }

	opts := model.NewOptions("billing-api")
	opts.MaxSpansPerTrace = 7
	opts.CaptureLogs = false
	opts.Clock = clock

	got := TraceOptionsFor(opts)
	if got.Service != "billing-api" {
		t.Errorf("Service is %q", got.Service)
	}
	if got.MaxSpans != 7 {
		t.Errorf("MaxSpans is %d, want 7", got.MaxSpans)
	}
	if got.CaptureLogs {
		t.Error("CaptureLogs stayed on")
	}
	if got.Clock == nil || !got.Clock().Equal(time.Unix(0, 0)) {
		t.Error("the clock did not come along")
	}
}
