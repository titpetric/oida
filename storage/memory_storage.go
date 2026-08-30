package storage

import (
	"context"
	"sync"

	"github.com/titpetric/oida/model"
)

// memoryStorage retains completed traces in a bounded ring buffer. It is the
// built-in default, sized by Options.RingBufferSize: nothing leaves the
// process and memory use is bounded by the configured size.
type memoryStorage struct {
	*unimplementedStorage

	mu  sync.RWMutex
	log *ring
}

var _ model.Storage = (*memoryStorage)(nil)

// NewMemoryStorage returns in-memory storage retaining size traces. A size of
// zero or less retains nothing, which is useful when only the live view and the
// lifetime counters are wanted.
func NewMemoryStorage(size int) *memoryStorage {
	return &memoryStorage{log: newRing(size)}
}

// Save retains a completed trace, evicting the oldest one when full.
func (s *memoryStorage) Save(ctx context.Context, trace model.Trace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.push(&trace)
	return nil
}

// Load returns a retained trace.
func (s *memoryStorage) Load(ctx context.Context, id string) (model.Trace, error) {
	if err := ctx.Err(); err != nil {
		return model.Trace{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	trace, ok := s.log.find(id)
	if !ok {
		return model.Trace{}, model.ErrTraceNotFound
	}
	return *trace, nil
}

// List returns retained traces, newest first.
func (s *memoryStorage) List(ctx context.Context, limit int) ([]model.Trace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	retained := s.log.list()
	if limit > 0 && len(retained) > limit {
		retained = retained[:limit]
	}
	out := make([]model.Trace, 0, len(retained))
	for _, trace := range retained {
		out = append(out, *trace)
	}
	return out, nil
}

// Len returns the number of retained traces.
func (s *memoryStorage) Len(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.log.len(), nil
}

// Cap returns the retention limit.
func (s *memoryStorage) Cap() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.log.cap()
}

// Reset drops every retained trace.
func (s *memoryStorage) Reset(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log.reset()
	return nil
}
