package oida

import (
	"fmt"

	"github.com/titpetric/oida/model"
)

// The errors this package returns. Every configuration failure wraps
// ErrInvalidOptions, so a caller can test for the class or for the case. The
// values live in the model package so the front end can return them too; these
// are the same error values, so errors.Is works with either spelling.
var (
	// ErrNilRouter is returned when Mount is called without a router.
	ErrNilRouter = model.ErrNilRouter

	// ErrNoTracer is returned when Mount is called without a tracer, which is
	// a dashboard with nothing to show.
	ErrNoTracer = model.ErrNoTracer

	// ErrInvalidOptions is the base error for every configuration failure.
	ErrInvalidOptions = model.ErrInvalidOptions

	// ErrInvalidPath is returned when Options.Path is not an absolute path.
	ErrInvalidPath = model.ErrInvalidPath

	// ErrInvalidSampleRate is returned when Options.SampleRate is outside
	// [0,100].
	ErrInvalidSampleRate = model.ErrInvalidSampleRate

	// ErrTraceNotFound is returned when a trace ID is not in the ring buffer.
	ErrTraceNotFound = model.ErrTraceNotFound

	// ErrDisabled is returned when a trace is requested from a disabled tracer.
	ErrDisabled = model.ErrDisabled
)

// invalidOption formats a field level configuration error.
func invalidOption(field string, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidOptions, field, reason)
}
