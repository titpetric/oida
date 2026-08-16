package oida

import "sync"

var (
	defaultMu     sync.RWMutex
	defaultTracer *Tracer
)

// Default returns the process wide tracer, creating it with the default options
// on first use. Prefer an explicit tracer from New in libraries and tests.
func Default() *Tracer {
	defaultMu.RLock()
	tracer := defaultTracer
	defaultMu.RUnlock()
	if tracer != nil {
		return tracer
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultTracer == nil {
		// NewOptions is always valid, so the error cannot occur.
		defaultTracer, _ = New(NewOptions())
	}
	return defaultTracer
}

// Configure replaces the process wide tracer with one built from opts and
// returns it. Call it once during startup, before wiring the middleware.
func Configure(opts Options) (*Tracer, error) {
	tracer, err := New(opts)
	if err != nil {
		return nil, err
	}

	defaultMu.Lock()
	defaultTracer = tracer
	defaultMu.Unlock()
	return tracer, nil
}

// resolve returns the tracer the options point at: the explicit one when set,
// the process default otherwise. The first resolution of the default configures
// it from opts.
func resolve(opts Options) (*Tracer, error) {
	if opts.Tracer != nil {
		return opts.Tracer, nil
	}

	defaultMu.RLock()
	tracer := defaultTracer
	defaultMu.RUnlock()
	if tracer != nil {
		return tracer, nil
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultTracer != nil {
		return defaultTracer, nil
	}
	tracer, err := New(opts)
	if err != nil {
		return nil, err
	}
	defaultTracer = tracer
	return tracer, nil
}

// mustResolve returns the tracer for opts, falling back to the default tracer
// when the options are invalid. It backs the entry points that cannot report an
// error.
func mustResolve(opts Options) *Tracer {
	tracer, err := resolve(opts)
	if err != nil || tracer == nil {
		return Default()
	}
	return tracer
}
