package oida

import (
	"io"
	"net"
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

func TestMiddlewareEmitsTraceContext(t *testing.T) {
	handler, tracer := newTestServer(t, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	context, err := model.ParseTraceParent(response.Header().Get(model.TraceParentHeader))
	if err != nil {
		t.Fatalf("the response carries no traceparent: %v", err)
	}
	trace := tracer.Traces()[0]
	if context.TraceID != model.TraceIDHex(trace.ID) || context.TraceID != trace.TraceID {
		t.Fatalf("the header names %q, want the trace's own %q", context.TraceID, trace.TraceID)
	}
	if context.SpanID != trace.SpanID || !context.Sampled() {
		t.Fatalf("the header names span %q, want the root span sampled", context.SpanID)
	}
	if trace.ParentSpanID != "" {
		t.Fatalf("a request nobody called recorded parent %q", trace.ParentSpanID)
	}
}

func TestMiddlewareJoinsTheCallersTrace(t *testing.T) {
	handler, tracer := newTestServer(t, func(o *Options) { o.TrustTraceContext = true })

	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	request.Header.Set(model.TraceParentHeader, exampleParent)
	request.Header.Set(model.TraceStateHeader, "rojo=1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	trace := tracer.Traces()[0]
	if trace.TraceID != exampleTraceID || trace.ParentSpanID != exampleSpanID {
		t.Fatalf("the request recorded under %q below %q", trace.TraceID, trace.ParentSpanID)
	}
	if trace.TraceState != "rojo=1" {
		t.Fatalf("the tracestate came through as %q", trace.TraceState)
	}
	// The transaction keeps an identifier of its own: a W3C trace spans many
	// of them, and the dashboard still lists this one on its own row.
	if !model.ValidID(trace.ID) || trace.ID == exampleTraceID {
		t.Fatalf("the transaction was recorded under %q", trace.ID)
	}
	context, err := model.ParseTraceParent(response.Header().Get(model.TraceParentHeader))
	if err != nil || context.TraceID != exampleTraceID {
		t.Fatalf("the response names %+v, %v", context, err)
	}
	if got := response.Header().Get(model.TraceStateHeader); got != "rojo=1" {
		t.Fatalf("the response carries tracestate %q", got)
	}
}

func TestMiddlewareIgnoresAnUntrustedTraceContext(t *testing.T) {
	handler, tracer := newTestServer(t, nil)

	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	request.Header.Set(model.TraceParentHeader, exampleParent)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	if trace := tracer.Traces()[0]; trace.TraceID == exampleTraceID || trace.ParentSpanID != "" {
		t.Fatalf("an untrusted header was joined: %q below %q", trace.TraceID, trace.ParentSpanID)
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

// A handler that upgrades the connection is served through the middleware,
// over a real server: the writer it is handed has to be an http.Hijacker
// itself, because golang.org/x/net/websocket asserts the interface on it and
// reaches no wrapper through Unwrap. A terminal attach panicked in here before
// the wrapper implemented it.
func TestMiddlewareServesHijack(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /attach", func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the writer the middleware passes on is no http.Hijacker")
			return
		}
		conn, buffered, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: terminal\r\n\r\nattached")
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
	})

	server := httptest.NewServer(tracer.Middleware(mux))
	defer server.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("GET /attach HTTP/1.1\r\nHost: test\r\n\r\n")); err != nil {
		t.Fatalf("request: %v", err)
	}
	answer, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(answer), "101 Switching Protocols") || !strings.HasSuffix(string(answer), "attached") {
		t.Fatalf("the upgrade did not come back: %q", answer)
	}

	// The handler closes the hijacked connection before the middleware
	// completes the trace, so the answer arriving says nothing about the
	// recording having happened.
	traces := recorded(t, tracer, 1)
	if len(traces) != 1 {
		t.Fatalf("recorded %d traces, want 1", len(traces))
	}
	if traces[0].HTTP == nil || traces[0].HTTP.Status != http.StatusSwitchingProtocols {
		t.Fatalf("unexpected http info: %+v", traces[0].HTTP)
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
