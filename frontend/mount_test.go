package frontend_test

import (
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

	mux := http.NewServeMux()
	mux.Handle(oida.DefaultPath+"/", frontend.HandlerFor(tracer))
	mux.Handle(oida.DefaultPath, frontend.HandlerFor(tracer))

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
