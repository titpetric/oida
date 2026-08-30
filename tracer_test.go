package oida

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// newTestTracer returns a tracer with a deterministic clock the test can drive.
func newTestTracer(t *testing.T, apply func(*Options)) (*Tracer, *testClock) {
	t.Helper()

	clock := &testClock{now: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)}
	opts := NewOptions("test")
	opts.Enabled = true
	opts.TrackMemoryUse = false
	// A unit test reads no environment: the deployment settings of whoever
	// runs it are not part of what is under test.
	opts.ReadEnv = false
	opts.Clock = clock.Now
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }
	if apply != nil {
		apply(&opts)
	}

	tracer, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tracer, clock
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

func TestObserveRecordsTrace(t *testing.T) {
	tracer, clock := newTestTracer(t, nil)

	err := tracer.Observe(context.Background(), "job", func(ctx context.Context) error {
		ctx, span := Start(ctx, "query", KindDatabase)
		clock.Advance(3 * time.Millisecond)
		span.SetAttribute("rows", 12)
		span.End()

		_, child := Start(ctx, "render", KindTemplate)
		child.End()
		return nil
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	traces := tracer.Traces()
	if len(traces) != 1 {
		t.Fatalf("recorded %d traces, want 1", len(traces))
	}

	trace := traces[0]
	if trace.Name != "job" || trace.Duration != 3*time.Millisecond {
		t.Fatalf("unexpected trace: %+v", trace)
	}
	if len(trace.Spans) != 3 {
		t.Fatalf("recorded %d spans, want 3", len(trace.Spans))
	}
	if got := trace.Spans[1]; got.Kind != KindDatabase || got.Duration != 3*time.Millisecond || got.Attributes["rows"] != 12 {
		t.Fatalf("unexpected database span: %+v", got)
	}
	// The callback rebound ctx to the query span, so render nests below it.
	if trace.Spans[2].ParentID != trace.Spans[1].ID || trace.Spans[2].Depth != 2 {
		t.Fatalf("render span parented to %d at depth %d, want %d at depth 2",
			trace.Spans[2].ParentID, trace.Spans[2].Depth, trace.Spans[1].ID)
	}
}

func TestObserveRecordsError(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)
	want := errors.New("boom")

	err := tracer.Observe(context.Background(), "job", func(ctx context.Context) error {
		_, span := Start(ctx, "step")
		span.EndWithError(want)
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("Observe returned %v, want %v", err, want)
	}

	traces := tracer.Traces()
	if len(traces) != 1 || traces[0].ErrorText != want.Error() || traces[0].State != StateError {
		t.Fatalf("unexpected trace: %+v", traces)
	}
}

func TestSpanLimitDropsExcess(t *testing.T) {
	tracer, _ := newTestTracer(t, func(o *Options) { o.MaxSpansPerTrace = 3 })

	_ = tracer.Observe(context.Background(), "job", func(ctx context.Context) error {
		for range 10 {
			StartSpan(ctx, "step").End()
		}
		return nil
	})

	trace := tracer.Traces()[0]
	if len(trace.Spans) != 3 {
		t.Fatalf("recorded %d spans, want 3", len(trace.Spans))
	}
	if trace.DroppedSpans != 8 {
		t.Fatalf("dropped %d spans, want 8", trace.DroppedSpans)
	}
}

func TestNilSafety(t *testing.T) {
	ctx, span := Start(context.Background(), "detached", KindDatabase)
	if span != nil {
		t.Fatalf("Start without a trace returned %+v", span)
	}

	// None of these may panic.
	span.SetAttribute("key", "value")
	span.SetAttributes(Attributes{"other": 1})
	span.SetSource("file.go", 12)
	span.RecordError(errors.New("ignored"))
	span.EndWithError(nil)
	span.End()

	if TraceID(ctx) != "" || TraceFromContext(ctx) != nil {
		t.Fatal("context gained a trace")
	}
	if span.Elapsed() != 0 || span.Err() != nil || span.Ended() {
		t.Fatal("nil span reported state")
	}
}

func TestSnapshotCountsAndReset(t *testing.T) {
	tracer, clock := newTestTracer(t, func(o *Options) { o.RingBufferSize = 2 })

	for range 4 {
		_ = tracer.Observe(context.Background(), "job", func(context.Context) error {
			clock.Advance(time.Millisecond)
			return nil
		})
	}

	snapshot := tracer.Snapshot()
	if snapshot.Sampled != 4 {
		t.Fatalf("sampled %d traces, want 4", snapshot.Sampled)
	}
	if len(snapshot.Log) != 2 {
		t.Fatalf("retained %d traces, want 2", len(snapshot.Log))
	}
	if snapshot.Dropped != 2 {
		t.Fatalf("dropped %d traces, want 2", snapshot.Dropped)
	}
	if snapshot.Statistics.Top[0].Count != 2 || snapshot.Statistics.Top[0].Name != "job" {
		t.Fatalf("unexpected statistics: %+v", snapshot.Statistics.Top)
	}

	tracer.Reset()
	if traces := tracer.Traces(); len(traces) != 0 {
		t.Fatalf("reset left %d traces", len(traces))
	}
}

func TestDisabledTracerRecordsNothing(t *testing.T) {
	tracer, _ := newTestTracer(t, func(o *Options) { o.Enabled = false })

	err := tracer.Observe(context.Background(), "job", func(ctx context.Context) error {
		if TraceFromContext(ctx) != nil {
			t.Error("disabled tracer created a trace")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if traces := tracer.Traces(); len(traces) != 0 {
		t.Fatalf("disabled tracer recorded %d traces", len(traces))
	}
}

func TestConcurrentSpansAndSnapshots(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	err := tracer.Observe(context.Background(), "fan out", func(ctx context.Context) error {
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, span := Start(ctx, "worker", KindInternal)
				span.SetAttribute("worker", i)
				span.End()
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				tracer.Snapshot()
			}
		}()
		wg.Wait()
		return nil
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	if got := len(tracer.Traces()[0].Spans); got != 9 {
		t.Fatalf("recorded %d spans, want 9", got)
	}
}
