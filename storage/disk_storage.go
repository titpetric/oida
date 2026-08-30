package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/titpetric/oida/model"
)

// traceFileSuffix is the extension of a stored trace document.
const traceFileSuffix = ".json"

// diskStorage is a write-through overlay of memoryStorage: every save goes
// to a JSON document in the folder and to the in-memory ring, and every read
// resolves from the ring, so listing costs no disk access. Load falls back
// to the folder for a trace the ring no longer holds.
//
// The ring holds what this process recorded, so the dashboard lists its own
// traces. The folder outlives the process: its documents are reachable by ID
// through Load and aged out by Prune. Trace IDs are lexicographically
// sortable, so the folder listing is chronological and pruning drops the
// oldest documents.
type diskStorage struct {
	*unimplementedStorage

	// memory is the read path: the same bounded ring the memory driver
	// uses. Its lock is the only one: documents use unique names written
	// atomically, so the folder needs no writer coordination, and every
	// folder read is a read.
	memory *memoryStorage

	path  string
	limit int
}

var _ model.Storage = (*diskStorage)(nil)

// NewDiskStorage creates the storage folder, verifies that it is writable and
// retains at most limit traces. A limit of zero or less is unbounded. With no
// path it uses a folder in the operating system temporary directory. The ring
// holds what the returned storage records.
func NewDiskStorage(limit int, paths ...string) (*diskStorage, error) {
	path := filepath.Join(os.TempDir(), "oida-traces")
	if len(paths) > 0 {
		path = paths[0]
	}
	if path == "" {
		return nil, fmt.Errorf("%w: storage path is empty", model.ErrInvalidOptions)
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

	return &diskStorage{memory: NewMemoryStorage(limit), path: path, limit: limit}, nil
}

// Restore fills the ring from the folder, so this process lists what earlier
// ones recorded. It reads the newest documents up to the retention limit,
// oldest first, so the ring lists them newest first.
//
// A document that cannot be read or decoded is skipped: the folder came from
// another process, and one unreadable file does not fail the read. The cost is
// one directory listing plus a read per document, so the caller asks for it
// rather than opening a folder doing it.
func (s *diskStorage) Restore(ctx context.Context) error {
	names, err := s.names()
	if err != nil {
		return err
	}
	if s.limit > 0 && len(names) > s.limit {
		names = names[len(names)-s.limit:]
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(s.path, name))
		if err != nil {
			continue
		}
		var trace model.Trace
		if err := json.Unmarshal(data, &trace); err != nil {
			continue
		}
		if err := s.memory.Save(ctx, trace); err != nil {
			return err
		}
	}
	return nil
}

// Path returns the folder traces are written to.
func (s *diskStorage) Path() string {
	return s.path
}

// Save writes a trace document atomically, prunes the oldest documents over
// the retention limit, and retains the trace in the ring. A disk failure
// returns before the ring is touched, so what the ring serves is persisted.
func (s *diskStorage) Save(ctx context.Context, trace model.Trace) error {
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
	if err := s.writeDocument(name, data); err != nil {
		return err
	}
	return s.memory.Save(ctx, trace)
}

// writeDocument writes one trace document atomically and prunes the oldest
// documents over the retention limit. No lock: the name is unique to the
// trace and the rename is atomic; concurrent prunes tolerate a document the
// other already removed.
func (s *diskStorage) writeDocument(name string, data []byte) error {
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

// Load returns a trace from the ring, falling back to the stored document
// for a trace the ring no longer holds.
func (s *diskStorage) Load(ctx context.Context, id string) (model.Trace, error) {
	if trace, err := s.memory.Load(ctx, id); err == nil {
		return trace, nil
	} else if !errors.Is(err, model.ErrTraceNotFound) {
		return model.Trace{}, err
	}
	name, err := s.traceFile(id)
	if err != nil {
		return model.Trace{}, err
	}

	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return model.Trace{}, model.ErrTraceNotFound
	}
	if err != nil {
		return model.Trace{}, fmt.Errorf("oida: read trace: %w", err)
	}

	var trace model.Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return model.Trace{}, fmt.Errorf("oida: decode trace: %w", err)
	}
	return trace, nil
}

// List returns retained traces from the ring, newest first. The folder is
// not read: the ring holds the same window the documents do.
func (s *diskStorage) List(ctx context.Context, limit int) ([]model.Trace, error) {
	return s.memory.List(ctx, limit)
}

// Len returns the number of retained traces in the ring.
func (s *diskStorage) Len(ctx context.Context) (int, error) {
	return s.memory.Len(ctx)
}

// Cap returns the retention limit.
func (s *diskStorage) Cap() int {
	return s.limit
}

// Reset drops the ring and removes every stored trace document.
func (s *diskStorage) Reset(ctx context.Context) error {
	if err := s.memory.Reset(ctx); err != nil {
		return err
	}
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

// Prune removes trace documents older than maxAge. The ring is untouched:
// it is the recent window and has no age semantics; the archive is what
// ages out.
func (s *diskStorage) Prune(ctx context.Context, maxAge time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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

// names returns the sorted trace document names.
func (s *diskStorage) names() ([]string, error) {
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
func (s *diskStorage) prune() error {
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
func (s *diskStorage) traceFile(id string) (string, error) {
	if !model.ValidID(id) {
		return "", fmt.Errorf("oida: invalid trace id %q", id)
	}
	return filepath.Join(s.path, id+traceFileSuffix), nil
}
