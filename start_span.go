package oida

import (
	"context"

	"github.com/titpetric/oida/model"
)

// StartSpan records a span without deriving a context. Use it for leaf spans
// that will not nest.
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span {
	return model.StartSpan(ctx, name, kind...)
}
