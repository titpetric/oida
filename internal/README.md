# internal

A composition space for the oida package. Go closes `internal/` to importers
outside this module, so nothing here is API and nothing here needs a
compatibility promise. The root package uses it to stay what a reader of the
public API needs it to be: the tracer, the middleware, the mount and the
instrumentation, rather than those plus the utilities and the component
implementations they are built from.

## What belongs here

A symbol moves when nothing in oida refers to it from a type definition.

- Free functions and unexported types are the usual case. They are already
  unreachable from outside the module, so moving them changes no API.
- A helper taking `model.Options` moves too. `oida.Options` is an alias of it,
  so the call site reads the same.
- A component whose whole implementation is private moves with its tests:
  reading options out of the environment, the rate sampler, the response
  writer that records a status.

## What does not

- Anything an exported type declares. `Tracer.events *broker` pins `broker` to
  the root package: moving it would change the type definition.
- Anything taking or returning `*Tracer`, which would import the root package
  and close the cycle.

## Layout

`internal` is one package unless a subpackage earns its own name. The file is
named after what it holds, `remote_addr.go` for `RemoteAddr`, so `gofsck`
grouping passes and a reader finds a symbol by its filename.

Symbols here are exported for the root package to call, and that export means
nothing outside this module.

## Imports

`internal` imports `model` and `storage`. It never imports `oida`, and the
dependency stays one way: `oida` to `internal`, `internal` to `model`.
