package internal

import "github.com/titpetric/oida/model"

// TraceOptionsFor returns the part of the configuration a recorded trace
// carries.
func TraceOptionsFor(o model.Options) model.TraceOptions {
	return model.TraceOptions{
		Service:     o.ServiceName,
		MaxSpans:    o.MaxSpansPerTrace,
		Clock:       o.Clock,
		CaptureLogs: o.CaptureLogs,
	}
}
