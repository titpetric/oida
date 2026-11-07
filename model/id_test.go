package model

import (
	"strings"
	"testing"
	"time"
)

func TestNewID(t *testing.T) {
	first, err := NewID(time.UnixMilli(1700000000000))
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if !ValidID(first) {
		t.Fatalf("%q is no ULID", first)
	}
	second, err := NewID(time.UnixMilli(1700000000001))
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if first >= second {
		t.Fatalf("%q does not sort before %q", first, second)
	}
}

func TestValidID(t *testing.T) {
	id, err := NewID(time.Now())
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if !ValidID(id) {
		t.Fatalf("%q is no ULID", id)
	}
	for _, value := range []string{"", strings.Repeat("0", 25), strings.Repeat("0", 27), strings.Repeat("U", 26)} {
		if ValidID(value) {
			t.Errorf("%q reads as a ULID", value)
		}
	}
}

func TestTraceIDHex(t *testing.T) {
	id, err := NewID(time.UnixMilli(1700000000000))
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	hex := TraceIDHex(id)
	if !ValidTraceID(hex) {
		t.Fatalf("TraceIDHex(%q) = %q, which is no trace identifier", id, hex)
	}
	if got := IDFromTraceID(hex); got != id {
		t.Fatalf("the round trip of %q came back as %q", id, got)
	}
	for _, value := range []string{"", "not-a-ulid", strings.Repeat("U", 26)} {
		if got := TraceIDHex(value); got != "" {
			t.Errorf("TraceIDHex(%q) = %q, want nothing", value, got)
		}
	}
}

func TestIDFromTraceID(t *testing.T) {
	// A trace identifier an OpenTelemetry SDK minted, which carries no
	// timestamp: it still has to land on a ULID oida can key a trace on.
	id := IDFromTraceID(exampleTraceID)
	if !ValidID(id) {
		t.Fatalf("IDFromTraceID(%q) = %q, which is no ULID", exampleTraceID, id)
	}
	if got := TraceIDHex(id); got != exampleTraceID {
		t.Fatalf("the round trip of %q came back as %q", exampleTraceID, got)
	}
	for _, value := range []string{"", exampleSpanID, strings.Repeat("0", 32), strings.ToUpper(exampleTraceID)} {
		if got := IDFromTraceID(value); got != "" {
			t.Errorf("IDFromTraceID(%q) = %q, want nothing", value, got)
		}
	}
}
