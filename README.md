# oida - in process telemetry for Go services

![The oida masthead: service identity, process facts, and the instrument row](docs/assets/header.png)

Oida is a Go importable package that implements in-process telemetry and provides a rich interface for observation. Its goal is to be an observability platform for small and big projects that benefit from local observability. The project started as an extension of [titpetric/phpscript](https://github.com/titpetric/phpscript) which got separated out to handle more concerns than just server status. The name of the project comes from my Austrian-roots friend Martin. The word itself is likely to mean various things depending on context; whole conversations can be had just by repeating the words in different tone.

[Docs](docs/README.md) | [Install](#install) | [Import](#import) | [Use with stdlib](#use-with-stdlib) | [Use with go-chi](#use-with-go-chi) | [Integration](#integration) | [Features](#features)

## Install

```bash
go get github.com/titpetric/oida@latest
```

## Import

```go
import "github.com/titpetric/oida"
```

## Use with stdlib

[testdata/examples/std/main_std.go](testdata/examples/std/main_std.go) is a service on `*http.ServeMux`: `oida.New` builds the tracer, `oida.Mount` registers the dashboard, and `tracer.Middleware` wraps the mux, so every sampled request through it is recorded. `Options.Path` is added to `IgnorePaths`, so browsing the dashboard records nothing of its own.

## Use with go-chi

[testdata/examples/chi/main_chi.go](testdata/examples/chi/main_chi.go) is the same service on a `chi.Router`, which takes the middleware through `r.Use(tracer.Middleware)`. Register it before the routes it records: chi panics on a `Use` that follows a route.

`oida.Mount` serves the dashboard under the path the tracer was configured with, and takes a `chi.Router` or an `*http.ServeMux` alike. A router whose `Handle` returns a value, such as gorilla's, mounts through `oida.RouterFunc`; [testdata/examples/gorilla/main_gorilla_mux.go](testdata/examples/gorilla/main_gorilla_mux.go) is that wiring, and the [getting started guide](docs/guide-getting-started.md) explains it. The tracer is an `http.Handler` of its own, so `mux.Handle("/debug/oida/", tracer)` works where one pattern is enough.

Recording is opt-in: set `opts.Enabled` in code, or leave it alone and set `OIDA_ENABLED=true` in the environment. Open `http://localhost:8080/debug/oida`.

## Integration

`getUser` is the handler each example registers, and `listUsers` under it records the span:

```go
_, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
defer span.End()

span.SetAttribute("limit", 100)
```

Pass the context down, and the next `oida.Start` records a child span under this one. The kind drives the colour in the timeline and the grouping of the segment sweep; the [span kinds](docs/spec-model.md#span-kinds) and the [attribute keys](docs/spec-model.md#attributes) the dashboard reads are documented with the rest of the data model. Every call is nil-safe, so instrumented code runs unchanged where no tracer was built, or where the request was not sampled.

## Features

- HTTP requests and background jobs
- Nested spans with kinds, attributes, errors, and source locations
- Bounded in-memory retention, or disk documents that outlive the process, chosen in code or from the environment
- Rate sampling and custom sampling rules
- Live activity, retained traces, route statistics, and trace details
- HTML for browsers, JSON for tools, and plain text for terminals
- Nil-safe instrumentation when tracing is absent or a request is not sampled

`github.com/titpetric/oida` is the public API, and [docs/api.md](docs/api.md) is its generated reference. The `frontend`, `model` and `storage` packages serve the root package and carry no compatibility promise of their own. [docs/](docs/README.md) covers getting started, instrumentation, configuration, the data model and the dashboard, and [testdata/examples/](testdata/examples/README.md) holds a runnable program per router.

## License

MIT

```
```
