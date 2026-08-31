package oida

import "net/http"

// Sampler decides whether a request is traced. The decision is taken before a
// trace is allocated, so rejecting a request costs one interface call. The
// definition is a copy of the model's; interfaces are structural, so a sampler
// written against either spelling works everywhere one is accepted.
//
// Options.SampleRate covers the common case. Set Options.Sampler to decide per
// request on something the rate cannot express, such as a header or a route.
type Sampler interface {
	Sample(r *http.Request) bool
}
