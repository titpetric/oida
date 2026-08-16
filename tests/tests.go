// Package tests provides a ready made oida-instrumented server for use in
// tests and examples: a chi router with the tracing middleware, the debug front
// end mounted, memory storage, and a handful of routes that record spans.
package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
)

// Path is the mount path of the debug front end on the test server.
const Path = "/debug/oida"

// NewServer returns a chi router with oida wired in: the tracing middleware,
// the debug front end mounted under Path, and sample routes that record spans.
// Recording and storage failures fail the test.
func NewServer(t testing.TB) http.Handler {
	t.Helper()

	router, _ := NewServerWithTracer(t)
	return router
}

// NewServerWithTracer returns the same router as NewServer together with the
// tracer recording it, so tests can assert on the recorded traces directly.
func NewServerWithTracer(t testing.TB) (http.Handler, *oida.Tracer) {
	t.Helper()

	opts := oida.NewOptions()
	opts.ServiceName = "oida-tests"
	opts.Path = Path
	opts.Storage = oida.NewStorageMemory(64)
	opts.RingBufferSize = 64
	opts.SampleRate = 1
	opts.RefreshInterval = 0
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }
	opts.RouteFunc = func(r *http.Request) string {
		if route := chi.RouteContext(r.Context()); route != nil {
			return route.RoutePattern()
		}
		return ""
	}

	tracer, err := oida.New(opts)
	if err != nil {
		t.Fatalf("oida.New: %v", err)
	}
	opts.Tracer = tracer

	router := chi.NewRouter()
	router.Use(oida.TracingMiddleware(opts))
	if err := frontend.Mount(router, opts); err != nil {
		t.Fatalf("frontend.Mount: %v", err)
	}

	router.Get("/", handleIndex)
	router.Get("/users/{id}", handleUser)
	router.Get("/slow", handleSlow)
	router.Get("/fail", handleFail)

	return router, tracer
}

// NewHTTPServer starts NewServer on a local listener and closes it when the
// test ends.
func NewHTTPServer(t testing.TB) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(NewServer(t))
	t.Cleanup(server.Close)
	return server
}

// handleIndex records one internal span.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "index", oida.KindInternal)
	defer span.End()

	_, render := oida.Start(ctx, "render index", oida.KindTemplate)
	defer render.End()

	_, _ = w.Write([]byte("oida"))
}

// handleUser records a nested cache and database lookup.
func handleUser(w http.ResponseWriter, r *http.Request) {
	ctx, span := oida.Start(r.Context(), "getUser")
	defer span.End()

	id := chi.URLParam(r, "id")
	span.SetAttribute("user_id", id)

	_, cache := oida.Start(ctx, "cache: user", oida.KindCache)
	cache.SetAttribute("hit", false)
	cache.End()

	err := oida.Do(ctx, "SELECT users", func(ctx context.Context) error {
		time.Sleep(time.Millisecond)
		return nil
	}, oida.KindDatabase)
	if err != nil {
		span.RecordError(err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
}

// handleSlow records an external call.
func handleSlow(w http.ResponseWriter, r *http.Request) {
	_, span := oida.Start(r.Context(), "GET upstream", oida.KindExternal)
	defer span.End()

	span.SetAttribute("url", "https://upstream.invalid/slow")
	time.Sleep(5 * time.Millisecond)
	_, _ = w.Write([]byte("slow"))
}

// handleFail records an error and responds with a failure.
func handleFail(w http.ResponseWriter, r *http.Request) {
	_, span := oida.Start(r.Context(), "fail", oida.KindInternal)
	defer span.End()

	span.RecordError(errors.New("deliberate failure"))
	http.Error(w, "boom", http.StatusInternalServerError)
}
