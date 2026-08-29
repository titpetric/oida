package tests_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/titpetric/oida"
)

// mountedTracer returns a tracer holding one recorded trace, built without
// touching the process default so the tests can run in parallel.
func mountedTracer(t *testing.T) *oida.Tracer {
	t.Helper()

	opts := oida.NewOptions("mount-test")
	opts.TrackMemoryUse = false
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }

	tracer, err := oida.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tracer.Observe(t.Context(), "cron: tick", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	return tracer
}

// checkDashboard drives the front end through a handler the tracer is mounted
// on and verifies the pages that exercise both path shapes.
func checkDashboard(t *testing.T, handler http.Handler) {
	t.Helper()

	for path, want := range map[string]string{
		"/debug/oida/":                "mount-test",
		"/debug/oida/traces":          "cron: tick",
		"/debug/oida/live?stream=off": "cron: tick",
		"/debug/oida/assets/oida.css": "--signal",
		"/debug/oida/stats":           "mount-test",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("GET %s returned %d", path, response.Code)
			continue
		}
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("GET %s does not contain %q", path, want)
		}
	}
}

func TestMount(t *testing.T) {
	tracer := mountedTracer(t)

	router := chi.NewRouter()
	router.Mount("/debug/oida", tracer)

	checkDashboard(t, router)
}

func TestMountServeMux(t *testing.T) {
	tracer := mountedTracer(t)

	mux := http.NewServeMux()
	mux.Handle("/debug/oida/", tracer)

	checkDashboard(t, mux)
}

func TestMountStripped(t *testing.T) {
	tracer := mountedTracer(t)

	// http.StripPrefix delivers the path relative to the mount, which is the
	// shape a router that rewrites paths hands the tracer.
	handler := http.StripPrefix("/debug/oida", tracer)

	checkDashboard(t, handler)
}

func TestMountNilTracer(t *testing.T) {
	var tracer *oida.Tracer

	response := httptest.NewRecorder()
	tracer.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/oida/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("nil tracer returned %d, want 404", response.Code)
	}
}
