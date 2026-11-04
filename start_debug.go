package oida

import (
	"context"

	"github.com/titpetric/oida/model"
)

// StartDebug is Start for a debug span: one the trace keeps only when it ran
// at least Options.DebugSpanMinDuration, and counts in Trace.DroppedSpans when
// it did not.
//
//	ctx, span := oida.StartDebug(ctx, "Row::text")
//	defer span.End()
//
// Use it where the call is instrumented because it might be slow and not
// because it is interesting: a frame of an interpreter, a column read, a
// cache lookup. The filter runs as the span ends, so the span nests and
// carries attributes like any other while the region runs.
func StartDebug(ctx context.Context, name string, kind ...Kind) (context.Context, *Span) {
	return model.StartDebug(ctx, name, kind...)
}

// StartDebugSpan is StartDebug without deriving a context, for a debug span
// that will not nest.
func StartDebugSpan(ctx context.Context, name string, kind ...Kind) *Span {
	return model.StartDebugSpan(ctx, name, kind...)
}
