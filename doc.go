// Package oida records in-process telemetry: traces and spans held in a ring
// buffer inside the process, with a server side rendered front end mounted at
// /debug/oida.
//
// Wire it into a service in three calls:
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
//	r := chi.NewRouter()
//	r.Use(oida.TracingMiddleware(opts))
//	if err := oida.Mount(r, opts); err != nil {
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
// Nothing in this package writes to stdout or stderr. Storage and rendering
// failures are reported through Options.OnError.
package oida
