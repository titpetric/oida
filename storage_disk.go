package oida

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

// traceFileSuffix is the extension of a stored trace document.
const traceFileSuffix = ".json"

// StorageDisk retains completed traces as JSON documents in a folder, so traces
// survive a process restart. Trace IDs are lexicographically sortable, so the
// folder listing is chronological and pruning drops the oldest documents.
type StorageDisk struct {
	mu    sync.Mutex
	path  string
	limit int
}

var _ Storage = (*StorageDisk)(nil)

// NewStorageDisk creates the storage folder, verifies that it is writable, and
// retains at most limit traces. A limit of zero or less is unbounded. With no
// path it uses a folder in the operating system temporary directory.
func NewStorageDisk(limit int, paths ...string) (*StorageDisk, error) {
	path := filepath.Join(os.TempDir(), "oida-traces")
	if len(paths) > 0 {
		path = paths[0]
	}
	if path == "" {
		return nil, invalidOption("storage path", "is empty")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, fmt.Errorf("oida: create trace storage: %w", err)
	}

	probe, err := os.CreateTemp(path, ".writable-")
	if err != nil {
		return nil, fmt.Errorf("oida: open trace storage: %w", err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return nil, fmt.Errorf("oida: close trace storage probe: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return nil, fmt.Errorf("oida: remove trace storage probe: %w", err)
	}

	return &StorageDisk{path: path, limit: limit}, nil
}

// Path returns the folder traces are written to.
func (s *StorageDisk) Path() string {
	return s.path
}

// Save writes a trace document atomically and prunes the oldest documents over
// the retention limit.
func (s *StorageDisk) Save(ctx context.Context, trace Trace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name, err := s.traceFile(trace.ID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("oida: encode trace: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tmp, err := os.CreateTemp(s.path, ".trace-")
	if err != nil {
		return fmt.Errorf("oida: write trace: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, name); err != nil {
		return fmt.Errorf("oida: store trace: %w", err)
	}
	return s.prune()
}

// Load reads a stored trace document.
func (s *StorageDisk) Load(ctx context.Context, id string) (Trace, error) {
	if err := ctx.Err(); err != nil {
		return Trace{}, err
	}
	name, err := s.traceFile(id)
	if err != nil {
		return Trace{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return Trace{}, ErrTraceNotFound
	}
	if err != nil {
		return Trace{}, fmt.Errorf("oida: read trace: %w", err)
	}

	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return Trace{}, fmt.Errorf("oida: decode trace: %w", err)
	}
	return trace, nil
}

// List reads stored traces, newest first.
func (s *StorageDisk) List(ctx context.Context, limit int) ([]Trace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	names, err := s.names()
	if err != nil {
		return nil, err
	}
	slices.Reverse(names)
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}

	traces := make([]Trace, 0, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(s.path, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("oida: read trace: %w", err)
		}
		var trace Trace
		if err := json.Unmarshal(data, &trace); err != nil {
			continue
		}
		traces = append(traces, trace)
	}
	return traces, nil
}

// Len returns the number of stored trace documents.
func (s *StorageDisk) Len(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	names, err := s.names()
	if err != nil {
		return 0, err
	}
	return len(names), nil
}

// Cap returns the retention limit.
func (s *StorageDisk) Cap() int {
	return s.limit
}

// Reset removes every stored trace document.
func (s *StorageDisk) Reset(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	names, err := s.names()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := os.Remove(filepath.Join(s.path, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Prune removes trace documents older than maxAge.
func (s *StorageDisk) Prune(ctx context.Context, maxAge time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.path)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != traceFileSuffix {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(s.path, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// names returns the sorted trace document names. Callers hold the lock.
func (s *StorageDisk) names() ([]string, error) {
	entries, err := os.ReadDir(s.path)
	if err != nil {
		return nil, fmt.Errorf("oida: list trace storage: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != traceFileSuffix {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// prune drops the oldest documents over the retention limit. Callers hold the
// lock.
func (s *StorageDisk) prune() error {
	if s.limit <= 0 {
		return nil
	}
	names, err := s.names()
	if err != nil {
		return err
	}
	for i := 0; i < len(names)-s.limit; i++ {
		if err := os.Remove(filepath.Join(s.path, names[i])); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// traceFile returns the document path of a trace ID, rejecting IDs that are not
// plain identifiers so a hostile ID cannot escape the folder.
func (s *StorageDisk) traceFile(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("oida: invalid trace id %q", id)
	}
	return filepath.Join(s.path, id+traceFileSuffix), nil
}
