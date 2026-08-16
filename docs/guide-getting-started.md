# Getting started

## 1. Install

```bash
go get github.com/titpetric/oida
```

oida depends only on `github.com/a-h/templ` (the templ runtime). It does not
depend on chi; the router integration is a structural interface.

## 2. chi/v5

The complete wiring is three calls: configure the tracer, add the middleware,
mount the UI.

```go
package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/titpetric/oida"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	opts := oida.NewOptions()
	opts.ServiceName = "billing-api"
	opts.Path = "/debug/oida"
	opts.RingBufferSize = 500
	opts.SampleRate = 1
	opts.RouteFunc = func(r *http.Request) string {
		if route := chi.RouteContext(r.Context()); route != nil {
			return route.RoutePattern()
		}
		return ""
	}

	tracer, err := oida.Configure(opts)
	if err != nil {
		return err
	}
	opts.Tracer = tracer

	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(oida.TracingMiddleware(opts))

	if err := oida.Mount(r, opts); err != nil {
		return err
	}

	r.Get("/users/{id}", getUser)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
```

Order matters:

- `oida.TracingMiddleware` goes **after** `middleware.RealIP` (so the remote
  address is the real one) and **after** `middleware.Recoverer` is registered if
  you want the recoverer to catch panics first — oida re-panics after recording,
  so either order records the failure; putting oida inside the recoverer keeps
  the 500 response behaviour of chi.
- `oida.Mount` must be called on a router that has the middleware registered, or
  on any router in the same process — the tracer is shared, not the router.
- Routes registered *before* `r.Use` panic in chi; register middleware first.

Visit `http://localhost:8080/debug/oida`.

### 2.1 Mounting on a separate admin listener

The UI does not have to sit on the public router. A separate listener keeps
`/debug/oida` off the internet entirely:

```go
admin := chi.NewRouter()
if err := oida.Mount(admin, opts); err != nil {
	return err
}
go http.ListenAndServe("127.0.0.1:9090", admin) //nolint:errcheck // admin listener
```

Both routers talk to the same `opts.Tracer`, so the public router records and
the admin router displays.

### 2.2 Route patterns in statistics

`Options.RouteFunc` above is what makes statistics group `/users/1` and
`/users/2` under `GET /users/{id}`: the middleware calls it *after* the handler
ran, when chi's route context knows the matched pattern. Leave it out and every
distinct URI becomes its own row.

With `net/http` you do not need it — oida reads `http.Request.Pattern`.

## 3. net/http

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /users/{id}", getUser)

opts := oida.NewOptions()
opts.ServiceName = "billing-api"

tracer, err := oida.Configure(opts)
if err != nil {
	return err
}
opts.Tracer = tracer

if err := oida.MountServeMux(mux, opts); err != nil {
	return err
}

handler := oida.TracingMiddleware(opts)(mux)
return http.ListenAndServe(":8080", handler)
```

`MountServeMux` registers both `/debug/oida` and `/debug/oida/`, because
`ServeMux` treats those as different patterns.

## 4. Protecting the endpoint

`Options.Authorize` gates the whole UI. Requests that fail it get a 404, so the
endpoint's existence is not advertised:

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
	return oida.Default().Observe(ctx, "worker.tick", func(ctx context.Context) error {
		ctx, span := oida.Start(ctx, "drain queue", oida.KindQueue)
		defer span.End()
		return w.drain(ctx)
	})
}
```

`Observe` creates the trace, runs the function, records the returned error and
pushes the trace into the same ring buffer, so background work shows up in the
same UI as requests.

## 7. In tests

`github.com/titpetric/oida/tests` hands you a chi router with the middleware,
the front end and memory storage already wired, plus a few instrumented routes:

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

`tests.NewServerWithTracer(t)` returns the tracer alongside the handler when you
want to assert on the recorded traces themselves. Both fail the test on any
`OnError` callback, so a broken storage backend shows up as a test failure
rather than silence.

For your own service, build an explicit tracer instead of `Default()` so
packages can run in parallel:

```go
tracer, err := oida.New(oida.NewOptions())
```

## 8. Verifying the wiring

```bash
curl -s localhost:8080/users/1 -o /dev/null -D - | grep -i request-id
curl -s -H 'Accept: application/json' localhost:8080/debug/oida | jq '.log[0].name'
curl -s localhost:8080/debug/oida/stats            # plain text, curl user agent
```

If the log is empty, check in order: the middleware is registered, the sampler
is not rejecting everything (`SampleRate`), the tracer is shared between the
middleware and the handler (`opts.Tracer`), and the request path is not in
`IgnorePaths`.
