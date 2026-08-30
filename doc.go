// Package oida records in-process telemetry: traces and spans held in a ring
// buffer inside the process, with a server side rendered front end mounted at
// /debug/oida.
//
// Wire it into a service in three calls: configure the tracer, mount it, add
// the middleware. Recording is opt-in: enable it in code, or leave the field
// alone and set OIDA_ENABLED=true in the environment. The tracer is an
// http.Handler serving the debug front end, so it mounts like any other
// handler and no second import is needed:
//
//	opts := oida.NewOptions("billing-api")
//	opts.Enabled = true
//
//	tracer, err := oida.New(opts)
//	if err != nil {
//		return err
//	}
//
//	mux := http.NewServeMux()
//	mux.Handle("/debug/oida/", tracer)
//
// A chi router mounts the same tracer with its own call:
//
//	r := chi.NewRouter()
//	r.Mount("/debug/oida", tracer)
//
// Mount registers the front end on either router, adding the subtree patterns
// each one understands:
//
//	if err := oida.Mount(mux, tracer); err != nil {
//		return err
//	}
//
// The middleware records every sampled request into the tracer:
//
//	handler := tracer.Middleware(mux)
//	return http.ListenAndServe(":8080", handler)
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
// The project is four packages. This one records and serves: the tracer, the
// middleware and the options. Package model holds the recorded data and the
// configuration and depends on nothing; the types it defines are aliased here,
// so instrumenting a service needs this import alone. Package storage holds
// the retention drivers, which New builds from the environment.
// Package frontend renders the dashboard, reads the model alone, and is
// imported here so the tracer can serve it.
//
// Nothing in this package writes to stdout or stderr. Storage and rendering
// failures are reported through Options.OnError.
package oida
