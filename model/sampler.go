package model

import "net/http"

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
