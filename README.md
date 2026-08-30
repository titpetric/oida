# oida - in process telemetry for Go services

![The oida masthead: service identity, process facts, and the instrument row](docs/assets/header.png)

Oida is a Go importable package that implements in-process telemetry and provides a rich interface for observation. Its goal is to be an observability platform for small and big projects that benefit from local observability. The project started as an extension of [titpetric/phpscript](https://github.com/titpetric/phpscript) which got separated out to handle more concerns than just server status. The name of the project comes from my Austrian-roots friend Martin. The word itself is likely to mean various things depending on context; whole conversations can be had just by repeating the words in different tone.

[Docs](docs/README.md) | [Install](#install) | [Import](#import) | [Use with stdlib](#use-with-stdlib) | [Use with go-chi](#use-with-go-chi) | [API](docs/api.md) | [Features](#features)

## Install

```bash
go get github.com/titpetric/oida@latest
```

## Import

```go
import "github.com/titpetric/oida"
```

One import covers instrumenting, mounting and configuring. The middleware records the request; spans record what the request did:

```go
func GetUsers(ctx context.Context) (UserList, error) {
	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
	defer span.End()

	span.SetAttribute("limit", 100)

	// implementation...
}
```

Pass the returned `ctx` down, and the next `oida.Start` records a child span. The kind drives the colour in the timeline and the grouping of the segment sweep; the [span kinds](docs/spec-model.md#span-kinds) and the [attribute keys](docs/spec-model.md#attributes) the dashboard reads are documented with the rest of the data model. Every call is nil-safe, so instrumented code runs unchanged where no tracer was built, or where the request was not sampled.

## Use with stdlib

```go
opts := oida.NewOptions("billing-api")
opts.Enabled = true

tracer, err := oida.New(opts)
if err != nil {
	return err
}

mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUser)

if err := oida.Mount(mux, tracer); err != nil {
	return err
}

return http.ListenAndServe(":8080", tracer.Middleware(mux))
```

## Use with go-chi

```go
opts := oida.NewOptions("billing-api")
opts.Enabled = true

tracer, err := oida.New(opts)
if err != nil {
	return err
}

r := chi.NewRouter()
r.Use(tracer.Middleware)
r.Get("/users/{id}", getUser)

if err := oida.Mount(r, tracer); err != nil {
	return err
}

return http.ListenAndServe(":8080", r)
```

`oida.Mount` serves the dashboard under the path the tracer was configured with, and takes a `chi.Router` or an `*http.ServeMux` alike. The tracer is an `http.Handler` of its own, so `mux.Handle("/debug/oida/", tracer)` works where one pattern is enough.

Recording is opt-in: set `opts.Enabled` in code, or leave it alone and set `OIDA_ENABLED=true` in the environment. Open `http://localhost:8080/debug/oida`.

## Features

- HTTP requests and background jobs
- Nested spans with kinds, attributes, errors, and source locations
- Bounded in-memory retention, or disk documents that outlive the process, chosen in code or from the environment
- Rate sampling and custom sampling rules
- Live activity, retained traces, route statistics, and trace details
- HTML for browsers, JSON for tools, and plain text for terminals
- Nil-safe instrumentation when tracing is absent or a request is not sampled

`github.com/titpetric/oida` is the public API, and [docs/api.md](docs/api.md) is its generated reference. The `frontend`, `model` and `storage` packages serve the root package and carry no compatibility promise of their own. [docs/](docs/README.md) covers getting started, instrumentation, configuration, the data model and the dashboard.

## License

MIT
