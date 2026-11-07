package model

import (
	"testing"
)

func TestCollected(t *testing.T) {
	collected := NewCollected("01K0000000000000000000000A", TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, Flags: FlagSampled})
	if collected.ID != "01K0000000000000000000000A" || collected.TraceParent != exampleParent {
		t.Fatalf("the answer reads %+v", collected)
	}
	if collected.TraceID != exampleTraceID || collected.SpanID != exampleSpanID {
		t.Fatalf("the embedded context reads %+v", collected.TraceContext)
	}
}

func TestNewCollected(t *testing.T) {
	if got := NewCollected("", TraceContext{}); got.TraceParent != "" || got.ID != "" {
		t.Fatalf("an unrecorded transaction answered %+v", got)
	}
}
