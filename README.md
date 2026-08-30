# oida

`oida` records traces and spans inside a Go service and provides a dashboard at `/debug/oida`. Use it to inspect slow requests, failed operations, nested work, memory use, and background jobs without running a separate telemetry service.

Telemetry for Go.

## Install

```bash
go get github.com/titpetric/oida@latest
```

## Use

```go
import "github.com/titpetric/oida"

tracer, err := oida.New(oida.NewOptions("billing-api"))
if err != nil {
	return err
}

r := chi.NewRouter()
r.Use(tracer.Middleware)
r.Mount("/debug/oida", tracer)
```

The tracer is an `http.Handler` serving the dashboard, so it mounts the same way on the standard library mux:

```go
mux := http.NewServeMux()
mux.Handle("/debug/oida/", tracer)
```

`oida.Mount(r, tracer)` does the same on either router, registering the subtree patterns each one understands.

Instrument work below the middleware:

```go
ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
defer span.End()
span.SetAttribute("limit", limit)
```

Open `http://localhost:8080/debug/oida`.

## What it covers

- HTTP requests and background jobs
- Nested spans with kinds, attributes, errors, and source locations
- Bounded in-memory retention, or disk documents that outlive the process, chosen in code or from the environment
- Rate sampling and custom sampling rules
- Live activity, retained traces, route statistics, and trace details
- HTML for browsers, JSON for tools, and plain text for terminals
- Nil-safe instrumentation when tracing is absent or a request is not sampled

## Documentation

`github.com/titpetric/oida` is the public API: one import covers instrumenting, mounting and configuring. [docs/api.md](docs/api.md) is its generated reference. The `frontend`, `model` and `storage` packages serve the root package and carry no compatibility promise of their own.

| Guide                                            | Contents                                                    |
|--------------------------------------------------|-------------------------------------------------------------|
| [Getting started](docs/guide-getting-started.md) | `chi/v5`, `net/http`, endpoint protection, and verification |
| [Instrumentation](docs/guide-instrumentation.md) | Spans, errors, SQL, HTTP, cache, and concurrent work        |
| [Configuration](docs/guide-configuration.md)     | Options, sampling, retention, sizing, and YAML              |
| [Public API](docs/spec-api.md)                   | Supported functions, methods, storage, and errors           |
| [Data returned by oida](docs/spec-model.md)      | Trace, span, snapshot, and statistics fields                |
| [Using the dashboard](docs/spec-frontend.md)     | Views, filters, JSON, plain text, and access control        |
| [Screenshots](docs/screenshots.md)               | Dashboard views                                             |

## License

MIT
