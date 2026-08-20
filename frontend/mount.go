package frontend

import (
	"net/http"

	"github.com/titpetric/oida"
)

// Router is the subset of a router needed to mount the debug front end. It is
// satisfied by chi.Router and *chi.Mux, so oida does not depend on chi.
type Router interface {
	Mount(pattern string, h http.Handler)
}

// Mount registers the debug front end on r under Options.Path, wired to the
// tracer resolved from opts.
//
//	r := chi.NewRouter()
//	r.Use(oida.TracingMiddleware(opts))
//	if err := frontend.Mount(r, opts); err != nil {
//		return err
//	}
//
// It returns an error when r is nil, when opts do not validate, or when the
// tracer they name cannot be resolved.
func Mount(r Router, opts oida.Options) error {
	if r == nil {
		return oida.ErrNilRouter
	}

	opts = opts.WithDefaults()
	if err := opts.Validate(); err != nil {
		return err
	}
	tracer, err := oida.Resolve(opts)
	if err != nil {
		return err
	}
	opts.Tracer = tracer

	r.Mount(opts.Path, newHandler(opts, tracer))
	return nil
}

// MountServeMux registers the debug front end on a standard library mux. Both
// the bare path and its subtree are registered, because ServeMux treats them as
// different patterns.
func MountServeMux(mux *http.ServeMux, opts oida.Options) error {
	if mux == nil {
		return oida.ErrNilRouter
	}

	opts = opts.WithDefaults()
	if err := opts.Validate(); err != nil {
		return err
	}
	tracer, err := oida.Resolve(opts)
	if err != nil {
		return err
	}
	opts.Tracer = tracer

	handler := newHandler(opts, tracer)
	mux.Handle(opts.Path, handler)
	mux.Handle(opts.Path+"/", handler)
	return nil
}
