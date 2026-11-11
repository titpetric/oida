package oida

import (
	"context"
	"errors"
	"testing"

	"github.com/titpetric/oida/model"
)

// pageload is the transaction the browser library posts on load, with the two
// spans it reads out of the Navigation Timing entry.
func pageload() Transaction {
	return Transaction{
		TraceID:   exampleTraceID,
		SpanID:    "aaaaaaaaaaaaaaa1",
		Name:      "pageload /runs",
		Kind:      KindBrowser,
		StartedAt: 1700000000000,
		Duration:  820,
		Attributes: Attributes{
			"url":      "https://task.cdn.si/runs",
			"title":    "Runs",
			"referrer": "https://task.cdn.si/",
		},
		HTTP: &model.HTTPInfo{Method: "GET", URI: "/runs", Host: "task.cdn.si", Status: 200},
		Spans: []TransactionSpan{
			{Name: "documentFetch", Kind: KindExternal, Start: 12, Duration: 140},
			{Name: "domContentLoaded", Start: 420, Duration: 2},
		},
	}
}

func TestCollection(t *testing.T) {
	collection := Collection{Transactions: []Transaction{pageload()}}
	if err := collection.Validate(); err != nil {
		t.Fatalf("the collection is invalid: %v", err)
	}
	if err := (Collection{}).Validate(); !errors.Is(err, model.ErrInvalidTransaction) {
		t.Fatalf("an empty collection returned %v", err)
	}
}

func TestTracer_Record(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	collected, err := tracer.Record(context.Background(), pageload())
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if collected.TraceID != exampleTraceID {
		t.Fatalf("the transaction recorded under trace %q", collected.TraceID)
	}
	if collected.SpanID != "aaaaaaaaaaaaaaa1" {
		t.Fatalf("the span the client already named came back as %q", collected.SpanID)
	}
	if want := "00-" + exampleTraceID + "-aaaaaaaaaaaaaaa1-01"; collected.TraceParent != want {
		t.Fatalf("the receipt reads %q, want %q", collected.TraceParent, want)
	}

	trace, ok := tracer.Trace(collected.ID)
	if !ok {
		t.Fatalf("the transaction was not retained under %q", collected.ID)
	}
	if trace.Name != "pageload /runs" || trace.InFlight {
		t.Fatalf("unexpected trace: %+v", trace)
	}
	if len(trace.Spans) != 3 || trace.Spans[1].Kind != KindExternal {
		t.Fatalf("recorded %d spans: %+v", len(trace.Spans), trace.Spans)
	}
	if trace.HTTP == nil || trace.HTTP.Host != "task.cdn.si" {
		t.Fatalf("the page host was not recorded: %+v", trace.HTTP)
	}

	snapshot := tracer.Snapshot()
	if snapshot.Total != 1 || snapshot.Sampled != 1 {
		t.Fatalf("counters read total %d sampled %d", snapshot.Total, snapshot.Sampled)
	}
	if snapshot.Statistics.Hosts[0].Host != "task.cdn.si" {
		t.Fatalf("the host view lists %+v", snapshot.Statistics.Hosts)
	}
}

func TestTracer_RecordRefusesAnInvalidTransaction(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	if _, err := tracer.Record(context.Background(), Transaction{}); !errors.Is(err, model.ErrInvalidTransaction) {
		t.Fatalf("record returned %v, want ErrInvalidTransaction", err)
	}
	if traces := tracer.Traces(); len(traces) != 0 {
		t.Fatalf("a refused transaction was retained: %+v", traces)
	}
}

func TestTracer_RecordOnADisabledTracer(t *testing.T) {
	tracer, _ := newTestTracer(t, func(o *Options) { o.Enabled = false })

	if _, err := tracer.Record(context.Background(), pageload()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("record returned %v, want ErrDisabled", err)
	}
}

func TestTracer_RecordDerivesASpanOfItsOwn(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	tx := pageload()
	tx.SpanID = ""
	collected, err := tracer.Record(context.Background(), tx)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if !model.ValidSpanID(collected.SpanID) {
		t.Fatalf("the recorder derived span %q", collected.SpanID)
	}

	// Two transactions of one trace must not claim the same span: a
	// parent-id names a span inside the trace, not inside the transaction.
	second, err := tracer.Record(context.Background(), tx)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if second.SpanID == collected.SpanID {
		t.Fatalf("both transactions claim span %q", second.SpanID)
	}
}
