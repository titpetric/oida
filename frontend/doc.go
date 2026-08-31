// Package frontend serves the debug front end of a tracer: the views, the view
// model behind them, and the HTTP handler that renders HTML for browsers, JSON
// for tools and plain text for terminals.
//
// HandlerFor builds the handler of a tracer:
//
//	handler := frontend.HandlerFor(tracer)
//
// The tracer is an http.Handler serving this front end itself, and the root
// package mounts it, so a service needs no import of this package.
//
// The package reads the recorded data through the model package alone. The
// root package imports it to serve the dashboard from the tracer.
//
// The assets live in frontend/assets and are embedded, so the front end makes
// no external requests: no webfonts, no CDN, no analytics. Rendering failures
// are reported through Options.OnError like every other failure.
package frontend
