package oida

import "context"

// RecordError records err on the innermost span in ctx, which marks that span
// and its trace as failed. It is Span.RecordError for code that holds a context
// rather than the span, such as an error path several calls below the one that
// started it.
//
//	if err := store.Save(ctx, u); err != nil {
//		oida.RecordError(ctx, err)
//		return err
//	}
//
// A nil error, a context without a trace and an unsampled request are all
// no-ops, so a caller never has to check first.
func RecordError(ctx context.Context, err error) {
	SpanFromContext(ctx).RecordError(err)
}
