# Wiring examples

Each program wires oida into a service and serves the dashboard at
`/debug/oida`. They build against the oida in this repository through the
`replace` directive in `go.mod`, so a change to the package that breaks the
documented wiring breaks the build.

| Program                                    | Router                    |
|--------------------------------------------|---------------------------|
| [std/main_std.go](std/main_std.go)          | `*http.ServeMux`          |
| [chi/main_chi.go](chi/main_chi.go)          | `chi.Router`              |
| [gorilla/main_gorilla_mux.go](gorilla/main_gorilla_mux.go) | `*mux.Router` |

Run one, then open <http://localhost:8080/debug/oida>:

```bash
go -C testdata/examples run ./std
curl http://localhost:8080/users/1
```

`atkins examples` builds all three.
