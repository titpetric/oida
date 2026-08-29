package oida

import (
	"sync"

	"github.com/titpetric/oida/model"
)

// defaultMu guards the creation of the process wide tracer. The slot itself
// lives in the model package, so the front end can read it without depending
// on this one.
var defaultMu sync.Mutex

// Default returns the process wide tracer, creating it with the default options
// on first use. Prefer an explicit tracer from New in libraries and tests.
func Default() *Tracer {
	if tracer := defaultTracer(); tracer != nil {
		return tracer
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if tracer := defaultTracer(); tracer != nil {
		return tracer
	}
	// NewOptions is always valid, so the error cannot occur.
	tracer, _ := New(NewOptions(""))
	model.SetDefaultRecorder(tracer)
	return tracer
}

// defaultTracer returns the process wide tracer, or nil when none is set.
func defaultTracer() *Tracer {
	tracer, _ := model.DefaultRecorder().(*Tracer)
	return tracer
}

// Configure replaces the process wide tracer with one built from opts and
// returns it. Call it once during startup, before wiring the middleware.
func Configure(opts Options) (*Tracer, error) {
	tracer, err := New(opts)
	if err != nil {
		return nil, err
	}

	model.SetDefaultRecorder(tracer)
	return tracer, nil
}

// Resolve returns the tracer the options point at: the explicit one when set,
// the process default otherwise. The first resolution of the default configures
// it from opts. The front end resolves the tracer it serves this way.
func Resolve(opts Options) (*Tracer, error) {
	if tracer, ok := opts.Tracer.(*Tracer); ok && tracer != nil {
		return tracer, nil
	}
	if tracer := defaultTracer(); tracer != nil {
		return tracer, nil
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if tracer := defaultTracer(); tracer != nil {
		return tracer, nil
	}
	tracer, err := New(opts)
	if err != nil {
		return nil, err
	}
	model.SetDefaultRecorder(tracer)
	return tracer, nil
}

// MustResolve returns the tracer for opts, falling back to the default tracer
// when the options are invalid. It backs the entry points that cannot report an
// error.
func MustResolve(opts Options) *Tracer {
	tracer, err := Resolve(opts)
	if err != nil || tracer == nil {
		return Default()
	}
	return tracer
}
