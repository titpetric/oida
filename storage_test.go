package oida

import (
	"context"
	"testing"
)

// TestTracerUsesConfiguredStorage pins the wiring the storage aliases exist
// for: a driver from the storage package, set on Options.Storage, receives
// the finished traces.
func TestTracerUsesConfiguredStorage(t *testing.T) {
	store, err := newStorageDisk(5, t.TempDir())
	if err != nil {
		t.Fatalf("newStorageDisk: %v", err)
	}

	tracer, _ := newTestTracer(t, func(o *Options) { o.Storage = store })
	if err := tracer.Observe(context.Background(), "job", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	length, err := store.Len(context.Background())
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 1 {
		t.Fatalf("storage holds %d traces, want 1", length)
	}
	if traces := tracer.Traces(); len(traces) != 1 || traces[0].Name != "job" {
		t.Fatalf("unexpected traces: %+v", traces)
	}
}
