package model

import (
	"context"
	"testing"
)

// countingRecorder records nothing and answers every transaction, which is
// all Collect asks of a recorder.
type countingRecorder struct {
	Recorder

	recorded int
	refuse   bool
}

var _ Recorder = (*countingRecorder)(nil)

// Record implements the write side Collect calls, counting what it was
// handed.
func (r *countingRecorder) Record(context.Context, Transaction) (Collected, error) {
	if r.refuse {
		return Collected{}, ErrDisabled
	}
	r.recorded++
	return NewCollected("01K0000000000000000000000A", TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, Flags: FlagSampled}), nil
}

func TestRecorder(t *testing.T) {
	var recorder Recorder = &countingRecorder{}
	collected, err := recorder.Record(context.Background(), browserTransaction())
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if collected.TraceParent != exampleParent {
		t.Fatalf("the recorder answered %+v", collected)
	}
}
