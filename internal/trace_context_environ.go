package internal

import (
	"github.com/titpetric/oida/model"
)

// TraceContextEnviron returns the TRACEPARENT and TRACESTATE variables a
// child process reads to join a trace. A context naming no trace returns
// nothing, so the command runs with the environment it would have had.
func TraceContextEnviron(context model.TraceContext) []string {
	value := context.TraceParent()
	if value == "" {
		return nil
	}
	env := []string{envTraceParent + "=" + value}
	if context.State != "" {
		env = append(env, envTraceState+"="+context.State)
	}
	return env
}
