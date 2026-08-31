# Getting started

## 1. Install

```bash
go get github.com/titpetric/oida@latest
```

## 2. chi/v5

The complete wiring is three calls: configure the tracer, add the middleware, mount the tracer. [testdata/examples/chi/main_chi.go](../testdata/examples/chi/main_chi.go) is the program, built by `atkins examples` against this checkout:

```go
r := chi.NewRouter()
r.Use(tracer.Middleware)
r.Get("/users/{id}", getUser)

if err := oida.Mount(r, tracer); err != nil {
	return err
}
```

Register the middleware before the routes it records: chi panics on a `Use` that follows a route. Put it after `middleware.Recoverer` if you want the recoverer to catch panics first; oida re-panics after recording, so either order records the failure.

The tracer is an `http.Handler` serving the UI, so `r.Mount(opts.Path, tracer)` is the alternative to `oida.Mount` where one pattern is enough.

### 2.1 Mounting on a separate admin listener

The UI does not have to sit on the public router. A separate listener keeps `/debug/oida` off the internet entirely:

```go
admin := chi.NewRouter()
admin.Mount("/debug/oida", tracer)
go http.ListenAndServe("127.0.0.1:9090", admin) //nolint:errcheck // admin listener
```

Both routers talk to the same tracer, so the public router records and the admin router displays.

### 2.2 Route patterns in statistics

`Options.RouteFunc` above is what makes statistics group `/users/1` and `/users/2` under `GET /users/{id}`: the middleware calls it *after* the handler ran, when chi's route context knows the matched pattern. Leave it out and every distinct URI becomes its own row.

With `net/http` you do not need it: oida reads `http.Request.Pattern`.

## 3. net/http

[testdata/examples/std/main_std.go](../testdata/examples/std/main_std.go) is the same service on `*http.ServeMux`:

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUser)

if err := oida.Mount(mux, tracer); err != nil {
	return err
}

return http.ListenAndServe(":8080", tracer.Middleware(mux))
```

The middleware wraps the mux rather than being registered on it, since `ServeMux` has no `Use`. The subtree pattern `/debug/oida/` covers the whole UI and `ServeMux` redirects the bare `/debug/oida` to it, so `mux.Handle("/debug/oida/", tracer)` is enough when the redirect does not bother you; `oida.Mount` registers both patterns and skips it.

### 3.1 gorilla/mux

`oida.Router` is the one method chi and `*http.ServeMux` share, `Handle(pattern string, h http.Handler)`. gorilla returns a `*mux.Route` from `Handle` for chaining, so `*mux.Router` does not satisfy it:

```
cannot use r (variable of type *mux.Router) as oida.Router value in argument to oida.Mount:
  *mux.Router does not implement oida.Router (wrong type for method Handle)
        have Handle(string, http.Handler) *mux.Route
        want Handle(string, http.Handler)
```

`oida.RouterFunc` adapts any registration call to the interface. Adapting `r.Handle` is not enough: gorilla matches those paths exactly, so `/debug/oida` would serve and `/debug/oida/traces` would 404. Register by prefix instead, which is how gorilla serves a subtree:

```go
r := mux.NewRouter()
r.Use(tracer.Middleware)
r.HandleFunc("/users/{id}", getUser).Methods(http.MethodGet)

mount := oida.RouterFunc(func(pattern string, h http.Handler) {
	r.PathPrefix(pattern).Handler(h)
})
if err := oida.Mount(mount, tracer); err != nil {
	return err
}
```

The middleware needs no adapter: `mux.MiddlewareFunc` is `func(http.Handler) http.Handler`, which is what `tracer.Middleware` is. [testdata/examples/gorilla/main_gorilla_mux.go](../testdata/examples/gorilla/main_gorilla_mux.go) is the program.

The same adapter fits any router with a chaining `Handle`, and any router that registers under a different method name.

## 4. Protecting the endpoint

`Options.Authorize` gates the whole UI. Requests that fail it get a 404, so the endpoint's existence is not advertised:

```go
opts.Authorize = func(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	return ok && subtle.ConstantTimeCompare([]byte(user), []byte(adminUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(adminPass)) == 1
}
```

Or restrict by network:

```go
opts.Authorize = func(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}
```

The check reads `r.RemoteAddr`, the connection peer, rather than the resolved client IP the traces record: a forwarded header is client input, so it cannot decide access.

## 5. Instrumenting the handler

```go
func getUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "getUser")
	defer span.End()

	id := chi.URLParam(r, "id")
	span.SetAttribute("user_id", id)

	user, err := loadUser(ctx, id)
	if err != nil {
		span.RecordError(err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	_, render := oida.Start(ctx, "render user", oida.KindTemplate)
	defer render.End()

	writeJSON(w, user)
}

func loadUser(ctx context.Context, id string) (*User, error) {
	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
	defer span.End()

	span.SetAttribute("table", "users")
	row := db.QueryRowContext(ctx, "SELECT id, email FROM users WHERE id = ?", id)

	var user User
	if err := row.Scan(&user.ID, &user.Email); err != nil {
		span.RecordError(err)
		return nil, err
	}
	return &user, nil
}
```

More patterns are in [guide-instrumentation.md](guide-instrumentation.md).

## 6. Background work

Work that does not arrive over HTTP still gets a trace:

```go
func (w *Worker) tick(ctx context.Context) error {
	return w.tracer.Observe(ctx, "worker.tick", func(ctx context.Context) error {
		ctx, span := oida.Start(ctx, "drain queue", oida.KindQueue)
		defer span.End()
		return w.drain(ctx)
	})
}
```

`Observe` creates the trace, runs the function, records the returned error and pushes the trace into the same ring buffer, so background work shows up in the same UI as requests.

## 7. In tests

`github.com/titpetric/oida/tests` hands you a chi router with the middleware, the front end and memory storage already wired, plus a few instrumented routes:

```go
func TestUsers(t *testing.T) {
	server := tests.NewHTTPServer(t)

	response, err := server.Client().Get(server.URL + "/users/42")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	...
}
```

`tests.NewServerWithTracer(t)` returns the tracer alongside the handler when you want to assert on the recorded traces themselves. Both fail the test on any `OnError` callback, so a broken storage backend shows up as a test failure rather than silence.

For your own service, build the tracer yourself and hold it where the work is, so packages can run in parallel:

```go
tracer, err := oida.New(oida.NewOptions("billing-api"))
```

## 8. Verifying the wiring

```bash
curl -s localhost:8080/users/1 -o /dev/null -D - | grep -i request-id
curl -s -H 'Accept: application/json' localhost:8080/debug/oida/traces | jq '.[0].name'
curl -s localhost:8080/debug/oida/stats            # plain text, curl user agent
```

If the log is empty, check in order: the middleware is registered, the sampler is not rejecting everything (`SampleRate`), the middleware and the mounted dashboard are the same tracer, and the request path is not in `IgnorePaths`.
