package model

import (
	"strings"
	"testing"
	"time"
)

// browserTransaction is a pageload the browser library posts, with the two
// spans it reads out of the Navigation Timing entry.
func browserTransaction() Transaction {
	return Transaction{
		TraceID:    exampleTraceID,
		ParentID:   exampleSpanID,
		TraceState: "rojo=1",
		Name:       "pageload /runs",
		Kind:       KindBrowser,
		StartedAt:  1700000000000,
		Duration:   820,
		Attributes: Attributes{"url": "https://task.cdn.si/runs", "title": "Runs"},
		Spans: []TransactionSpan{
			{Name: "documentFetch", Start: 12, Duration: 140},
			{Name: "responseEnd", Start: 152, Duration: 0, Parent: 1},
		},
	}
}

func TestTransaction(t *testing.T) {
	if err := browserTransaction().Validate(); err != nil {
		t.Fatalf("the browser transaction is invalid: %v", err)
	}
}

func TestTransactionSpan(t *testing.T) {
	tx := browserTransaction()
	tx.Spans[1].Parent = 2
	if err := tx.Validate(); err == nil {
		t.Fatal("a span nesting under itself was accepted")
	}
	tx.Spans[1].Parent = 0
	tx.Spans[1].Name = ""
	if err := tx.Validate(); err == nil {
		t.Fatal("a span with no name was accepted")
	}
}

func TestTransaction_Validate(t *testing.T) {
	cases := map[string]func(*Transaction){
		"no name":          func(tx *Transaction) { tx.Name = "" },
		"a name too long":  func(tx *Transaction) { tx.Name = strings.Repeat("x", maxTransactionName+1) },
		"a negative time":  func(tx *Transaction) { tx.Duration = -1 },
		"a negative start": func(tx *Transaction) { tx.StartedAt = -1 },
		"a bad trace":      func(tx *Transaction) { tx.TraceID = "nope" },
		"a bad parent":     func(tx *Transaction) { tx.ParentID = "nope" },
		"too many spans":   func(tx *Transaction) { tx.Spans = make([]TransactionSpan, maxTransactionSpans+1) },
	}
	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			tx := browserTransaction()
			break_(&tx)
			if err := tx.Validate(); err == nil {
				t.Fatal("the transaction was accepted")
			}
		})
	}
}

func TestTransaction_Trace(t *testing.T) {
	now := time.UnixMilli(1700000000900)
	trace := browserTransaction().Trace("01K0000000000000000000000A", TraceOptions{Service: "task-ui", Clock: func() time.Time { return now }})

	if trace.TraceID != exampleTraceID || trace.ParentSpanID != exampleSpanID || trace.TraceState != "rojo=1" {
		t.Fatalf("the trace joined %q under %q with state %q", trace.TraceID, trace.ParentSpanID, trace.TraceState)
	}
	if trace.Service != "task-ui" || trace.Name != "pageload /runs" {
		t.Fatalf("unexpected identity: service %q name %q", trace.Service, trace.Name)
	}
	if trace.StartedAt.UnixMilli() != 1700000000000 || trace.Duration != 820*time.Millisecond {
		t.Fatalf("the client times were not kept: %s for %s", trace.StartedAt, trace.Duration)
	}
	if trace.InFlight || !trace.finished {
		t.Fatal("a posted transaction reads as still running")
	}
	if len(trace.Spans) != 3 {
		t.Fatalf("recorded %d spans, want the transaction's own and its two", len(trace.Spans))
	}
	root, fetch, nested := trace.Spans[0], trace.Spans[1], trace.Spans[2]
	if root.ID != 1 || root.ParentID != 0 || root.Kind != KindBrowser {
		t.Fatalf("unexpected root span: %+v", root)
	}
	if fetch.ID != 2 || fetch.ParentID != 1 || fetch.Depth != 1 || fetch.StartedAt.UnixMilli() != 1700000000012 {
		t.Fatalf("unexpected fetch span: %+v", fetch)
	}
	if nested.ID != 3 || nested.ParentID != 2 || nested.Depth != 2 {
		t.Fatalf("unexpected nested span: %+v", nested)
	}
}

func TestTransaction_TraceOpensItsOwn(t *testing.T) {
	tx := browserTransaction()
	tx.TraceID, tx.ParentID = "", ""
	trace := tx.Trace("01K0000000000000000000000A", TraceOptions{})

	if want := TraceIDHex("01K0000000000000000000000A"); trace.TraceID != want {
		t.Fatalf("the trace opened %q, want its own %q", trace.TraceID, want)
	}
	if trace.ParentSpanID != "" {
		t.Fatalf("a trace nobody called named a parent: %q", trace.ParentSpanID)
	}
}

func TestTransaction_TraceCorrectsTheClientClock(t *testing.T) {
	now := time.UnixMilli(1700000000900)
	tx := browserTransaction()
	// A browser clock a year out: the start gives way to the recorder's.
	tx.StartedAt = float64(time.UnixMilli(1700000000000).AddDate(1, 0, 0).UnixMilli())
	trace := tx.Trace("01K0000000000000000000000A", TraceOptions{Clock: func() time.Time { return now }})

	if want := now.Add(-820 * time.Millisecond); !trace.StartedAt.Equal(want) {
		t.Fatalf("the start reads %s, want the recorder's %s", trace.StartedAt, want)
	}
}

func TestTransaction_TraceRecordsAnError(t *testing.T) {
	tx := browserTransaction()
	tx.Error = "the request was refused"
	trace := tx.Trace("01K0000000000000000000000A", TraceOptions{})

	if trace.State != StateError || !trace.Failed() {
		t.Fatalf("a failed transaction recorded state %q", trace.State)
	}
}
