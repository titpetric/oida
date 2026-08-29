package tests_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/tests"
)

// get performs a request against the test server and returns the response body.
func get(t *testing.T, client *http.Client, url string, header map[string]string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for key, value := range header {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return response.StatusCode, string(body)
}

func TestServerRecordsTraces(t *testing.T) {
	server := tests.NewHTTPServer(t)
	client := server.Client()

	for _, path := range []string{"/", "/users/42", "/slow"} {
		if status, _ := get(t, client, server.URL+path, nil); status != http.StatusOK {
			t.Fatalf("%s: status %d", path, status)
		}
	}

	status, body := get(t, client, server.URL+tests.Path+"/traces?format=json", nil)
	if status != http.StatusOK {
		t.Fatalf("front end status %d", status)
	}

	var traces []oida.Trace
	if err := json.Unmarshal([]byte(body), &traces); err != nil {
		t.Fatalf("decode traces: %v", err)
	}
	if len(traces) != 3 {
		t.Fatalf("recorded %d traces, want 3", len(traces))
	}

	byName := make(map[string]oida.Trace, len(traces))
	for _, trace := range traces {
		byName[trace.Name] = trace
	}

	user, ok := byName["GET /users/{id}"]
	if !ok {
		t.Fatalf("routed pattern missing, recorded: %v", byName)
	}
	if user.HTTP == nil || user.HTTP.Status != http.StatusOK {
		t.Fatalf("unexpected http info: %+v", user.HTTP)
	}
	if user.Duration <= 0 {
		t.Fatalf("trace duration not recorded: %s", user.Duration)
	}

	kinds := make(map[oida.Kind]int)
	for _, span := range user.Spans {
		kinds[span.Kind]++
		if span.Duration <= 0 {
			t.Errorf("span %q has no duration", span.Name)
		}
	}
	for _, kind := range []oida.Kind{oida.KindHTTP, oida.KindCache, oida.KindDatabase} {
		if kinds[kind] == 0 {
			t.Errorf("no %s span recorded, got %v", kind, kinds)
		}
	}
}

func TestServerRecordsFailures(t *testing.T) {
	server := tests.NewHTTPServer(t)
	client := server.Client()

	status, _ := get(t, client, server.URL+"/fail", nil)
	if status != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", status)
	}

	_, body := get(t, client, server.URL+tests.Path+"/traces?format=json&status=error", nil)
	var traces []oida.Trace
	if err := json.Unmarshal([]byte(body), &traces); err != nil {
		t.Fatalf("decode traces: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("recorded %d failed traces, want 1", len(traces))
	}
	if traces[0].ErrorText == "" {
		t.Fatalf("failed trace has no error: %+v", traces[0])
	}
}

func TestServerViews(t *testing.T) {
	server := tests.NewHTTPServer(t)
	client := server.Client()

	if status, _ := get(t, client, server.URL+"/users/7", nil); status != http.StatusOK {
		t.Fatalf("seed request status %d", status)
	}

	_, body := get(t, client, server.URL+tests.Path+"/traces", nil)
	if !strings.Contains(body, "<!doctype html>") && !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatalf("list view is not html: %s", truncate(body))
	}
	if !strings.Contains(body, "GET /users/{id}") {
		t.Fatalf("list view misses the trace: %s", truncate(body))
	}

	id := traceID(t, client, server.URL)
	status, detail := get(t, client, server.URL+tests.Path+"/trace/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("detail status %d", status)
	}
	for _, want := range []string{"Transaction", "SELECT users", "database"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail view misses %q", want)
		}
	}

	for _, view := range []string{"", "/traces", "/live", "/stats"} {
		status, body := get(t, client, server.URL+tests.Path+view, map[string]string{"User-Agent": "curl/8.0"})
		if status != http.StatusOK {
			t.Fatalf("%s text status %d", view, status)
		}
		if !strings.HasPrefix(body, "oida · oida-tests") {
			t.Errorf("%s is not the text rendering: %s", view, truncate(body))
		}
	}

	if status, _ := get(t, client, server.URL+tests.Path+"/assets/oida.css", nil); status != http.StatusOK {
		t.Fatalf("stylesheet status %d", status)
	}
	if status, _ := get(t, client, server.URL+tests.Path+"/trace/NOPE", nil); status != http.StatusNotFound {
		t.Fatalf("unknown trace status %d, want 404", status)
	}
}

func TestServerSkipsOwnRequests(t *testing.T) {
	server := tests.NewHTTPServer(t)
	client := server.Client()

	for range 3 {
		get(t, client, server.URL+tests.Path, nil)
	}

	_, body := get(t, client, server.URL+tests.Path+"/traces?format=json", nil)
	var traces []oida.Trace
	if err := json.Unmarshal([]byte(body), &traces); err != nil {
		t.Fatalf("decode traces: %v", err)
	}
	if len(traces) != 0 {
		t.Fatalf("front end requests were traced: %d", len(traces))
	}
}

// traceID returns the ID of the newest recorded trace.
func traceID(t *testing.T, client *http.Client, base string) string {
	t.Helper()

	_, body := get(t, client, base+tests.Path+"/traces?format=json", nil)
	var traces []oida.Trace
	if err := json.Unmarshal([]byte(body), &traces); err != nil {
		t.Fatalf("decode traces: %v", err)
	}
	if len(traces) == 0 {
		t.Fatal("no traces recorded")
	}
	return traces[0].ID
}

// truncate shortens a body for failure messages.
func truncate(body string) string {
	if len(body) > 400 {
		return body[:400] + "…"
	}
	return body
}
