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
