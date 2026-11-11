package model

import (
	"strings"
	"testing"
)

// The example of W3C Trace Context section 3.2, which every parser is
// measured against.
const (
	exampleTraceID  = "4bf92f3577b34da6a3ce929d0e0e4736"
	exampleSpanID   = "00f067aa0ba902b7"
	exampleParent   = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	exampleUnsample = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"
)

func TestTraceContext(t *testing.T) {
	context := TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, Flags: FlagSampled, State: "rojo=00f067aa0ba902b7"}
	if !context.Valid() || !context.Sampled() {
		t.Fatalf("the example context reads invalid or unsampled: %+v", context)
	}
	if got := context.TraceParent(); got != exampleParent {
		t.Fatalf("traceparent is %q, want %q", got, exampleParent)
	}
}

func TestParseTraceParent(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		traceID string
		spanID  string
		sampled bool
	}{
		{name: "the specification example", value: exampleParent, traceID: exampleTraceID, spanID: exampleSpanID, sampled: true},
		{name: "an unsampled caller", value: exampleUnsample, traceID: exampleTraceID, spanID: exampleSpanID},
		{name: "a reserved flag bit", value: "00-" + exampleTraceID + "-" + exampleSpanID + "-03", traceID: exampleTraceID, spanID: exampleSpanID, sampled: true},
		{name: "surrounding whitespace", value: " " + exampleParent + " ", traceID: exampleTraceID, spanID: exampleSpanID, sampled: true},
		// The version format reserves the first three fields, so a later
		// version is read as those and its additions are dropped.
		{name: "a later version with extra fields", value: "cc-" + exampleTraceID + "-" + exampleSpanID + "-01-what-comes-later", traceID: exampleTraceID, spanID: exampleSpanID, sampled: true},
		{name: "a later version with none", value: "cc-" + exampleTraceID + "-" + exampleSpanID + "-01", traceID: exampleTraceID, spanID: exampleSpanID, sampled: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			context, err := ParseTraceParent(test.value)
			if err != nil {
				t.Fatalf("parse %q: %v", test.value, err)
			}
			if context.TraceID != test.traceID || context.SpanID != test.spanID || context.Sampled() != test.sampled {
				t.Fatalf("parsed %+v, want trace %s span %s sampled %v", context, test.traceID, test.spanID, test.sampled)
			}
		})
	}

	refused := map[string]string{
		"nothing":              "",
		"too short":            "00-" + exampleTraceID + "-" + exampleSpanID + "-0",
		"version ff":           "ff-" + exampleTraceID + "-" + exampleSpanID + "-01",
		"version 00 with more": exampleParent + "-later",
		"an all zero trace":    "00-00000000000000000000000000000000-" + exampleSpanID + "-01",
		"an all zero span":     "00-" + exampleTraceID + "-0000000000000000-01",
		"uppercase hex":        strings.ToUpper(exampleParent),
		"a missing delimiter":  "00-" + exampleTraceID + "." + exampleSpanID + "-01",
		"a version delimiter":  "00." + exampleTraceID + "-" + exampleSpanID + "-01",
		"non hex flags":        "00-" + exampleTraceID + "-" + exampleSpanID + "-zz",
	}
	for name, value := range refused {
		t.Run(name, func(t *testing.T) {
			if context, err := ParseTraceParent(value); err == nil {
				t.Fatalf("parsed %q as %+v, want a refusal", value, context)
			}
		})
	}
}

func TestTraceContext_TraceParent(t *testing.T) {
	if got := (TraceContext{TraceID: exampleTraceID}).TraceParent(); got != "" {
		t.Fatalf("a context naming no span wrote %q", got)
	}
	if got := TraceParentOf(exampleTraceID, exampleSpanID); got != exampleParent {
		t.Fatalf("TraceParentOf wrote %q, want %q", got, exampleParent)
	}
}

func TestTraceParentOf(t *testing.T) {
	if got := TraceParentOf("", exampleSpanID); got != "" {
		t.Fatalf("a header was written for no trace: %q", got)
	}
}

func TestTraceContext_Sampled(t *testing.T) {
	if (TraceContext{Flags: 0x02}).Sampled() {
		t.Fatal("a reserved bit on its own reads as sampled")
	}
}

func TestTraceContext_Valid(t *testing.T) {
	if (TraceContext{}).Valid() {
		t.Fatal("the zero context reads valid")
	}
}

func TestTraceContext_WithSpan(t *testing.T) {
	context := TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, State: "rojo=1"}.WithSpan("0000000000000009")
	if context.SpanID != "0000000000000009" || context.TraceID != exampleTraceID || context.State != "rojo=1" {
		t.Fatalf("rebinding the span gave %+v", context)
	}
}

func TestCleanTraceState(t *testing.T) {
	cases := map[string]string{
		"":                                    "",
		"rojo=00f067aa0ba902b7":               "rojo=00f067aa0ba902b7",
		" rojo=1 , congo=2 ":                  "rojo=1,congo=2",
		"rojo=1,,congo=2":                     "rojo=1,congo=2",
		"rojo=1,nonsense,congo=2":             "rojo=1,congo=2",
		"=novalue,rojo=1":                     "rojo=1",
		"tenant@system=1,rojo=00f067aa0ba902": "tenant@system=1,rojo=00f067aa0ba902",
	}
	for value, want := range cases {
		if got := CleanTraceState(value); got != want {
			t.Errorf("CleanTraceState(%q) = %q, want %q", value, got, want)
		}
	}

	members := make([]string, 0, 40)
	for i := range 40 {
		members = append(members, "key"+string(rune('a'+i%26))+string(rune('a'+i/26))+"=1")
	}
	cleaned := CleanTraceState(strings.Join(members, ","))
	if got := strings.Count(cleaned, ",") + 1; got != traceStateMembers {
		t.Fatalf("kept %d members, want the %d the specification allows", got, traceStateMembers)
	}
}

