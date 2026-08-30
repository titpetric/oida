package oida

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/oida/model"
)

// newTestServer returns a mux with the middleware and the front end mounted on
// a private tracer.
func newTestServer(t *testing.T, apply func(*Options)) (http.Handler, *Tracer) {
	t.Helper()

	tracer, _ := newTestTracer(t, apply)
	opts := tracer.Options()
	opts.Tracer = tracer

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, span := Start(r.Context(), "load user", KindDatabase)
		span.End()
		_, _ = w.Write([]byte("user"))
	})
	mux.HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) {
		panic("deliberate")
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return tracer.Middleware(mux), tracer
}

func TestMiddlewareRecordsRequest(t *testing.T) {
	handler, tracer := newTestServer(t, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if id := response.Header().Get(RequestIDHeader); !model.ValidID(id) {
		t.Fatalf("response carries no trace id: %q", id)
	}

	traces := tracer.Traces()
	if len(traces) != 1 {
		t.Fatalf("recorded %d traces, want 1", len(traces))
	}
	trace := traces[0]
	if trace.Name != "GET /users/{id}" {
		t.Fatalf("trace name is %q, want the routed pattern", trace.Name)
	}
	if trace.HTTP == nil || trace.HTTP.Status != http.StatusOK || trace.HTTP.ResponseBytes != 4 {
		t.Fatalf("unexpected http info: %+v", trace.HTTP)
	}
	if len(trace.Spans) != 2 || trace.Spans[0].Kind != KindHTTP || trace.Spans[1].Kind != KindDatabase {
		t.Fatalf("unexpected spans: %+v", trace.Spans)
	}
}

func TestMiddlewareIgnoresConfiguredPaths(t *testing.T) {
	handler, tracer := newTestServer(t, nil)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, DefaultPath, nil))

	if traces := tracer.Traces(); len(traces) != 0 {
		t.Fatalf("recorded %d ignored requests", len(traces))
	}
}

func TestMiddlewareRecordsPanic(t *testing.T) {
	handler, tracer := newTestServer(t, nil)

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("panic was swallowed by the middleware")
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic", nil))
	}()

	traces := tracer.Traces()
	if len(traces) != 1 || traces[0].State != StateError {
		t.Fatalf("unexpected traces: %+v", traces)
	}
	if !strings.Contains(traces[0].ErrorText, "deliberate") {
		t.Fatalf("panic not recorded: %q", traces[0].ErrorText)
	}
}

func TestMiddlewareSampling(t *testing.T) {
	handler, tracer := newTestServer(t, func(o *Options) { o.SampleRate = 25 })

	for range 8 {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/1", nil))
	}

	if traces := tracer.Traces(); len(traces) != 2 {
		t.Fatalf("recorded %d of 8 requests at a 25%% sample rate, want 2", len(traces))
	}
	snapshot := tracer.Snapshot()
	if snapshot.Total != 8 || snapshot.Sampled != 2 || snapshot.Dropped != 6 {
		t.Fatalf("unexpected counters: total %d sampled %d dropped %d", snapshot.Total, snapshot.Sampled, snapshot.Dropped)
	}
}

func TestMiddlewareZeroSampleRate(t *testing.T) {
	handler, tracer := newTestServer(t, func(o *Options) { o.SampleRate = 0 })

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/1", nil))

	if traces := tracer.Traces(); len(traces) != 0 {
		t.Fatalf("recorded %d requests at a zero sample rate", len(traces))
	}
	snapshot := tracer.Snapshot()
	if snapshot.Total != 1 || snapshot.Sampled != 0 || snapshot.Dropped != 1 {
		t.Fatalf("unexpected counters: total %d sampled %d dropped %d", snapshot.Total, snapshot.Sampled, snapshot.Dropped)
	}
}
