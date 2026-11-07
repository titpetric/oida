package internal

import (
	"slices"
	"testing"

	"github.com/titpetric/oida/model"
)

func TestTraceContextEnviron(t *testing.T) {
	context, err := model.ParseTraceParent(exampleParent)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	want := []string{"TRACEPARENT=" + exampleParent}
	if got := TraceContextEnviron(context); !slices.Equal(got, want) {
		t.Fatalf("environ is %v, want %v", got, want)
	}

	context.State = "rojo=1"
	want = append(want, "TRACESTATE=rojo=1")
	if got := TraceContextEnviron(context); !slices.Equal(got, want) {
		t.Fatalf("environ is %v, want %v", got, want)
	}

	if got := TraceContextEnviron(model.TraceContext{}); got != nil {
		t.Fatalf("a context naming no trace returned %v", got)
	}
}
