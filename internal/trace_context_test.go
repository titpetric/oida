package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/oida/model"
)

func TestTraceContextFor(t *testing.T) {
	request := func(traceparent, tracestate string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/users/42", nil)
		if traceparent != "" {
			r.Header.Set(model.TraceParentHeader, traceparent)
		}
		if tracestate != "" {
			r.Header.Set(model.TraceStateHeader, tracestate)
		}
		return r
	}

	trusting := model.NewOptions("test")
	trusting.TrustTraceContext = true

	parent, id := TraceContextFor(request(exampleParent, " rojo=1 ,bad"), trusting)
	if !parent.Valid() || parent.State != "rojo=1" {
		t.Fatalf("the request joined %+v", parent)
	}
	if !model.ValidID(id) {
		t.Fatalf("the transaction was recorded under %q", id)
	}

	// Untrusted options ignore the header, header or no header.
	if parent, _ := TraceContextFor(request(exampleParent, ""), model.NewOptions("test")); parent.Valid() {
		t.Fatalf("an untrusting tracer joined %+v", parent)
	}

	// A header that does not parse restarts the trace, and takes the
	// tracestate with it.
	parent, id = TraceContextFor(request("nonsense", "rojo=1"), trusting)
	if parent.Valid() || parent.State != "" {
		t.Fatalf("a broken header was joined as %+v", parent)
	}
	if !model.ValidID(id) {
		t.Fatalf("the transaction was recorded under %q", id)
	}
}
