package view

import (
	"math"
	"testing"
	"time"
)

func TestDurationText(t *testing.T) {
	tests := map[time.Duration]string{
		0:                       "0s",
		-time.Second:            "0s",
		1500 * time.Nanosecond:  "2µs",
		time.Millisecond:        "1ms",
		90 * time.Second:        "1m30s",
		1500 * time.Microsecond: "1.5ms",
	}

	for d, want := range tests {
		if got := durationText(d); got != want {
			t.Errorf("durationText(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestPreciseText walks every magnitude the renderer switches unit at. The
// front end reaches it with whatever a request took, so the branch it lands on
// is the one the traffic decided; the coverage of the others is only ever
// accidental.
func TestPreciseText(t *testing.T) {
	tests := map[time.Duration]string{
		0:                       "0",
		-time.Second:            "0",
		300 * time.Nanosecond:   "300ns",
		time.Microsecond:        "1.0µs",
		1500 * time.Nanosecond:  "1.5µs",
		time.Millisecond:        "1.00ms",
		1500 * time.Microsecond: "1.50ms",
		time.Second:             "1.00s",
		30 * time.Second:        "30.00s",
	}

	for d, want := range tests {
		if got := preciseText(d); got != want {
			t.Errorf("preciseText(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestBytesText(t *testing.T) {
	tests := map[uint64]string{
		0:                 "0 B",
		1023:              "1023 B",
		1024:              "1.0 KiB",
		1536:              "1.5 KiB",
		1 << 20:           "1.0 MiB",
		1 << 30:           "1.0 GiB",
		1 << 40:           "1.0 TiB",
		1 << 50:           "1.0 PiB",
		1 << 60:           "1.0 EiB",
		math.MaxUint64:    "16.0 EiB",
		3 * (1 << 20) / 2: "1.5 MiB",
	}

	for n, want := range tests {
		if got := bytesText(n); got != want {
			t.Errorf("bytesText(%d) = %q, want %q", n, got, want)
		}
	}
}

// TestSignedBytesText covers both signs, which a run of the front end does not:
// the delta it renders is the heap moving under the process, so whether it is
// negative on any given run is the allocator's business.
func TestSignedBytesText(t *testing.T) {
	tests := map[int64]string{
		0:             "0 B",
		1024:          "1.0 KiB",
		-1024:         "-1.0 KiB",
		-1:            "-1 B",
		math.MaxInt64: "8.0 EiB",
		math.MinInt64: "-8.0 EiB",
		1 << 20:       "1.0 MiB",
		-(3 << 19):    "-1.5 MiB",
	}

	for n, want := range tests {
		if got := signedBytesText(n); got != want {
			t.Errorf("signedBytesText(%d) = %q, want %q", n, got, want)
		}
	}
}
