// Package frontend serves the debug front end of a tracer: the views, the view
// model behind them, and the HTTP handler that renders HTML for browsers, JSON
// for tools and plain text for terminals.
//
// Mount it on a router next to the middleware that records:
//
//	r := chi.NewRouter()
//	r.Use(oida.TracingMiddleware(opts))
//	if err := frontend.Mount(r, opts); err != nil {
//		return err
//	}
//
// Use Handler to build the handler without a router, MountServeMux with the
// standard library mux, and HandlerFor when a tracer is already at hand.
//
// The assets live in frontend/assets and are embedded, so the front end makes
// no external requests: no webfonts, no CDN, no analytics. Rendering failures
// are reported through Options.OnError like every other failure.
package frontend
