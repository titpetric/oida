package oida

import (
	"testing"
	"time"
)

// timelineTrace builds a trace with nested spans at fixed offsets.
func timelineTrace() Trace {
	start := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	span := func(id, parent, depth int, name string, kind Kind, offset, duration time.Duration) *Span {
		return &Span{
			ID:        id,
			ParentID:  parent,
			TraceID:   "TRACE",
			Name:      name,
			Kind:      kind,
			Depth:     depth,
			StartedAt: start.Add(offset),
			Duration:  duration,
		}
	}

	return Trace{
		ID:        "TRACE",
		Name:      "GET /users/{id}",
		StartedAt: start,
		Duration:  100 * time.Millisecond,
		Spans: []*Span{
			span(1, 0, 0, "GET /users/1", KindHTTP, 0, 100*time.Millisecond),
			span(2, 1, 1, "handler", KindInternal, 10*time.Millisecond, 70*time.Millisecond),
			span(3, 2, 2, "SELECT users", KindDatabase, 20*time.Millisecond, 30*time.Millisecond),
			span(4, 1, 1, "render", KindTemplate, 80*time.Millisecond, 20*time.Millisecond),
		},
	}
}

func TestTimelineAttributesInnermostSpan(t *testing.T) {
	segments := Timeline(timelineTrace())

	want := []struct {
		kind     Kind
		offset   time.Duration
		duration time.Duration
	}{
		{KindInternal, 10 * time.Millisecond, 10 * time.Millisecond},
		{KindDatabase, 20 * time.Millisecond, 30 * time.Millisecond},
		{KindInternal, 50 * time.Millisecond, 30 * time.Millisecond},
		{KindTemplate, 80 * time.Millisecond, 20 * time.Millisecond},
	}
	if len(segments) != len(want) {
		t.Fatalf("produced %d segments, want %d: %+v", len(segments), len(want), segments)
	}
	for i, expected := range want {
		got := segments[i]
		if got.Kind != expected.kind || got.Offset != expected.offset || got.Duration != expected.duration {
			t.Errorf("segment %d is %s at %s for %s, want %s at %s for %s",
				i, got.Kind, got.Offset, got.Duration, expected.kind, expected.offset, expected.duration)
		}
	}
	if segments[1].Share != 30 || segments[1].OffsetShare != 20 {
		t.Errorf("database share is %.2f%% at %.2f%%, want 30%% at 20%%", segments[1].Share, segments[1].OffsetShare)
	}
}

func TestTimelineWithoutDurationIsEmpty(t *testing.T) {
	trace := timelineTrace()
	trace.Duration = 0
	if segments := Timeline(trace); segments != nil {
		t.Fatalf("produced %d segments for an unfinished trace", len(segments))
	}
}

func TestRowsAreDepthFirst(t *testing.T) {
	rows := Rows(timelineTrace())

	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	want := []string{"GET /users/1", "handler", "SELECT users", "render"}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("rows are %v, want %v", names, want)
		}
	}

	if rows[2].Depth != 2 || rows[2].OffsetShare != 20 || rows[2].Share != 30 {
		t.Errorf("database row: depth %d, offset %.2f%%, share %.2f%%", rows[2].Depth, rows[2].OffsetShare, rows[2].Share)
	}
	if !rows[3].Last || rows[1].Last {
		t.Errorf("last-child flags are wrong: handler=%v render=%v", rows[1].Last, rows[3].Last)
	}
}

func TestRateSamplerIsDeterministic(t *testing.T) {
	sampler := NewRateSampler(0.5)

	sampled := 0
	for range 10 {
		if sampler.Sample(nil) {
			sampled++
		}
	}
	if sampled != 5 {
		t.Fatalf("sampled %d of 10 at rate 0.5, want 5", sampled)
	}

	if NewRateSampler(0).Sample(nil) {
		t.Error("rate 0 sampled a request")
	}
	if !NewRateSampler(1).Sample(nil) {
		t.Error("rate 1 rejected a request")
	}
}

func TestOptionsDefaultsAndValidation(t *testing.T) {
	opts := Options{}.WithDefaults()
	if opts.Path != DefaultPath || opts.RingBufferSize != 200 || opts.SampleRate != 1 || opts.Clock == nil {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("defaults are invalid: %v", err)
	}

	trimmed := Options{Path: "/debug/oida/"}.WithDefaults()
	if trimmed.Path != DefaultPath {
		t.Fatalf("path is %q, want the trailing slash trimmed", trimmed.Path)
	}

	if !opts.ignored(DefaultPath + "/stats") {
		t.Error("the front end path is traced")
	}
	if !opts.ignored("/healthz") || opts.ignored("/users/1") {
		t.Error("ignore paths do not match as documented")
	}

	prefixed := NewOptions()
	prefixed.IgnorePaths = []string{"/assets/*"}
	if !prefixed.ignored("/assets/app.css") || prefixed.ignored("/assetsx") {
		t.Error("prefix ignore patterns do not match as documented")
	}
}
