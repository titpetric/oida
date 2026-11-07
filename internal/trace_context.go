package internal

import (
	"net/http"

	"github.com/titpetric/oida/model"
)

// TraceContextFor returns the trace a request joins and the ID to record it
// under: the caller's trace where the options trust the traceparent header,
// and a trace of its own otherwise.
//
// A header that does not parse restarts the trace, which is what the
// specification asks of a receiver, and the tracestate goes with it.
func TraceContextFor(r *http.Request, opts model.Options) (model.TraceContext, string) {
	if !opts.TrustTraceContext {
		return model.TraceContext{}, RequestID(r, opts)
	}
	parent, err := model.ParseTraceParent(r.Header.Get(model.TraceParentHeader))
	if err != nil {
		return model.TraceContext{}, RequestID(r, opts)
	}
	parent.State = model.CleanTraceState(r.Header.Get(model.TraceStateHeader))
	return parent, RequestID(r, opts)
}
