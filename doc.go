// Package oida records in-process telemetry: traces and spans held in a ring
// buffer inside the process, with a server side rendered front end mounted at
// /debug/oida.
//
// Wire it into a service in three calls: configure the tracer, add the
// middleware, mount the dashboard from github.com/titpetric/oida/frontend.
// With the standard library ServeMux:
//
//	opts := oida.NewOptions()
//	opts.ServiceName = "billing-api"
//
//	tracer, err := oida.Configure(opts)
//	if err != nil {
//		return err
//	}
//	opts.Tracer = tracer
//
//	mux := http.NewServeMux()
//	if err := frontend.MountServeMux(mux, opts); err != nil {
//		return err
//	}
//	handler := oida.TracingMiddleware(opts)(mux)
//	return http.ListenAndServe(":8080", handler)
//
// With a chi router the middleware registers like any other, and the same
// options carry over:
//
//	r := chi.NewRouter()
//	r.Use(oida.TracingMiddleware(opts))
//	if err := frontend.Mount(r, opts); err != nil {
//		return err
//	}
//
// Instrument anything below the middleware:
//
//	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
//	defer span.End()
//	span.SetAttribute("limit", limit)
//
// Every instrumentation call is nil safe, so instrumented code runs unchanged
// in processes where oida is disabled, where the request was not sampled, or
// where no trace is in the context.
//
// The project is three packages. This one records: the tracer, the middleware,
// the options and the storage. Package model holds the recorded data and
// depends on nothing; the types it defines are aliased here, so instrumenting a
// service needs this import alone. Package frontend serves the dashboard and is
// the only one that renders.
//
// Nothing in this package writes to stdout or stderr. Storage and rendering
// failures are reported through Options.OnError.
package oida
