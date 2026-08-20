package oida

import "context"

// RecordError records err on the innermost span in ctx and on its trace.
//
//	if err := store.Save(ctx, u); err != nil {
//		oida.RecordError(ctx, err)
//		return err
//	}
//
// It is Span.RecordError for code that holds a context rather than the span. A
// nil error, a context without a trace and an unsampled request are no-ops.
func RecordError(ctx context.Context, err error) {
	SpanFromContext(ctx).RecordError(err)
}
