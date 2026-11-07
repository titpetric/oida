package oida

import (
	"context"
	"net/http"

	"github.com/titpetric/oida/internal"
	"github.com/titpetric/oida/model"
)

// TraceContext is the W3C trace context of one transaction: the trace it
// belongs to, one span of that trace, and the vendor state travelling beside
// them. It lives in the model package so the front end can read it; the alias
// keeps it spelled oida.TraceContext.
type TraceContext = model.TraceContext

// The headers W3C Trace Context propagates a trace with.
const (
	TraceParentHeader = model.TraceParentHeader
	TraceStateHeader  = model.TraceStateHeader
)

// Propagate returns the trace context of the innermost span in ctx, which an
// outbound call names as its parent.
func Propagate(ctx context.Context) TraceContext {
	span := SpanFromContext(ctx)
	if span == nil {
		return TraceContext{}
	}
	return span.Trace().SpanContext(span.ID)
}

// TraceParent returns the traceparent header of the span in ctx.
func TraceParent(ctx context.Context) string {
	return Propagate(ctx).TraceParent()
}

// Inject writes the trace context of ctx into an outbound header set.
func Inject(ctx context.Context, header http.Header) {
	internal.InjectTraceContext(Propagate(ctx), header)
}

// Environ returns the TRACEPARENT and TRACESTATE of the trace in ctx.
func Environ(ctx context.Context) []string {
	return internal.TraceContextEnviron(Propagate(ctx))
}
