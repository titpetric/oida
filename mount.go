package oida

import (
	"net/http"

	"github.com/titpetric/oida/frontend"
)

// Router is the one method chi and the standard library share: chi.Router,
// *chi.Mux and *http.ServeMux all register handlers with Handle, so one
// interface mounts the front end on either, and oida depends on neither.
type Router interface {
	Handle(pattern string, h http.Handler)
}

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

// Mount registers the debug front end of t on r, under the path t was
// configured with. Mounting the tracer itself, r.Handle(path, tracer), is
// equivalent; this call adds the patterns each router uses to serve a subtree.
//
// Three patterns are registered: the bare path, the trailing slash form that
// is the subtree on a ServeMux, and the /* wildcard that is the subtree on
// chi. Each router uses the ones it understands.
//
// It returns an error when r or t is nil.
func Mount(r Router, t *Tracer) error {
	if r == nil {
		return ErrNilRouter
	}
	if t == nil {
		return ErrNoTracer
	}

	path := t.Options().Path
	h := frontend.HandlerFor(t)
	r.Handle(path, h)
	r.Handle(path+"/", h)
	r.Handle(path+"/*", h)
	return nil
}
