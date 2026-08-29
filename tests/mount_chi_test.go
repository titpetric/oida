package tests_test

import (
	"testing"

	chi "github.com/go-chi/chi/v5"
)

func TestMountChi(t *testing.T) {
	tracer := mountedTracer(t)

	router := chi.NewRouter()
	router.Mount("/debug/oida", tracer)

	checkDashboard(t, router)
}

func TestMountChiStatusPage(t *testing.T) {
	tracer := mountedTracerAt(t, "/status-page")

	router := chi.NewRouter()
	router.Mount("/status-page", tracer)

	checkDashboardAt(t, router, "/status-page")
}
