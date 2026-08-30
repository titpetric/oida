package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskStorageReadsFromMemory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewDiskStorage(10, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage: %v", err)
	}

	trace := storedTrace(t, "job", time.Now())
	if err := store.Save(ctx, trace); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Delete the document behind the driver's back: reads still answer,
	// because the ring is the read path.
	if err := os.Remove(filepath.Join(dir, trace.ID+traceFileSuffix)); err != nil {
		t.Fatalf("remove document: %v", err)
	}
	if _, err := store.Load(ctx, trace.ID); err != nil {
		t.Fatalf("Load after document removal: %v", err)
	}
	traces, err := store.List(ctx, 0)
	if err != nil || len(traces) != 1 {
		t.Fatalf("List after document removal = %v, %v", traces, err)
	}
}

func TestDiskStorageLoadFallsBackToDisk(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewDiskStorage(10, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage: %v", err)
	}

	// A document placed in the folder outside Save is not in the ring; Load
	// falls back to the file.
	trace := storedTrace(t, "archived", time.Now().Add(-time.Hour))
	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, trace.ID+traceFileSuffix), data, 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}

	loaded, err := store.Load(ctx, trace.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "archived" {
		t.Fatalf("loaded %q, want the archived trace", loaded.Name)
	}
}

func TestDiskStorageOpensEmpty(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	first, err := NewDiskStorage(10, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage: %v", err)
	}
	var ids []string
	for i, name := range []string{"one", "two", "three"} {
		trace := storedTrace(t, name, time.Now().Add(time.Duration(i)*time.Second))
		if err := first.Save(ctx, trace); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
		ids = append(ids, trace.ID)
	}

	// The ring holds what one storage recorded, so a second one on the same
	// folder lists its own traces, of which it has none.
	second, err := NewDiskStorage(10, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage reopen: %v", err)
	}
	traces, err := second.List(ctx, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(traces) != 0 {
		t.Fatalf("List after reopen returned %d traces, want none", len(traces))
	}
	if length, err := second.Len(ctx); err != nil || length != 0 {
		t.Fatalf("Len after reopen = %d, %v, want 0", length, err)
	}

	// The documents stay where they are, reachable by ID.
	loaded, err := second.Load(ctx, ids[0])
	if err != nil {
		t.Fatalf("Load after reopen: %v", err)
	}
	if loaded.Name != "one" {
		t.Fatalf("loaded %q, want one", loaded.Name)
	}
}

func TestDiskStorageRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	first, err := NewDiskStorage(10, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage: %v", err)
	}
	for i, name := range []string{"one", "two", "three"} {
		trace := storedTrace(t, name, time.Now().Add(time.Duration(i)*time.Second))
		if err := first.Save(ctx, trace); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	// Restore reads the folder into the ring, so a second storage lists what
	// the first recorded, newest first.
	second, err := NewDiskStorage(10, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage reopen: %v", err)
	}
	if err := second.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	traces, err := second.List(ctx, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(traces) != 3 {
		t.Fatalf("restored %d traces, want 3", len(traces))
	}
	if traces[0].Name != "three" {
		t.Errorf("newest first is %q, want three", traces[0].Name)
	}
}

func TestDiskStorageRestoreBounds(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	full, err := NewDiskStorage(10, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage: %v", err)
	}
	for i, name := range []string{"one", "two", "three"} {
		trace := storedTrace(t, name, time.Now().Add(time.Duration(i)*time.Second))
		if err := full.Save(ctx, trace); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	// A document nothing can decode is skipped rather than failing the read,
	// and the retention limit bounds how much of the folder is read at all.
	if err := os.WriteFile(filepath.Join(dir, "01JUNK"+traceFileSuffix), []byte("{"), 0o600); err != nil {
		t.Fatalf("write junk: %v", err)
	}
	small, err := NewDiskStorage(2, dir)
	if err != nil {
		t.Fatalf("NewDiskStorage: %v", err)
	}
	if err := small.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	length, err := small.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 2 {
		t.Fatalf("restored %d traces, want the 2 the limit allows", length)
	}
}
