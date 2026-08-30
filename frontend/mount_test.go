package frontend_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
)

// newTestServer returns a mux with the front end mounted on a private tracer,
// wrapped in the tracing middleware, which is how a service wires oida up.
func newTestServer(t *testing.T, apply func(*oida.Options)) (http.Handler, *oida.Tracer) {
	t.Helper()

	tracer, _ := newTestTracer(t, apply)
	opts := tracer.Options()
	opts.Tracer = tracer

	mux := http.NewServeMux()
	if err := frontend.MountServeMux(mux, opts); err != nil {
		t.Fatalf("MountServeMux: %v", err)
	}

	return tracer.Middleware(mux), tracer
}

func TestHandlerAuthorization(t *testing.T) {
	handler, _ := newTestServer(t, func(o *oida.Options) {
		o.Authorize = func(r *http.Request) bool { return r.Header.Get("X-Admin") == "yes" }
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, oida.DefaultPath, nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unauthorized request got %d, want 404", response.Code)
	}

	request := httptest.NewRequest(http.MethodGet, oida.DefaultPath, nil)
	request.Header.Set("X-Admin", "yes")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized request got %d", response.Code)
	}
}

func TestMountValidation(t *testing.T) {
	opts := oida.NewOptions("")
	if err := frontend.Mount(nil, opts); !errors.Is(err, oida.ErrNilRouter) {
		t.Fatalf("Mount(nil) returned %v, want oida.ErrNilRouter", err)
	}

	opts.Path = "debug/oida"
	if err := frontend.Mount(&stubRouter{}, opts); !errors.Is(err, oida.ErrInvalidPath) {
		t.Fatalf("relative path returned %v, want oida.ErrInvalidPath", err)
	}

	opts = oida.NewOptions("")
	opts.SampleRate = 200
	if err := frontend.Mount(&stubRouter{}, opts); !errors.Is(err, oida.ErrInvalidSampleRate) {
		t.Fatalf("invalid sample rate returned %v, want oida.ErrInvalidSampleRate", err)
	}

	opts = oida.NewOptions("")
	opts.Tracer, _ = newTestTracer(t, nil)
	router := &stubRouter{}
	if err := frontend.Mount(router, opts); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if router.pattern != oida.DefaultPath || router.handler == nil {
		t.Fatalf("mounted %q with handler %v", router.pattern, router.handler)
	}
}

// stubRouter records what was mounted on it.
type stubRouter struct {
	pattern string
	handler http.Handler
}

// Mount implements frontend.Router.
func (r *stubRouter) Mount(pattern string, h http.Handler) {
	r.pattern = pattern
	r.handler = h
}
