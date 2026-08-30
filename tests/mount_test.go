package tests_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/oida"
)

// mountedTracer returns a tracer holding one recorded trace, built without
// touching the process default so the tests can run in parallel.
func mountedTracer(t *testing.T) *oida.Tracer {
	t.Helper()
	return mountedTracerAt(t, oida.DefaultPath)
}

// mountedTracerAt returns the same tracer configured for another mount path.
func mountedTracerAt(t *testing.T, path string) *oida.Tracer {
	t.Helper()

	opts := oida.NewOptions("mount-test")
	opts.Enabled = true
	opts.Path = path
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
	checkDashboardAt(t, handler, oida.DefaultPath)
}

// checkDashboardAt verifies the dashboard under another mount path, including
// that the pages link their assets under it, so a misconfigured base surfaces.
func checkDashboardAt(t *testing.T, handler http.Handler, base string) {
	t.Helper()

	for path, want := range map[string]string{
		base + "/":                `href="` + base + `/assets/oida.css"`,
		base + "/traces":          "cron: tick",
		base + "/live?stream=off": "cron: tick",
		base + "/assets/oida.css": "--signal",
		base + "/stats":           "mount-test",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("GET %s returned %d", path, response.Code)
			continue
		}
		if response.Body.Len() == 0 {
			t.Errorf("GET %s returned an empty body", path)
			continue
		}
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("GET %s does not contain %q", path, want)
		}
	}
}

func TestMountServeMux(t *testing.T) {
	tracer := mountedTracer(t)

	mux := http.NewServeMux()
	mux.Handle("/debug/oida/", tracer)

	checkDashboard(t, mux)
}

func TestMountServeMuxStatusPage(t *testing.T) {
	tracer := mountedTracerAt(t, "/status-page")

	mux := http.NewServeMux()
	mux.Handle("/status-page/", tracer)

	checkDashboardAt(t, mux, "/status-page")
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

func TestMountRouter(t *testing.T) {
	router := &stubRouter{}
	if err := oida.Mount(router, mountedTracer(t)); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	// The three patterns cover both router families: the bare path, the
	// trailing slash subtree of a ServeMux, and the /* subtree of chi.
	want := []string{oida.DefaultPath, oida.DefaultPath + "/", oida.DefaultPath + "/*"}
	if len(router.patterns) != len(want) {
		t.Fatalf("mounted on %v, want %v", router.patterns, want)
	}
	for i, pattern := range want {
		if router.patterns[i] != pattern {
			t.Fatalf("mounted on %v, want %v", router.patterns, want)
		}
	}

	checkDashboard(t, router.handler)
}

func TestMountServeMuxOptions(t *testing.T) {
	mux := http.NewServeMux()
	if err := oida.Mount(mux, mountedTracer(t)); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	checkDashboard(t, mux)
}

func TestMountValidation(t *testing.T) {
	if err := oida.Mount(nil, mountedTracer(t)); !errors.Is(err, oida.ErrNilRouter) {
		t.Errorf("Mount(nil) returned %v, want oida.ErrNilRouter", err)
	}
	if err := oida.Mount(&stubRouter{}, nil); !errors.Is(err, oida.ErrNoTracer) {
		t.Errorf("Mount without a tracer returned %v, want oida.ErrNoTracer", err)
	}
}

// stubRouter records what was mounted on it. It stands in for the routers oida
// does not import, and satisfies oida.Router the way chi.Router and
// *http.ServeMux do.
type stubRouter struct {
	patterns []string
	handler  http.Handler
}

// Handle implements oida.Router.
func (r *stubRouter) Handle(pattern string, h http.Handler) {
	r.patterns = append(r.patterns, pattern)
	r.handler = h
}
