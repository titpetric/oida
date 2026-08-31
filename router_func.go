package oida

import "net/http"

// RouterFunc adapts a registration function to Router, for a router whose own
// Handle does not fit:
//
//	oida.Mount(oida.RouterFunc(func(pattern string, h http.Handler) {
//		r.PathPrefix(pattern).Handler(h)
//	}), tracer)
//
// gorilla returns a *mux.Route from Handle and matches its paths exactly, so
// its dashboard is registered by prefix.
type RouterFunc func(pattern string, h http.Handler)

// Handle implements Router.
func (f RouterFunc) Handle(pattern string, h http.Handler) {
	f(pattern, h)
}
