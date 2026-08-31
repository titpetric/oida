package internal

import "github.com/titpetric/oida/model"

// SamplerFor returns the configured sampler, or a rate sampler for SampleRate.
func SamplerFor(o model.Options) model.Sampler {
	if o.Sampler != nil {
		return o.Sampler
	}
	return NewRateSampler(o.SampleRate)
}