func TestValidTraceID(t *testing.T) {
	if !ValidTraceID(exampleTraceID) {
		t.Fatal("the example trace identifier reads invalid")
	}
	for _, id := range []string{"", exampleSpanID, strings.Repeat("0", 32), strings.ToUpper(exampleTraceID)} {
		if ValidTraceID(id) {
			t.Errorf("%q reads as a trace identifier", id)
		}
	}
}

func TestValidSpanID(t *testing.T) {
	if !ValidSpanID(exampleSpanID) {
		t.Fatal("the example span identifier reads invalid")
	}
	for _, id := range []string{"", exampleTraceID, strings.Repeat("0", 16), "00f067aa0ba902bg"} {
		if ValidSpanID(id) {
			t.Errorf("%q reads as a span identifier", id)
		}
	}
}

func TestSpanIDHex(t *testing.T) {
	cases := map[uint64]string{
		0:                    "",
		1:                    "0000000000000001",
		15:                   "000000000000000f",
		4096:                 "0000000000001000",
		65535:                "000000000000ffff",
		0xffffffffffffffff:   "ffffffffffffffff",
		0x00f067aa0ba902b700: "f067aa0ba902b700",
	}
	for value, want := range cases {
		if got := SpanIDHex(value); got != want {
			t.Errorf("SpanIDHex(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestSpanIDFor(t *testing.T) {
	const trace = "01K0000000000000000000000A"

	root := SpanIDFor(trace, 1)
	if !ValidSpanID(root) {
		t.Fatalf("the root span of %q is %q", trace, root)
	}
	// Every span of one transaction has an identifier of its own, and so does
	// the same sequence number of another transaction.
	seen := map[string]int{root: 1}
	for span := 2; span < 64; span++ {
		id := SpanIDFor(trace, span)
		if !ValidSpanID(id) {
			t.Fatalf("span %d reads %q", span, id)
		}
		if before, ok := seen[id]; ok {
			t.Fatalf("span %d and span %d share %q", span, before, id)
		}
		seen[id] = span
	}
	if other := SpanIDFor("01K0000000000000000000000B", 1); other == root {
		t.Fatalf("two transactions claim span %q", root)
	}

	for _, id := range []string{"", "not-a-ulid"} {
		if got := SpanIDFor(id, 1); got != "" {
			t.Errorf("SpanIDFor(%q, 1) = %q, want nothing", id, got)
		}
	}
	if got := SpanIDFor(trace, 0); got != "" {
		t.Errorf("SpanIDFor(%q, 0) = %q, want nothing", trace, got)
	}
}

func TestTrace_SetParent(t *testing.T) {
	trace := NewTrace("01K0000000000000000000000A", "GET /users", TraceOptions{})
	defer trace.Release()

	trace.SetParent(TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, Flags: FlagSampled, State: " rojo=1 ,bad"})
	if trace.TraceID != exampleTraceID || trace.ParentSpanID != exampleSpanID || trace.TraceState != "rojo=1" {
		t.Fatalf("the trace joined %q under %q with state %q", trace.TraceID, trace.ParentSpanID, trace.TraceState)
	}

	// A context naming no trace leaves the one the trace opened for itself.
	trace.SetParent(TraceContext{})
	if trace.TraceID != exampleTraceID {
		t.Fatalf("an invalid context overwrote the trace identifier: %q", trace.TraceID)
	}
}

func TestTrace_Parent(t *testing.T) {
	if context := (*Trace)(nil).Parent(); context.Valid() {
		t.Fatal("a nil trace reports a valid parent")
	}
	trace := NewTrace("01K0000000000000000000000A", "GET /users", TraceOptions{})
	defer trace.Release()
	trace.SetParent(TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, Flags: FlagSampled})
	if context := trace.Parent(); context.TraceParent() != exampleParent {
		t.Fatalf("the parent reads %q, want %q", context.TraceParent(), exampleParent)
	}
}

func TestTrace_SpanContext(t *testing.T) {
	trace := NewTrace("01K0000000000000000000000A", "GET /users", TraceOptions{})
	defer trace.Release()
	trace.SetParent(TraceContext{TraceID: exampleTraceID, SpanID: exampleSpanID, Flags: FlagSampled})

	// The trace's own span is the one it was given; a span below it is
	// derived from the trace and the sequence number.
	root := trace.SpanContext(1)
	if root.TraceID != exampleTraceID || root.SpanID != trace.SpanID {
		t.Fatalf("the root context reads %+v", root)
	}
	nested := trace.SpanContext(7)
	if nested.SpanID != SpanIDFor(trace.ID, 7) || nested.SpanID == root.SpanID {
		t.Fatalf("the nested context reads %+v", nested)
	}
	if _, err := ParseTraceParent(nested.TraceParent()); err != nil {
		t.Fatalf("the header a span wrote does not parse: %v", err)
	}
	if context := (*Trace)(nil).SpanContext(1); context.Valid() {
		t.Fatalf("a nil trace propagates %+v", context)
	}
}
