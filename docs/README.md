# oida documentation

`oida` is an in-process telemetry package for Go services: it records traces and
spans into a ring buffer that lives in the same process, and serves a
server-side rendered UI under `/debug/oida` so you can inspect what a request
actually did without shipping data to an external collector.

The package is extracted from an in-process status page and its request/span
telemetry model. The analysis of that prior art, and what carried over, is in
[architecture.md](architecture.md).

## Guides

| Document | Contents |
| --- | --- |
| [guide-getting-started.md](guide-getting-started.md) | Add oida to a Go service, `net/http` and `chi/v5` samples |
| [guide-instrumentation.md](guide-instrumentation.md) | Creating and closing spans, attributes, errors, common patterns |
| [guide-configuration.md](guide-configuration.md) | `oida.Options` reference, sampling, defaults, validation |

## Specifications

| Document | Contents |
| --- | --- |
| [architecture.md](architecture.md) | Prior art analysis, package layout, concurrency model |
| [spec-model.md](spec-model.md) | Data models: `Trace`, `Span`, `Kind`, statistics, snapshots |
| [spec-api.md](spec-api.md) | Public API surface, function signatures, error contract |
| [spec-frontend.md](spec-frontend.md) | `/debug/oida` routes, templ components, content negotiation |
| [conventions.md](conventions.md) | File layout rules, naming, testing, pipeline |

## Repository

| Path | Contents |
| --- | --- |
| `*.go`, `*.templ` | the package |
| `cmd/oida/` | runnable demo service, and the Docker image payload |
| `tests/` | `tests.NewServer(t)`: chi + oida wired for your tests |
| `atkins.yml`, `compose.yml`, `docker/` | pipeline, image and demo deployment |

Run the whole pipeline — tidy, generate, format, test, build, image — with
`atkins`.

## Design goals

1. **In-process.** No agent, no exporter, no network egress. The ring buffer is
   the storage; when it wraps, old traces are gone.
2. **Zero-cost when off.** A nil tracer, a disabled tracer, or an unsampled
   request must not allocate span structures.
3. **Import and go.** `oida.Start(ctx, name)` is safe to call from any package
   at any depth, with or without an active trace in the context.
4. **Server-side rendered.** The UI is templ components rendered on the server.
   No JavaScript build step, no client-side framework, no CDN.
5. **Strict scopes.** One struct per file, data models in `model.go`. See
   [conventions.md](conventions.md).
