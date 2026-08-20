package oida

import (
	"context"

	"github.com/titpetric/oida/model"
)

// WithTrace returns a context carrying the trace. Spans started from the
// returned context, or any context derived from it, are recorded on it.
func WithTrace(ctx context.Context, t *Trace) context.Context {
	return model.WithTrace(ctx, t)
}

// TraceFromContext returns the trace in ctx, or nil.
func TraceFromContext(ctx context.Context) *Trace {
	return model.TraceFromContext(ctx)
}

// SpanFromContext returns the innermost span in ctx, or nil.
func SpanFromContext(ctx context.Context) *Span {
	return model.SpanFromContext(ctx)
}

// TraceID returns the identifier of the trace in ctx, or an empty string. It is
// the value of the Request-Id header for HTTP traces, which makes it the
// cheapest correlation key for logs.
func TraceID(ctx context.Context) string {
	return model.TraceID(ctx)
}

// Start records a span in the trace carried by ctx and returns a context
// carrying it. When ctx has no trace, or the trace was not sampled, it returns
// ctx unchanged and a nil span: every span method tolerates that.
//
//	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
//	defer span.End()
//
// The kind is optional and defaults to KindInternal.
func Start(ctx context.Context, name string, kind ...Kind) (context.Context, *Span) {
	return model.Start(ctx, name, kind...)
}

// StartSpan records a span without deriving a context. Use it for leaf spans
// that will not nest.
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span {
	return model.StartSpan(ctx, name, kind...)
}

// Do runs fn inside a span, records the returned error on it and ends it. The
// error is returned unchanged.
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error {
	return model.Do(ctx, name, fn, kind...)
}
