package oida

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/oida/model"
)

const (
	testSession      = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	testSessionTrace = "01563e3ab5d3d6764c61efb99302bd5b"
)

// TestSessionGroupsTheBrowserAndTheBackend is the acceptance flow: a pageload,
// a back-end call and a fragment change all land under one session, each its
// own transaction under the session's trace.
func TestSessionGroupsTheBrowserAndTheBackend(t *testing.T) {
	handler, tracer := newTestServer(t, func(o *Options) { o.TrustTraceContext = true })

	if model.SessionTraceID(testSession) != testSessionTrace {
		t.Fatalf("the session's trace is %q", model.SessionTraceID(testSession))
	}

	// The browser posts its pageload and a fragment change.
	pageload := Transaction{Name: "pageload /inventory", SessionID: testSession, TraceID: testSessionTrace, SpanID: "aaaaaaaaaaaaaaa1"}
	route := Transaction{Name: "route /inventory#home", SessionID: testSession, TraceID: testSessionTrace, SpanID: "aaaaaaaaaaaaaaa2"}
	receipt, err := tracer.Collect(context.Background(), Collection{Transactions: []Transaction{pageload, route}})
	if err != nil || receipt.Transactions[0].Error != "" || receipt.Transactions[1].Error != "" {
		t.Fatalf("collect: %v %+v", err, receipt)
	}

	// The page then calls the back end with the traceparent and the tracestate.
	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	request.Header.Set(model.TraceParentHeader, "00-"+testSessionTrace+"-aaaaaaaaaaaaaaa3-01")
	request.Header.Set(model.TraceStateHeader, "oida="+testSession)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	traces := tracer.Traces()
	if len(traces) != 3 {
		t.Fatalf("recorded %d transactions, want 3", len(traces))
	}
	for _, trace := range traces {
		if trace.SessionID != testSession || trace.TraceID != testSessionTrace {
			t.Errorf("%s: session %q, trace %q", trace.Name, trace.SessionID, trace.TraceID)
		}
	}
}

func TestSessionIsNotReadWithoutTrust(t *testing.T) {
	handler, tracer := newTestServer(t, nil)

	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	request.Header.Set(model.TraceParentHeader, "00-"+testSessionTrace+"-aaaaaaaaaaaaaaa3-01")
	request.Header.Set(model.TraceStateHeader, "oida="+testSession)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if got := tracer.Traces()[0].SessionID; got != "" {
		t.Fatalf("an untrusted session was recorded: %q", got)
	}
}
