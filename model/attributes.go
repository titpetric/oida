package model

import (
	"math"
	"strconv"
)

// Well known attribute keys. The set is open; these are the ones the front end
// renders as sizes rather than as bare integers.
const (
	// AttrMemoryLimit is the memory a transaction was allowed to use, in bytes.
	// It is recorded on the trace.
	AttrMemoryLimit = "memory_limit"

	// AttrMemoryUsage is the memory in use when a span or a trace finished, in
	// bytes.
	AttrMemoryUsage = "memory_usage"
)

// byteKeys are the attribute names whose value is a size in bytes.
var byteKeys = map[string]struct{}{
	AttrMemoryLimit: {},
	AttrMemoryUsage: {},
}

// IsBytes reports whether key holds a size in bytes.
func IsBytes(key string) bool {
	_, ok := byteKeys[key]
	return ok
}

// Int64 returns an attribute as an integer, and whether it was a number. A
// value decoded from JSON as a float, or carried as a decimal string, reads as
// the number it is.
func (a Attributes) Int64(key string) (int64, bool) {
	switch value := a[key].(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		return clampUint(uint64(value)), true
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		return clampUint(value), true
	case float32:
		return clampFloat(float64(value)), true
	case float64:
		return clampFloat(value), true
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// clampUint narrows an unsigned value, saturating at the int64 bound.
func clampUint(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}

// clampFloat truncates a float value, saturating at the int64 bounds. A NaN
// reads as zero.
func clampFloat(value float64) int64 {
	switch {
	case math.IsNaN(value):
		return 0
	case value >= math.MaxInt64:
		return math.MaxInt64
	case value <= math.MinInt64:
		return math.MinInt64
	default:
		return int64(value)
	}
}
