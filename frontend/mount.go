package frontend

import (
	"net/http"

	"github.com/titpetric/oida/model"
)

// Router is the subset of a router needed to mount the debug front end. It is
// satisfied by chi.Router and *chi.Mux, so oida does not depend on chi.
type Router interface {
	Mount(pattern string, h http.Handler)
}

// Mount registers the debug front end on r under Options.Path, wired to the
// tracer in Options.Tracer. Mounting the tracer itself, r.Mount(opts.Path,
// tracer), is equivalent.
//
//	r := chi.NewRouter()
//	r.Use(oida.TracingMiddleware(opts))
//	if err := frontend.Mount(r, opts); err != nil {
//		return err
//	}
//
// It returns an error when r is nil or when opts do not validate.
func Mount(r Router, opts model.Options) error {
	if r == nil {
		return model.ErrNilRouter
	}

	opts = opts.WithDefaults()
	if err := opts.Validate(); err != nil {
		return err
	}

	r.Mount(opts.Path, newHandler(opts, opts.Tracer))
	return nil
}

// MountServeMux registers the debug front end on a standard library mux. Both
// the bare path and its subtree are registered, because ServeMux treats them as
// different patterns.
func MountServeMux(mux *http.ServeMux, opts model.Options) error {
	if mux == nil {
		return model.ErrNilRouter
	}

	opts = opts.WithDefaults()
	if err := opts.Validate(); err != nil {
		return err
	}

	handler := newHandler(opts, opts.Tracer)
	mux.Handle(opts.Path, handler)
	mux.Handle(opts.Path+"/", handler)
	return nil
}
