package internal

import (
	"testing"

	"github.com/titpetric/oida/model"
)

// The example of W3C Trace Context section 3.2.
const exampleParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestTraceContextFromEnv(t *testing.T) {
	t.Setenv(envTraceParent, exampleParent)
	t.Setenv(envTraceState, "rojo=1")

	opts := envOptions(t)
	if opts.TraceParent != exampleParent || opts.TraceState != "rojo=1" {
		t.Fatalf("the process read %q with state %q", opts.TraceParent, opts.TraceState)
	}
	parent, err := opts.Parent()
	if err != nil || !parent.Valid() {
		t.Fatalf("the context the process joined is %+v, %v", parent, err)
	}
}

func TestTraceContextFromEnvNamespaced(t *testing.T) {
	t.Setenv(envTraceParent, "nonsense")
	t.Setenv("OIDA_"+envTraceParent, exampleParent)

	if opts := envOptions(t); opts.TraceParent != exampleParent {
		t.Fatalf("the namespaced variable did not win: %q", opts.TraceParent)
	}
}

func TestTraceContextFromEnvKeepsTheCode(t *testing.T) {
	t.Setenv(envTraceParent, exampleParent)

	opts := model.NewOptions("env-test")
	opts.TraceParent = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	wanted := opts.TraceParent
	if got, err := envReadInto(&opts); err != nil || got.TraceParent != wanted {
		t.Fatalf("the environment overwrote the traceparent set in code: %q, %v", got.TraceParent, err)
	}
}

func TestFirstEnv(t *testing.T) {
	t.Setenv("OIDA_TRACE_FIRST", "")
	t.Setenv("OIDA_TRACE_SECOND", "second")

	if got := firstEnv("OIDA_TRACE_FIRST", "OIDA_TRACE_SECOND"); got != "second" {
		t.Fatalf("firstEnv = %q, want the one that is set", got)
	}
	if got := firstEnv("OIDA_TRACE_FIRST"); got != "" {
		t.Fatalf("firstEnv = %q, want nothing", got)
	}
}
