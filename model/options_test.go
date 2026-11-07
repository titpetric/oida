package model

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOptionsDebugSpanMinDuration(t *testing.T) {
	if got := NewOptions("test").DebugSpanMinDuration; got != time.Microsecond {
		t.Errorf("the default is %v, want a microsecond", got)
	}

	// An uninitialized Options takes the default the way the other numbers
	// do; one from NewOptions keeps an explicit zero, which turns the filter
	// off.
	if got := (Options{}).WithDefaults().DebugSpanMinDuration; got != time.Microsecond {
		t.Errorf("an empty Options defaults to %v, want a microsecond", got)
	}
	off := NewOptions("test")
	off.DebugSpanMinDuration = 0
	if got := off.WithDefaults().DebugSpanMinDuration; got != 0 {
		t.Errorf("an explicit zero became %v, want the filter off", got)
	}

	negative := NewOptions("test")
	negative.DebugSpanMinDuration = -time.Millisecond
	if err := negative.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("Validate = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_Parent(t *testing.T) {
	opts := NewOptions("test")
	if parent, err := opts.Parent(); err != nil || parent.Valid() {
		t.Fatalf("a process started outside a trace reports %+v, %v", parent, err)
	}

	opts.TraceParent, opts.TraceState = exampleParent, " rojo=1 ,bad"
	parent, err := opts.Parent()
	if err != nil {
		t.Fatalf("parent: %v", err)
	}
	if parent.TraceID != exampleTraceID || parent.SpanID != exampleSpanID || parent.State != "rojo=1" {
		t.Fatalf("the process joined %+v", parent)
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("a valid traceparent failed validation: %v", err)
	}

	opts.TraceParent = "nonsense"
	if _, err := opts.Parent(); !errors.Is(err, ErrInvalidTraceParent) {
		t.Fatalf("parent returned %v, want ErrInvalidTraceParent", err)
	}
	if err := opts.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate = %v, want ErrInvalidOptions", err)
	}
}

func TestOptionsAuthorized(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/oida", nil)

	open := NewOptions("test")
	if !open.Authorized(request) {
		t.Error("a nil Authorize denies; it must allow")
	}

	closed := NewOptions("test")
	closed.Authorize = func(*http.Request) bool { return false }
	if closed.Authorized(request) {
		t.Error("a denying Authorize allowed the request")
	}

	inspecting := NewOptions("test")
	inspecting.Authorize = func(r *http.Request) bool { return r.URL.Path == "/debug/oida" }
	if !inspecting.Authorized(request) {
		t.Error("Authorize did not receive the request it decides on")
	}
}
