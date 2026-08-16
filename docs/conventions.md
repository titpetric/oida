# Conventions

## 1. One struct per file

Every file declares at most one struct type, named after the file:

| File | Struct |
| --- | --- |
| `tracer.go` | `Tracer` |
| `span.go` | `Span` |
| `trace.go` | `Trace` |
| `options.go` | `Options` |
| `storage_memory.go` | `StorageMemory` |
| `storage_disk.go` | `StorageDisk` |
| `ring.go` | `ring` |
| `sampler.go` | `rateSampler` |
| `handler.go` | `handler` |
| `responsewriter.go` | `responseWriter` |
| `page.go` | `Page` |

Exceptions, deliberate and limited:

- **`model.go`** holds the data models: enums (`Kind`, `State`, `View`) and the
  pure structs that carry no behaviour beyond accessors (`HTTPInfo`,
  `MemoryUse`, `Memory`, `PoolEstimate`, `StateDuration`, `Segment`, `SpanRow`,
  `Statistic`, `Stats`, `Snapshot`). They are one cohesive vocabulary and are
  read together; splitting them across a dozen files hides the model rather than
  clarifying it.
- **Function-only files** (`context.go`, `id.go`, `errors.go`, `stats.go`,
  `timeline.go`, `format.go`, `text.go`, `assets.go`, `default.go`,
  `middleware.go`, `mount.go`) declare no struct at all.
- **Interface files** (`recorder.go`, `storage.go`, the `Sampler` interface in
  `sampler.go`, `Router` in `mount.go`) declare the interface next to its
  primary implementation or its only consumer.

`Span` and `Trace` are data models *with* behaviour, so they live in their own
files rather than in `model.go`. Methods always sit in the same file as their
receiver.

## 2. Naming

- Exported API reads as a sentence at the call site: `oida.Start`,
  `oida.Mount`, `oida.TracingMiddleware`, `span.RecordError`.
- Constructors: `New` for the primary type, `NewX` for secondary types
  (`NewOptions`, `NewRateSampler`). No `Must*` variants in the public API.
- Options fields are nouns, not verbs; behaviour toggles read as adjectives
  (`Enabled`, `TrackMemoryUse`, `TrustRequestID`).
- Unexported helpers use the shortest unambiguous name (`ring`, `handler`,
  `page`), because the package name already qualifies them.

## 3. Errors

- Sentinels live in `errors.go` and are wrapped with `%w`, so
  `errors.Is(err, oida.ErrInvalidOptions)` works for every configuration
  failure.
- Constructors and mounting return errors. Instrumentation never does, and never
  panics: `Start`, `Span` and `Trace` methods degrade to no-ops.
- Nothing in this package writes to stdout, stderr, or a logger. Failures are
  returned; recorded errors live on spans. A library that prints is a library
  you cannot deploy.

## 4. Nil safety

Every exported method on `*Span` and `*Trace` starts with a nil-receiver guard.
This is what allows instrumentation to be written once and run in processes
where oida is disabled, unsampled, or absent from the context.

## 5. Time

All time comes from `Options.Clock` (defaulting to `time.Now`), threaded through
the tracer. No package-level `time.Now()` calls outside the default clock, so
tests can drive a deterministic clock and assert exact durations.

## 6. Templates

- Components live in `view_*.templ`, generate to `view_*_templ.go`, and both are
  committed.
- Components take exactly one argument, the `Page` view model. They never touch
  a tracer, a request, or global state.
- Anything a component needs computed goes into `Page` or a `format.go` helper —
  never into template logic beyond `if`, `for` and a function call.

## 7. Tests

- Table-driven, `testdata/` for golden output, `t.Parallel()` where the tracer
  is per-test.
- Tests construct explicit tracers via `New`; they never touch `Default()`, so
  packages can run in parallel.
- HTTP tests use `httptest.NewServer` plus the real middleware, not a hand-built
  trace, so the wiring is covered.
- Rendering tests assert on the rendered HTML of a hand-built `Page`, which is
  possible because components have no hidden dependencies.
- Race coverage is mandatory: `go test -race ./...` exercises concurrent
  `Start`/`End`/`Snapshot`.

## 8. Dependencies

The `oida` package itself imports `github.com/a-h/templ` and the standard
library. Nothing else. Router integration is a structural interface, so chi,
gorilla and echo all work without an import.

`github.com/go-chi/chi/v5` appears in `go.mod` because two satellite packages
use it: `tests` (the ready made test server) and `cmd/oida` (the demo service).
Importing `oida` does not pull chi into a build that does not ask for it.

## 9. Pipeline

`atkins` runs the whole pipeline: `go mod tidy`, `templ generate`, formatting,
`go:test`, `go:build` and `docker:build`. The image name comes from the folder
name, so `docker/Dockerfile` builds `titpetric/oida:latest` over
`bin/oida-linux-amd64`, and `compose.yml` runs it behind the ingress network.

Rules for the pipeline:

- Generated templ output is committed; `atkins templ:verify` fails on drift.
- Tests run with `-count 1 -cover`; nothing in the suite depends on the process
  wide default tracer, so packages stay parallel-safe.
- The pipeline must stay green without network access to anything but the Go
  module proxy and the Docker daemon.
