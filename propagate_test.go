package oida

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/titpetric/oida/model"
)

// The example of W3C Trace Context section 3.2.
const (
	exampleTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	exampleSpanID  = "00f067aa0ba902b7"
	exampleParent  = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
)

// tracedContext returns a context inside one span of a trace the tracer
// opened, which is what instrumented code holds.
func tracedContext(t *testing.T) context.Context {
	t.Helper()

	tracer, _ := newTestTracer(t, func(o *Options) { o.TraceParent = exampleParent })
	ctx, trace, err := tracer.StartTrace(context.Background(), "atkins build")
	if err != nil {
		t.Fatalf("start trace: %v", err)
	}
	t.Cleanup(func() { tracer.Finish(trace) })
	return ctx
}

func TestTraceContext(t *testing.T) {
	var context TraceContext = model.TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, Flags: model.FlagSampled}
	if got := context.TraceParent(); got != exampleParent {
		t.Fatalf("the alias wrote %q, want %q", got, exampleParent)
	}
}

func TestPropagate(t *testing.T) {
	if got := Propagate(context.Background()); got.Valid() {
		t.Fatalf("a context carrying no trace propagates %+v", got)
	}

	// The trace joined the process's own trace, so what it propagates names
	// that trace and a span of its own.
	propagated := Propagate(tracedContext(t))
	if propagated.TraceID != exampleTraceID {
		t.Fatalf("the trace propagates %q, want the process's %q", propagated.TraceID, exampleTraceID)
	}
	if propagated.SpanID == exampleSpanID || !propagated.Valid() {
		t.Fatalf("the span propagated is %q, want one of its own", propagated.SpanID)
	}
}

func TestTraceParent(t *testing.T) {
	if got := TraceParent(context.Background()); got != "" {
		t.Fatalf("a context carrying no trace wrote %q", got)
	}
	header := TraceParent(tracedContext(t))
	if !strings.HasPrefix(header, "00-"+exampleTraceID+"-") {
		t.Fatalf("the header reads %q, want the process's trace", header)
	}
	if _, err := model.ParseTraceParent(header); err != nil {
		t.Fatalf("the header this package wrote does not parse: %v", err)
	}
}

func TestInject(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	Inject(tracedContext(t), request.Header)

	if _, err := model.ParseTraceParent(request.Header.Get(TraceParentHeader)); err != nil {
		t.Fatalf("the injected header does not parse: %v", err)
	}

	empty := http.Header{}
	Inject(context.Background(), empty)
	if len(empty) != 0 {
		t.Fatalf("a context carrying no trace injected %v", empty)
	}
}

func TestEnviron(t *testing.T) {
	if got := Environ(context.Background()); got != nil {
		t.Fatalf("a context carrying no trace returned %v", got)
	}

	env := Environ(tracedContext(t))
	if len(env) != 1 || !strings.HasPrefix(env[0], "TRACEPARENT=00-"+exampleTraceID+"-") {
		t.Fatalf("the environment is %v, want one TRACEPARENT of the process's trace", env)
	}
	if slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "TRACESTATE=") }) {
		t.Fatalf("a process with no vendor state exported one: %v", env)
	}
}
