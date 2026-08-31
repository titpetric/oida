package internal

import (
	"net/http"
	"testing"

	"github.com/titpetric/oida/model"
)

// everySampler traces every request, and is what a service sets when the rate
// cannot express its rule.
type everySampler struct{}

// Sample implements model.Sampler.
func (everySampler) Sample(*http.Request) bool { return true }

func TestSamplerFor(t *testing.T) {
	opts := model.NewOptions("test")
	opts.Sampler = everySampler{}
	if _, ok := SamplerFor(opts).(everySampler); !ok {
		t.Errorf("SamplerFor returned %T, want the configured sampler", SamplerFor(opts))
	}

	// Without one the rate builds it, and a rate of zero refuses everything.
	opts = model.NewOptions("test")
	opts.SampleRate = 0
	if SamplerFor(opts).Sample(nil) {
		t.Error("a rate of zero sampled a request")
	}

	opts.SampleRate = 100
	if !SamplerFor(opts).Sample(nil) {
		t.Error("a rate of 100 refused a request")
	}
}
