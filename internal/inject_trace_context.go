package internal

import (
	"net/http"

	"github.com/titpetric/oida/model"
)

// InjectTraceContext writes a trace context into a header set, as the
// traceparent the receiver joins and the tracestate it carries on. A context
// naming no trace writes nothing.
func InjectTraceContext(context model.TraceContext, header http.Header) {
	value := context.TraceParent()
	if value == "" || header == nil {
		return
	}
	header.Set(model.TraceParentHeader, value)
	if context.State != "" {
		header.Set(model.TraceStateHeader, context.State)
	}
}
