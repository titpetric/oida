package oida

import (
	"context"
	"sync"
)

// StorageMemory retains completed traces in a bounded ring buffer. It is the
// default storage: nothing leaves the process and memory use is bounded by the
// configured size.
type StorageMemory struct {
	mu  sync.RWMutex
	log *ring
}

var _ Storage = (*StorageMemory)(nil)

// NewStorageMemory returns in-memory storage retaining size traces. A size of
// zero or less retains nothing, which is useful when only the live view and the
// lifetime counters are wanted.
func NewStorageMemory(size int) *StorageMemory {
	return &StorageMemory{log: newRing(size)}
}

// Save retains a completed trace, evicting the oldest one when full.
func (s *StorageMemory) Save(ctx context.Context, trace Trace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.push(&trace)
	return nil
}

// Load returns a retained trace.
func (s *StorageMemory) Load(ctx context.Context, id string) (Trace, error) {
	if err := ctx.Err(); err != nil {
		return Trace{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	trace, ok := s.log.find(id)
	if !ok {
		return Trace{}, ErrTraceNotFound
	}
	return *trace, nil
}

// List returns retained traces, newest first.
func (s *StorageMemory) List(ctx context.Context, limit int) ([]Trace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	retained := s.log.list()
	if limit > 0 && len(retained) > limit {
		retained = retained[:limit]
	}
	out := make([]Trace, 0, len(retained))
	for _, trace := range retained {
		out = append(out, *trace)
	}
	return out, nil
}

// Len returns the number of retained traces.
func (s *StorageMemory) Len(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.log.len(), nil
}

// Cap returns the retention limit.
func (s *StorageMemory) Cap() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.log.cap()
}

// Reset drops every retained trace.
func (s *StorageMemory) Reset(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.reset()
	return nil
}
