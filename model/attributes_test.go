package model

import (
	"math"
	"testing"
)

func TestAttributesInt64(t *testing.T) {
	attributes := Attributes{
		"int":      42,
		"int64":    int64(-7),
		"uint":     uint(9),
		"uint64":   uint64(math.MaxUint64),
		"float":    float64(12.9),
		"floatBig": math.MaxFloat64,
		"floatNaN": math.NaN(),
		"string":   "1204",
		"words":    "twelve",
		"bool":     true,
	}

	for key, want := range map[string]int64{
		"int":      42,
		"int64":    -7,
		"uint":     9,
		"uint64":   math.MaxInt64, // saturates at the bound
		"float":    12,            // truncates
		"floatBig": math.MaxInt64,
		"floatNaN": 0,
		"string":   1204,
	} {
		got, ok := attributes.Int64(key)
		if !ok {
			t.Errorf("Int64(%q) reports not a number", key)
			continue
		}
		if got != want {
			t.Errorf("Int64(%q) = %d, want %d", key, got, want)
		}
	}

	for _, key := range []string{"words", "bool", "missing"} {
		if got, ok := attributes.Int64(key); ok {
			t.Errorf("Int64(%q) = %d, want not a number", key, got)
		}
	}
}

func TestAttributesIsBytes(t *testing.T) {
	if !IsBytes(AttrMemoryLimit) || !IsBytes(AttrMemoryUsage) {
		t.Error("the memory keys do not read as byte sizes")
	}
	if IsBytes("tenant_id") {
		t.Error("a custom key reads as a byte size")
	}
}
