package internal

import (
	"net/http"
	"sync/atomic"

	"github.com/titpetric/oida/model"
)

// RateSampler samples a fixed percentage of requests using a counter rather
// than randomness, so the decision sequence is deterministic and testable.
type RateSampler struct {
	// every is the sampling period: one request in every N is traced. Zero
	// disables sampling entirely.
	every uint64

	counter atomic.Uint64
}

var _ model.Sampler = (*RateSampler)(nil)

// NewRateSampler returns a sampler tracing the given percentage of requests. A
// rate of 100 or more traces everything, a rate of 0 or less traces nothing.
func NewRateSampler(rate float64) model.Sampler {
	switch {
	case rate >= 100:
		return &RateSampler{every: 1}
	case rate <= 0:
		return &RateSampler{every: 0}
	default:
		every := uint64(100/rate + 0.5)
		if every < 1 {
			every = 1
		}
		return &RateSampler{every: every}
	}
}

// Sample implements model.Sampler.
func (s *RateSampler) Sample(*http.Request) bool {
	switch s.every {
	case 0:
		return false
	case 1:
		return true
	default:
		return s.counter.Add(1)%s.every == 0
	}
}
