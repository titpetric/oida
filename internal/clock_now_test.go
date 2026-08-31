package internal

import (
	"testing"
	"time"

	"github.com/titpetric/oida/model"
)

func TestClockNow(t *testing.T) {
	fixed := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

	opts := model.NewOptions("test")
	opts.Clock = func() time.Time { return fixed }
	if got := ClockNow(opts); !got.Equal(fixed) {
		t.Errorf("ClockNow() = %v, want the configured clock", got)
	}

	// Without a clock the wall clock answers, which is what a service that
	// configured none gets.
	before := time.Now()
	got := ClockNow(model.Options{})
	if got.Before(before) || time.Since(got) > time.Minute {
		t.Errorf("ClockNow() = %v, want a time around now", got)
	}
}
