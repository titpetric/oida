# internal

A composition space for the oida package. Go closes `internal/` to importers
outside this module, so nothing here is API and nothing here needs a
compatibility promise. The root package uses it to stay what a reader of the
public API needs it to be: the tracer, the middleware, the mount and the
instrumentation, rather than those plus the utilities and the component
implementations they are built from.

## What belongs here

A symbol moves when it stands on its own: a utility, or a component whose whole
implementation is private.

- Free functions and unexported types are the usual case. They are already
  unreachable from outside the module, so moving them changes no API.
- A structure another package's type holds moves too, when the structure is
  generic rather than the thing that package exists to be. A ring buffer is a
  ring buffer wherever it lives.
- A helper taking `model.Options` moves too. `oida.Options` is an alias of it,
  so the call site reads the same.
- A component whose whole implementation is private moves with its tests:
  reading options out of the environment, the rate sampler, the response
  writer that records a status.

## What does not

- Anything taking or returning `*Tracer`, which would import the root package
  and close the cycle. That is what keeps `serveTraced` and the methods on
  `Tracer` where they are.
- The implementation surface of a package. The front end's route handlers are
  what `frontend` is; moving them would move the package.

## Layout

`internal` is one package unless a subpackage earns its own name, the way
`internal/ring` does: a structure with a vocabulary of its own reads better as
`ring.New` than as `internal.NewRing`, and a subpackage is what keeps the
imports one way. The file is named after what it holds, `remote_addr.go` for
`RemoteAddr`, so `gofsck` grouping passes and a reader finds a symbol by its
filename.

Symbols here are exported for the rest of the module to call, and that export
means nothing outside it. A standalone structure moves here even when a
package's own type refers to it, which is how `storage.memoryStorage` came to
hold a `*ring.Ring`: the ring is a buffer, not the retention driver.

## Imports

`internal` imports `model` and `storage`, and `internal/ring` imports `model`
alone. Nothing here imports `oida`, and nothing `storage` imports may import
`internal` itself, which is why the ring has a package of its own.
