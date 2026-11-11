package internal

import (
	"net/http"
	"testing"

	"github.com/titpetric/oida/model"
)

func TestInjectTraceContext(t *testing.T) {
	context, err := model.ParseTraceParent(exampleParent)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	context.State = "rojo=1"

	header := http.Header{}
	InjectTraceContext(context, header)
	if got := header.Get(model.TraceParentHeader); got != exampleParent {
		t.Fatalf("traceparent is %q, want %q", got, exampleParent)
	}
	if got := header.Get(model.TraceStateHeader); got != "rojo=1" {
		t.Fatalf("tracestate is %q, want the one that arrived", got)
	}

	empty := http.Header{}
	InjectTraceContext(model.TraceContext{}, empty)
	if len(empty) != 0 {
		t.Fatalf("a context naming no trace wrote %v", empty)
	}

	// A nil header is what a caller holding no response has.
	InjectTraceContext(context, nil)
}
