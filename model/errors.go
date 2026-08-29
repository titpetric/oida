package model

import (
	"errors"
	"fmt"
)

// The errors this project returns. Every configuration failure wraps
// ErrInvalidOptions, so a caller can test for the class or for the case. The
// root package aliases each of these, so services keep spelling them oida.Err*.
var (
	// ErrNilRouter is returned when Mount is called without a router.
	ErrNilRouter = errors.New("oida: router is nil")

	// ErrInvalidOptions is the base error for every configuration failure.
	ErrInvalidOptions = errors.New("oida: invalid options")

	// ErrInvalidPath is returned when Options.Path is not an absolute path.
	ErrInvalidPath = fmt.Errorf("%w: path must be an absolute path", ErrInvalidOptions)

	// ErrInvalidSampleRate is returned when Options.SampleRate is outside
	// [0,100].
	ErrInvalidSampleRate = fmt.Errorf("%w: sample rate must be between 0 and 100", ErrInvalidOptions)

	// ErrTraceNotFound is returned when a trace ID is not in the ring buffer.
	ErrTraceNotFound = errors.New("oida: trace not found")

	// ErrDisabled is returned when a trace is requested from a disabled tracer.
	ErrDisabled = errors.New("oida: tracer is disabled")
)

// invalidOption formats a field level configuration error.
func invalidOption(field string, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidOptions, field, reason)
}
