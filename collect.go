package oida

import (
	"context"

	"github.com/titpetric/oida/model"
)

// Collect records a collection and answers one Collected per transaction.
func (t *Tracer) Collect(ctx context.Context, collection Collection) (Receipt, error) {
	return model.Collect(ctx, t, collection)
}
