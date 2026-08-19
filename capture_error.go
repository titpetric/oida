package oida

import "context"

// CaptureError records err on the innermost span in ctx, which marks that span
// and its trace as failed. A nil error, a context without a trace and an
// unsampled request are all no-ops, so a caller never has to check first.
//
//	if err := store.Save(ctx, u); err != nil {
//		oida.CaptureError(ctx, err)
//		return err
//	}
func CaptureError(ctx context.Context, err error) {
	SpanFromContext(ctx).RecordError(err)
}
