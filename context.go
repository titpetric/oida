package oida

import "context"

// traceKey carries the active trace in a context.
type traceKey struct{}

// spanKey carries the active span in a context.
type spanKey struct{}

// WithTrace returns a context carrying the trace. Spans started from the
// returned context, or any context derived from it, are recorded on it.
func WithTrace(ctx context.Context, t *Trace) context.Context {
	return withTrace(ctx, t)
}

// withTrace stores the trace in ctx, leaving ctx alone for a nil trace.
func withTrace(ctx context.Context, t *Trace) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if t == nil {
		return ctx
	}
	if current, _ := ctx.Value(traceKey{}).(*Trace); current == t {
		return ctx
	}
	return context.WithValue(ctx, traceKey{}, t)
}

// TraceFromContext returns the trace in ctx, or nil.
func TraceFromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(traceKey{}).(*Trace)
	return trace
}

// SpanFromContext returns the innermost span in ctx, or nil.
func SpanFromContext(ctx context.Context) *Span {
	return spanFromContext(ctx)
}

func spanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	span, _ := ctx.Value(spanKey{}).(*Span)
	return span
}

// TraceID returns the identifier of the trace in ctx, or an empty string. It is
// the value of the Request-Id header for HTTP traces, which makes it the
// cheapest correlation key for logs.
func TraceID(ctx context.Context) string {
	trace := TraceFromContext(ctx)
	if trace == nil {
		return ""
	}
	return trace.ID
}

// Start records a span in the trace carried by ctx and returns a context
// carrying it. When ctx has no trace, or the trace was not sampled, it returns
// ctx unchanged and a nil span: every span method tolerates that.
//
//	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
//	defer span.End()
func Start(ctx context.Context, name string, kind ...Kind) (context.Context, *Span) {
	return TraceFromContext(ctx).StartSpan(ctx, name, kind...)
}

// StartSpan records a span without deriving a context. Use it for leaf spans
// that will not nest.
func StartSpan(ctx context.Context, name string, kind ...Kind) *Span {
	_, span := Start(ctx, name, kind...)
	return span
}

// Do runs fn inside a span, records the returned error on it and ends it. The
// error is returned unchanged.
func Do(ctx context.Context, name string, fn func(context.Context) error, kind ...Kind) error {
	ctx, span := Start(ctx, name, kind...)
	err := fn(ctx)
	span.EndWithError(err)
	return err
}
