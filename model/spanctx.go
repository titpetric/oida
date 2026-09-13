package model

import (
	"context"
)

// spanCtx carries a span the way context.WithValue would, preallocated in
// the span's box so deriving a context from a span allocates nothing. The
// parent is embedded, so deadlines, cancellation and every other value pass
// through.
type spanCtx struct {
	context.Context
	span *Span
}

// Value returns the span for the span key and defers to the parent for
// everything else.
func (c *spanCtx) Value(key any) any {
	if _, ok := key.(spanKey); ok {
		return c.span
	}
	return c.Context.Value(key)
}
