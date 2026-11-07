package view

import (
	"testing"
	"time"

	"github.com/titpetric/oida/model"
)

// traceOf returns a retained transaction of one W3C trace.
func traceOf(id, trace, span, parent string, at time.Time) model.Trace {
	return model.Trace{ID: id, TraceID: trace, SpanID: span, ParentSpanID: parent, StartedAt: at, Name: id}
}

func TestRelated(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	click := traceOf("click", "4bf92f3577b34da6a3ce929d0e0e4736", "aaaaaaaaaaaaaaa1", "", now)
	request := traceOf("request", "4bf92f3577b34da6a3ce929d0e0e4736", "aaaaaaaaaaaaaaa2", "aaaaaaaaaaaaaaa1", now.Add(time.Second))
	other := traceOf("other", "0af7651916cd43dd8448eb211c80319c", "bbbbbbbbbbbbbbb1", "", now)
	log := []model.Trace{request, other, click}

	related := Related(request, log)
	if len(related) != 1 || related[0].ID != "click" {
		t.Fatalf("the request is related to %+v", related)
	}
	if got := Related(request, []model.Trace{request}); got != nil {
		t.Fatalf("a trace nothing joined is related to %+v", got)
	}
	if got := Related(model.Trace{ID: "none"}, log); got != nil {
		t.Fatalf("a trace with no identifier is related to %+v", got)
	}

	// Oldest first: the chain reads in the order the work happened.
	ordered := Related(click, log)
	if len(ordered) != 1 || ordered[0].ID != "request" {
		t.Fatalf("the click is related to %+v", ordered)
	}
}

func TestCaller(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	click := traceOf("click", "4bf92f3577b34da6a3ce929d0e0e4736", "aaaaaaaaaaaaaaa1", "", now)
	request := traceOf("request", "4bf92f3577b34da6a3ce929d0e0e4736", "aaaaaaaaaaaaaaa2", "aaaaaaaaaaaaaaa1", now.Add(time.Second))

	caller, ok := Caller(request, []model.Trace{click})
	if !ok || caller.ID != "click" {
		t.Fatalf("the request was called by %+v, %v", caller, ok)
	}
	if _, ok := Caller(click, []model.Trace{request}); ok {
		t.Fatal("a trace nobody called reports a caller")
	}
	if _, ok := Caller(request, nil); ok {
		t.Fatal("a caller the log does not hold was reported")
	}
}
