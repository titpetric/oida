package model

import (
	"context"
	"testing"
	"time"
)

func TestTraceAttribute(t *testing.T) {
	trace, _ := spanTestTrace()
	trace.SetAttribute("tenant_id", "acme-eu")

	if value, ok := trace.Attribute("tenant_id"); !ok || value != "acme-eu" {
		t.Errorf("Attribute(tenant_id) = %v, %v", value, ok)
	}
	if _, ok := trace.Attribute("missing"); ok {
		t.Error("a missing attribute reports recorded")
	}
	if _, ok := (*Trace)(nil).Attribute("tenant_id"); ok {
		t.Error("a nil trace reports a recorded attribute")
	}
}

func TestTraceKinds(t *testing.T) {
	trace, _ := spanTestTrace()
	ctx := context.Background()
	trace.StartSpan(ctx, "SELECT users", KindDatabase)
	trace.StartSpan(ctx, "SELECT orders", KindDatabase)
	trace.StartSpan(ctx, "GET pricing", KindExternal)

	kinds := trace.Kinds()
	if len(kinds) != 2 || kinds[0] != KindDatabase || kinds[1] != KindExternal {
		t.Errorf("Kinds() = %v, want database then external in first use order", kinds)
	}

	if !trace.HasKind(KindDatabase) {
		t.Error("HasKind misses a recorded kind")
	}
	if trace.HasKind(KindCache) {
		t.Error("HasKind reports a kind no span recorded")
	}
	if (*Trace)(nil).HasKind(KindDatabase) {
		t.Error("a nil trace reports a kind")
	}
}

func TestTraceElapsed(t *testing.T) {
	trace, now := spanTestTrace()

	*now = now.Add(7 * time.Millisecond)
	if got := trace.Elapsed(); got != 7*time.Millisecond {
		t.Errorf("Elapsed in flight = %v, want 7ms", got)
	}

	trace.Finish()
	*now = now.Add(time.Second)
	if got := trace.Elapsed(); got != 7*time.Millisecond {
		t.Errorf("Elapsed after Finish = %v, want the recorded 7ms", got)
	}
	if got := (*Trace)(nil).Elapsed(); got != 0 {
		t.Errorf("Elapsed on a nil trace = %v, want 0", got)
	}
}
