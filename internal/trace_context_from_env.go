package internal

import (
	"github.com/titpetric/oida/model"
)

// The variables a parent process sets for the command it runs. The bare pair
// is the convention the OpenTelemetry tooling settled on, which lets an
// instrumented process join a trace another vendor's tool opened; the
// namespaced pair wins where both are set.
const (
	envTraceParent = "TRACEPARENT"
	envTraceState  = "TRACESTATE"
)

// traceContextFromEnv applies TRACEPARENT and TRACESTATE to opts, each only
// where the code left the field empty.
func traceContextFromEnv(opts *model.Options) {
	if opts.TraceParent == "" {
		opts.TraceParent = firstEnv("OIDA_"+envTraceParent, envTraceParent)
	}
	if opts.TraceState == "" {
		opts.TraceState = firstEnv("OIDA_"+envTraceState, envTraceState)
	}
}

// firstEnv returns the first of the named variables that is set to something.
func firstEnv(names ...string) string {
	for _, name := range names {
		if value := envValue(name); value != "" {
			return value
		}
	}
	return ""
}
