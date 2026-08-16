package model

import "time"

// TraceOptions is the configuration a trace is recorded with. The recorder
// derives it from its own options, so this package stays free of them.
type TraceOptions struct {
	// Service is recorded on every trace, so a snapshot names the process it
	// came from.
	Service string

	// MaxSpans bounds the spans one trace records. Excess spans are counted in
	// Trace.DroppedSpans. Zero means unlimited.
	MaxSpans int

	// Clock is the time source of the trace. A nil clock uses time.Now.
	Clock func() time.Time
}

// now returns the current time from the configured clock.
func (o TraceOptions) now() time.Time {
	if o.Clock == nil {
		return time.Now()
	}
	return o.Clock()
}
