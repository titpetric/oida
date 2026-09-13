package storage

import (
	"context"
	"testing"
	"time"
)

// TestMemoryListDoesNotAliasSlots pins the isolation the in-place ring needs:
// a listed trace must survive its slot being rewritten by a later save.
func TestMemoryListDoesNotAliasSlots(t *testing.T) {
	ctx := context.Background()
	storage := NewMemoryStorage(1)

	first := storedTrace(t, "GET /first", time.Now())
	if err := storage.Save(ctx, &first); err != nil {
		t.Fatalf("Save: %v", err)
	}
	listed, err := storage.List(ctx, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("retained %d traces, want 1", len(listed))
	}
	kept := listed[0]

	second := storedTrace(t, "GET /second", time.Now())
	if err := storage.Save(ctx, &second); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if kept.ID != first.ID || kept.Name != "GET /first" {
		t.Errorf("listed trace changed after eviction: %q %q", kept.ID, kept.Name)
	}
	if len(kept.Spans) != 1 || kept.Spans[0].Name != "GET /first" {
		t.Errorf("listed spans changed after eviction: %+v", kept.Spans)
	}
	if reloaded, err := storage.Load(ctx, second.ID); err != nil || reloaded.Name != "GET /second" {
		t.Errorf("Load(second) = %q, %v", reloaded.Name, err)
	}
}
