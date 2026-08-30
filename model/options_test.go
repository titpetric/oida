package model

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOptionsAuthorized(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/oida", nil)

	open := NewOptions("test")
	if !open.Authorized(request) {
		t.Error("a nil Authorize denies; it must allow")
	}

	closed := NewOptions("test")
	closed.Authorize = func(*http.Request) bool { return false }
	if closed.Authorized(request) {
		t.Error("a denying Authorize allowed the request")
	}

	inspecting := NewOptions("test")
	inspecting.Authorize = func(r *http.Request) bool { return r.URL.Path == "/debug/oida" }
	if !inspecting.Authorized(request) {
		t.Error("Authorize did not receive the request it decides on")
	}
}
