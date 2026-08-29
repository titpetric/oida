package oida

import (
	"strings"
	"time"

	"github.com/titpetric/oida/model"
)

// Options configures telemetry behaviour, the debug front end and the
// middleware. It lives in the model package so the front end can read it
// without depending on the recorder; the alias keeps it spelled oida.Options.
type Options = model.Options

// DefaultPath is the default mount path of the debug front end.
const DefaultPath = model.DefaultPath

// NewOptions returns the default options for the named service.
func NewOptions(serviceName string) Options {
	return model.NewOptions(serviceName)
}

// samplerFor returns the configured sampler, or a rate sampler for SampleRate.
func samplerFor(o Options) Sampler {
	if o.Sampler != nil {
		return o.Sampler
	}
	return NewRateSampler(o.SampleRate)
}

// clockNow returns the current time from the configured clock.
func clockNow(o Options) time.Time {
	if o.Clock == nil {
		return time.Now()
	}
	return o.Clock()
}

// traceOptionsFor returns the part of the configuration a recorded trace
// carries.
func traceOptionsFor(o Options) model.TraceOptions {
	return model.TraceOptions{
		Service:  o.ServiceName,
		MaxSpans: o.MaxSpansPerTrace,
		Clock:    o.Clock,
	}
}

// ignoredPath reports whether a request path is excluded from tracing. The
// debug front end is always excluded so it does not trace itself.
func ignoredPath(o Options, path string) bool {
	if path == o.Path || strings.HasPrefix(path, o.Path+"/") {
		return true
	}
	for _, pattern := range o.IgnorePaths {
		if prefix, ok := strings.CutSuffix(pattern, "/*"); ok {
			if path == prefix || strings.HasPrefix(path, prefix+"/") {
				return true
			}
			continue
		}
		if path == pattern {
			return true
		}
	}
	return false
}
