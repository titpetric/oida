package oida

import (
	"net/http"
	"sync/atomic"
)

// Sampler decides whether a request is traced. The decision is taken before a
// trace is allocated, so rejecting a request costs one interface call.
type Sampler interface {
	Sample(r *http.Request) bool
}

// SamplerFunc adapts a function to the Sampler interface.
type SamplerFunc func(r *http.Request) bool

// Sample implements Sampler.
func (f SamplerFunc) Sample(r *http.Request) bool {
	return f(r)
}

// rateSampler samples a fixed fraction of requests using a counter rather than
// randomness, so the decision sequence is deterministic and testable.
type rateSampler struct {
	// every is the sampling period: one request in every N is traced. Zero
	// disables sampling entirely.
	every uint64

	counter atomic.Uint64
}

var _ Sampler = (*rateSampler)(nil)

// NewRateSampler returns a sampler tracing the given fraction of requests. A
// rate of 1 or more traces everything, a rate of 0 or less traces nothing.
func NewRateSampler(rate float64) Sampler {
	switch {
	case rate >= 1:
		return &rateSampler{every: 1}
	case rate <= 0:
		return &rateSampler{every: 0}
	default:
		every := uint64(1/rate + 0.5)
		if every < 1 {
			every = 1
		}
		return &rateSampler{every: every}
	}
}

// Sample implements Sampler.
func (s *rateSampler) Sample(*http.Request) bool {
	switch s.every {
	case 0:
		return false
	case 1:
		return true
	default:
		return s.counter.Add(1)%s.every == 0
	}
}
