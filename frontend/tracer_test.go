package frontend_test

import (
	"sync"
	"testing"
	"time"

	"github.com/titpetric/oida"
)

// newTestTracer returns a tracer with a deterministic clock the test can drive.
func newTestTracer(t *testing.T, apply func(*oida.Options)) (*oida.Tracer, *testClock) {
	t.Helper()

	clock := &testClock{now: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)}
	opts := oida.NewOptions("test")
	opts.TrackMemoryUse = false
	opts.Clock = clock.Now
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }
	if apply != nil {
		apply(&opts)
	}

	tracer, err := oida.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tracer, clock
}

// truncate shortens a value for failure messages.
func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	return value[:width] + "..."
}

// testClock is a deterministic time source.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

// Now returns the current time of the clock.
func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward.
func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
