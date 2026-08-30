package storage

import (
	"context"
	"time"

	"github.com/titpetric/oida/model"
)

// unimplementedStorage answers the whole Storage interface as no-ops: saves
// and resets succeed without effect, reads return nothing. Both drivers embed
// it, so a method added to the interface lands here first and neither breaks.
type unimplementedStorage struct{}

var _ model.Storage = (*unimplementedStorage)(nil)

// Save retains nothing.
func (*unimplementedStorage) Save(ctx context.Context, trace model.Trace) error {
	return nil
}

// Load holds nothing, so every lookup misses.
func (*unimplementedStorage) Load(ctx context.Context, id string) (model.Trace, error) {
	return model.Trace{}, model.ErrTraceNotFound
}

// List returns no traces.
func (*unimplementedStorage) List(ctx context.Context, limit int) ([]model.Trace, error) {
	return nil, nil
}

// Len reports an empty store.
func (*unimplementedStorage) Len(ctx context.Context) (int, error) {
	return 0, nil
}

// Cap reports an unbounded store.
func (*unimplementedStorage) Cap() int {
	return 0
}

// Reset has nothing to drop.
func (*unimplementedStorage) Reset(ctx context.Context) error {
	return nil
}

// Prune has nothing to prune.
func (*unimplementedStorage) Prune(ctx context.Context, maxAge time.Duration) error {
	return nil
}

// Restore has nothing persisted to read back.
func (*unimplementedStorage) Restore(ctx context.Context) error {
	return nil
}
