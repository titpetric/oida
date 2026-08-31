package frontend_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
)

// TestVirtualHostsKeepSeparateTracers wires two hosts to two tracers on one
// router, the way a virtual host setup does: each host records into its own
// ring buffer and serves its own dashboard, and neither can see the other.
func TestVirtualHostsKeepSeparateTracers(t *testing.T) {
	shop := hostRouter(t, "shop.example", "GET /orders")
	admin := hostRouter(t, "admin.example", "GET /users")

	// One router in front, dispatching by Host.
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Host {
		case "shop.example":
			shop.handler.ServeHTTP(w, r)
		case "admin.example":
			admin.handler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	for range 3 {
		router.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "http://shop.example/work", nil))
	}
	router.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "http://admin.example/work", nil))

	if got := len(shop.tracer.Traces()); got != 3 {
		t.Errorf("shop recorded %d traces, want 3", got)
	}
	if got := len(admin.tracer.Traces()); got != 1 {
		t.Errorf("admin recorded %d traces, want 1", got)
	}

	// Each dashboard is mounted at the same path but serves its own tracer.
	shopBody := dashboard(t, router, "shop.example")
	adminBody := dashboard(t, router, "admin.example")

	shopID := shop.tracer.Traces()[0].ID
	adminID := admin.tracer.Traces()[0].ID

	if !strings.Contains(shopBody, shopID) || strings.Contains(shopBody, adminID) {
		t.Error("the shop dashboard does not show exactly its own traces")
	}
	if !strings.Contains(adminBody, adminID) || strings.Contains(adminBody, shopID) {
		t.Error("the admin dashboard does not show exactly its own traces")
	}
	if !strings.Contains(shopBody, "shop.example") || !strings.Contains(adminBody, "admin.example") {
		t.Error("dashboards do not carry their own service name")
	}
}

// vhost is one host's tracer and the handler serving it.
type vhost struct {
	tracer  *oida.Tracer
	handler http.Handler
}

// hostRouter builds one virtual host: its own tracer, its own middleware, its
// own dashboard mounted at the same path.
func hostRouter(t *testing.T, service, span string) vhost {
	t.Helper()

	opts := oida.NewOptions(service)
	opts.Enabled = true
	opts.TrackMemoryUse = false
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }

	tracer, err := oida.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /work", func(w http.ResponseWriter, r *http.Request) {
		oida.StartSpan(r.Context(), span, oida.KindDatabase).End()
		_, _ = w.Write([]byte(service))
	})
	mux.Handle(opts.Path+"/", frontend.Handler(tracer))

	return vhost{tracer: tracer, handler: tracer.Middleware(mux)}
}

// dashboard fetches one host's trace list.
func dashboard(t *testing.T, router http.Handler, host string) string {
	t.Helper()

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://"+host+oida.DefaultPath+"/traces", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("%s dashboard returned %d", host, response.Code)
	}
	return response.Body.String()
}
