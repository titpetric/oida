# oida

`oida` records traces and spans inside a Go service and provides a dashboard at `/debug/oida`. Use it to inspect slow requests, failed operations, nested work, memory use, and background jobs without running a separate telemetry service.

Telemetry for Go.

## Install

```bash
go get github.com/titpetric/oida@latest
```

## Use

```go
import (
	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
)

opts := oida.NewOptions()
opts.ServiceName = "billing-api"

tracer, err := oida.Configure(opts)
if err != nil {
	return err
}
opts.Tracer = tracer

r := chi.NewRouter()
r.Use(oida.TracingMiddleware(opts))

if err := frontend.Mount(r, opts); err != nil {
	return err
}
```

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
- Bounded in-memory retention or disk retention across restarts
- Rate sampling and custom sampling rules
- Live activity, retained traces, route statistics, and trace details
- HTML for browsers, JSON for tools, and plain text for terminals
- Nil-safe instrumentation when tracing is absent or a request is not sampled

## Documentation

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
