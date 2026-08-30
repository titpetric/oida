package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/titpetric/oida/model"
)

func TestUnimplementedStorage(t *testing.T) {
	ctx := context.Background()

	// The zero value answers every method: the embed is a nil pointer on a
	// zero driver, and no method touches state.
	var store *unimplementedStorage

	if err := store.Save(ctx, model.Trace{ID: "t1"}); err != nil {
		t.Errorf("Save: %v", err)
	}
	if _, err := store.Load(ctx, "t1"); !errors.Is(err, model.ErrTraceNotFound) {
		t.Errorf("Load returned %v, want ErrTraceNotFound", err)
	}
	if traces, err := store.List(ctx, 0); err != nil || len(traces) != 0 {
		t.Errorf("List = %v, %v, want empty", traces, err)
	}
	if length, err := store.Len(ctx); err != nil || length != 0 {
		t.Errorf("Len = %d, %v, want 0", length, err)
	}
	if store.Cap() != 0 {
		t.Errorf("Cap = %d, want unbounded 0", store.Cap())
	}
	if err := store.Reset(ctx); err != nil {
		t.Errorf("Reset: %v", err)
	}
	if err := store.Prune(ctx, time.Hour); err != nil {
		t.Errorf("Prune: %v", err)
	}
}

func TestUnimplementedStoragePruneOnMemory(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStorage(4)

	if err := store.Save(ctx, model.Trace{ID: "t1"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Memory has nothing to prune: the call lands on the embed, succeeds,
	// and drops nothing.
	if err := store.Prune(ctx, time.Nanosecond); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if length, _ := store.Len(ctx); length != 1 {
		t.Fatalf("Prune dropped traces from the memory driver: %d left", length)
	}
}
