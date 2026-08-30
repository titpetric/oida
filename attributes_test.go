package oida

import (
	"context"
	"math"
	"testing"

	"github.com/titpetric/oida/model"
)

func TestAttributesInt64(t *testing.T) {
	for name, test := range map[string]struct {
		value any
		want  int64
		ok    bool
	}{
		"int":      {value: 1024, want: 1024, ok: true},
		"int64":    {value: int64(1 << 20), want: 1 << 20, ok: true},
		"uint64":   {value: uint64(4096), want: 4096, ok: true},
		"float":    {value: 10240.0, want: 10240, ok: true},
		"string":   {value: "10240", want: 10240, ok: true},
		"words":    {value: "10 KiB", want: 0, ok: false},
		"boolean":  {value: true, want: 0, ok: false},
		"missing":  {value: nil, want: 0, ok: false},
		"negative": {value: -1, want: -1, ok: true},
		"huge":     {value: uint64(math.MaxUint64), want: math.MaxInt64, ok: true},
		"nan":      {value: math.NaN(), want: 0, ok: true},
	} {
		t.Run(name, func(t *testing.T) {
			attributes := Attributes{}
			if test.value != nil {
				attributes[AttrMemoryUsage] = test.value
			}
			got, ok := attributes.Int64(AttrMemoryUsage)
			if got != test.want || ok != test.ok {
				t.Fatalf("Int64(%v) = %d, %v; want %d, %v", test.value, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestIsBytes(t *testing.T) {
	for key, want := range map[string]bool{
		AttrMemoryLimit: true,
		AttrMemoryUsage: true,
		"rows":          false,
		"":              false,
	} {
		if got := model.IsBytes(key); got != want {
			t.Errorf("IsBytes(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestTraceAttributes(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	err := tracer.Observe(context.Background(), "job", func(ctx context.Context) error {
		trace := TraceFromContext(ctx)
		trace.SetAttribute(AttrMemoryLimit, int64(1<<20))
		trace.SetAttributes(Attributes{AttrMemoryUsage: int64(30 << 10)})

		// An empty key is not an attribute, and neither is a nil trace.
		trace.SetAttribute("", "ignored")
		var missing *Trace
		missing.SetAttribute(AttrMemoryUsage, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	trace := tracer.Traces()[0]
	if got, _ := trace.Attributes.Int64(AttrMemoryLimit); got != 1<<20 {
		t.Fatalf("memory limit %d, want %d", got, 1<<20)
	}
	if got, _ := trace.Attributes.Int64(AttrMemoryUsage); got != 30<<10 {
		t.Fatalf("memory usage %d, want %d", got, 30<<10)
	}
	if len(trace.Attributes) != 2 {
		t.Fatalf("recorded %d attributes, want 2", len(trace.Attributes))
	}
}

func TestTraceAttributesAreCloned(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)
	_, trace, err := tracer.StartTrace(context.Background(), "job")
	if err != nil {
		t.Fatalf("StartTrace: %v", err)
	}
	defer tracer.Finish(trace)
	trace.SetAttribute(AttrMemoryLimit, int64(1<<20))

	copied := trace.Clone()
	copied.Attributes[AttrMemoryLimit] = int64(1)

	if got, _ := trace.Attribute(AttrMemoryLimit); got != int64(1<<20) {
		t.Fatalf("the recorded limit reads %v after the copy was written to", got)
	}
}
