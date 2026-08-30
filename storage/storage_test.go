package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/titpetric/oida/model"
)

// storageFactory builds one storage implementation for the shared suite.
type storageFactory struct {
	name  string
	build func(t *testing.T, limit int) model.Storage
}

// storageFactories lists every implementation the suite runs against.
var storageFactories = []storageFactory{
	{
		name:  "memory",
		build: func(_ *testing.T, limit int) model.Storage { return NewMemoryStorage(limit) },
	},
	{
		name: "disk",
		build: func(t *testing.T, limit int) model.Storage {
			storage, err := NewDiskStorage(limit, t.TempDir())
			if err != nil {
				t.Fatalf("NewDisk: %v", err)
			}
			return storage
		},
	},
}

// storedTrace returns a completed trace with a deterministic ID.
func storedTrace(t *testing.T, name string, at time.Time) model.Trace {
	t.Helper()

	id, err := model.NewID(at)
	if err != nil {
		t.Fatalf("model.NewID: %v", err)
	}
	return model.Trace{
		ID:        id,
		Name:      name,
		State:     model.StateWriting,
		StartedAt: at,
		Duration:  time.Millisecond,
		Spans: []*model.Span{
			{ID: 1, TraceID: id, Name: name, Kind: model.KindHTTP, StartedAt: at, Duration: time.Millisecond},
		},
	}
}

func TestStorage(t *testing.T) {
	for _, factory := range storageFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			storage := factory.build(t, 3)

			at := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
			traces := make([]model.Trace, 0, 5)
			for i := range 5 {
				trace := storedTrace(t, "GET /users", at.Add(time.Duration(i)*time.Second))
				if err := storage.Save(ctx, trace); err != nil {
					t.Fatalf("Save: %v", err)
				}
				traces = append(traces, trace)
			}

			if got := storage.Cap(); got != 3 {
				t.Fatalf("Cap is %d, want 3", got)
			}
			length, err := storage.Len(ctx)
			if err != nil {
				t.Fatalf("Len: %v", err)
			}
			if length != 3 {
				t.Fatalf("retained %d traces, want 3", length)
			}

			listed, err := storage.List(ctx, 0)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(listed) != 3 || listed[0].ID != traces[4].ID || listed[2].ID != traces[2].ID {
				t.Fatalf("unexpected listing: %v", ids(listed))
			}

			limited, err := storage.List(ctx, 2)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(limited) != 2 {
				t.Fatalf("limited listing returned %d traces, want 2", len(limited))
			}

			loaded, err := storage.Load(ctx, traces[4].ID)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if loaded.Name != "GET /users" || len(loaded.Spans) != 1 {
				t.Fatalf("unexpected trace: %+v", loaded)
			}

			if _, err := storage.Load(ctx, traces[0].ID); !errors.Is(err, model.ErrTraceNotFound) {
				t.Fatalf("evicted trace returned %v, want model.ErrTraceNotFound", err)
			}

			if err := storage.Reset(ctx); err != nil {
				t.Fatalf("Reset: %v", err)
			}
			if length, _ := storage.Len(ctx); length != 0 {
				t.Fatalf("reset left %d traces", length)
			}
		})
	}
}

func TestMemoryZeroSizeRetainsNothing(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage(0)

	if err := storage.Save(ctx, storedTrace(t, "job", time.Now())); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if length, _ := storage.Len(ctx); length != 0 {
		t.Fatalf("retained %d traces, want 0", length)
	}
}

func TestDiskRejectsHostileID(t *testing.T) {
	storage, err := NewDiskStorage(10, t.TempDir())
	if err != nil {
		t.Fatalf("NewDisk: %v", err)
	}
	if _, err := storage.Load(context.Background(), "../../etc/passwd"); err == nil {
		t.Fatal("hostile trace ID was accepted")
	}
}

func TestDiskPrunesByAge(t *testing.T) {
	ctx := context.Background()
	storage, err := NewDiskStorage(10, t.TempDir())
	if err != nil {
		t.Fatalf("NewDisk: %v", err)
	}
	if err := storage.Save(ctx, storedTrace(t, "job", time.Now())); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := storage.Prune(ctx, 0); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// Prune ages the archive out; the ring keeps the recent window.
	names, err := storage.names()
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("prune left %d documents", len(names))
	}
	if length, _ := storage.Len(ctx); length != 1 {
		t.Fatalf("prune dropped the ring window: %d traces left", length)
	}
}

// ids returns the IDs of traces for failure messages.
func ids(traces []model.Trace) []string {
	out := make([]string, 0, len(traces))
	for _, trace := range traces {
		out = append(out, trace.ID)
	}
	return out
}
