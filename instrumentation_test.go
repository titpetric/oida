package oida

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// startTestTrace returns a context carrying a live trace on a private tracer,
// which is what the helpers need to record anything.
func startTestTrace(t *testing.T) (context.Context, *Trace) {
	t.Helper()

	tracer, _ := newTestTracer(t, nil)
	ctx, trace, err := tracer.StartTrace(context.Background(), "test")
	if err != nil {
		t.Fatalf("StartTrace: %v", err)
	}
	return ctx, trace
}

func TestRecordError(t *testing.T) {
	ctx, trace := startTestTrace(t)

	want := errors.New("disk full")
	ctx, span := Start(ctx, "save user")
	RecordError(ctx, fmt.Errorf("save user: %w", want))
	span.End()

	// The recorded value carries to both, so a wrapped error still unwraps.
	if err := span.Err(); !errors.Is(err, want) {
		t.Fatalf("span error %v, want %v", err, want)
	}
	if err := trace.Err(); !errors.Is(err, want) {
		t.Fatalf("trace error %v, want %v", err, want)
	}
}

func TestRecordErrorIsNilSafe(t *testing.T) {
	ctx, _ := startTestTrace(t)

	// A nil error is not a failure, and a context without a span has nothing
	// to record onto. Neither may panic.
	RecordError(ctx, nil)
	RecordError(context.Background(), errors.New("nowhere to go"))
}

func TestStartRequest(t *testing.T) {
	ctx, trace := startTestTrace(t)

	request := httptest.NewRequest(http.MethodGet, "/users/42", nil).WithContext(ctx)
	request, span := StartRequest(request, "user.Handler", KindHTTP)
	if span == nil {
		t.Fatal("no span recorded for a traced request")
	}

	// The returned request carries the span, so work below it nests.
	_, child := Start(request.Context(), "load user", KindDatabase)
	child.End()
	span.End()

	if got := trace.SpanCount(); got != 3 {
		t.Fatalf("recorded %d spans, want 3 (root, handler, query)", got)
	}
	if SpanFromContext(request.Context()) != span {
		t.Fatal("returned request does not carry the span")
	}
}

func TestStartRequestWithoutTrace(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)

	got, span := StartRequest(request, "user.Handler")
	if span != nil {
		t.Fatal("recorded a span without a trace in the context")
	}
	if got != request {
		t.Fatal("untraced request was copied, it should pass through unchanged")
	}
	span.End()
}

func TestStartAutoNamesTheSymbol(t *testing.T) {
	ctx, trace := startTestTrace(t)

	store := userStore{}
	_, span := StartAuto(ctx, store.Get, KindDatabase)
	span.End()

	spans := trace.Clone().Spans
	if len(spans) != 2 {
		t.Fatalf("recorded %d spans, want 2", len(spans))
	}
	if spans[1].Name != "oida.userStore.Get" {
		t.Fatalf("span name is %q, want the symbol name", spans[1].Name)
	}
	if spans[1].Kind != KindDatabase {
		t.Fatalf("span kind is %q, want %q", spans[1].Kind, KindDatabase)
	}
}

// userStore is the receiver StartAuto reads a name from.
type userStore struct{}

// Get is the method under test.
func (userStore) Get() {}
