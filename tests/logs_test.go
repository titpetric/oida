package tests_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/oida"
)

// loggedTracer returns a tracer holding one recorded trace with two log
// entries, built without touching the process default.
func loggedTracer(t *testing.T, apply func(*oida.Options)) *oida.Tracer {
	t.Helper()

	opts := oida.NewOptions("logs-test")
	opts.Enabled = true
	opts.TrackMemoryUse = false
	opts.OnError = func(err error) { t.Errorf("oida: %v", err) }
	if apply != nil {
		apply(&opts)
	}

	tracer, err := oida.New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = tracer.Observe(t.Context(), "GET /users/{id}", func(ctx context.Context) error {
		trace := oida.TraceFromContext(ctx)

		_, db := oida.Start(ctx, "SELECT users", oida.KindDatabase)
		trace.Info("user loaded", "user_id", 42)
		db.End()

		trace.Error("stale cache entry evicted", "key", "user:42")
		return nil
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	return tracer
}

// detailPage renders the detail view of a trace through the tracer itself,
// which serves the debug front end.
func detailPage(t *testing.T, tracer *oida.Tracer, id, format string) string {
	t.Helper()

	target := oida.DefaultPath + "/trace/" + id
	if format != "" {
		target += "?format=" + format
	}
	response := httptest.NewRecorder()
	tracer.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s returned %d", target, response.Code)
	}
	return response.Body.String()
}

func TestLogs(t *testing.T) {
	tracer := loggedTracer(t, nil)
	trace := tracer.Traces()[0]

	// The entries survive the clone into ring storage.
	if len(trace.Logs) != 2 {
		t.Fatalf("the stored trace carries %d log entries, want 2", len(trace.Logs))
	}
	if trace.Logs[0].SpanID == 0 {
		t.Error("the entry written inside the span is not linked to it")
	}
	if trace.Logs[0].RequestID != trace.ID {
		t.Errorf("entry carries request id %q, want the trace id", trace.Logs[0].RequestID)
	}
	// Error only logs; the transaction is not failed by it.
	if trace.ErrorText != "" || trace.State == oida.StateError {
		t.Error("an Error log entry marked the transaction as failed")
	}
}

func TestLogsTab(t *testing.T) {
	tracer := loggedTracer(t, nil)
	trace := tracer.Traces()[0]

	body := detailPage(t, tracer, trace.ID, "")
	for _, want := range []string{
		`id="oida-tab-logs"`, // the Log tab beside the spans
		`Log <b>2</b>`,       // named with its count
		"user loaded",
		"user_id=42",   // the slog-style pair, rendered inline
		"SELECT users", // the entry names the span it was written under
		`class="log-level log-level-error"`,
		"stale cache entry evicted",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail view misses %q", want)
		}
	}
	if strings.Contains(body, "No log entries") {
		t.Error("the empty state is drawn for a trace with log entries")
	}
}

func TestLogsText(t *testing.T) {
	tracer := loggedTracer(t, nil)
	trace := tracer.Traces()[0]

	text := detailPage(t, tracer, trace.ID, "text")
	for _, want := range []string{"LEVEL", "user loaded user_id=42", "error", "stale cache entry evicted key=user:42"} {
		if !strings.Contains(text, want) {
			t.Errorf("text detail misses %q", want)
		}
	}
}

func TestLogsEmptyState(t *testing.T) {
	tracer := mountedTracer(t)
	trace := tracer.Traces()[0]

	body := detailPage(t, tracer, trace.ID, "")
	if !strings.Contains(body, `Log <b>0</b>`) {
		t.Error("the Log tab does not name its count")
	}
	// The empty state says what to do next, not just that nothing is there.
	if !strings.Contains(body, "Record one with span.Info or trace.Error") {
		t.Error("the empty log panel does not say how to record an entry")
	}
}

func TestLogsDisabled(t *testing.T) {
	tracer := loggedTracer(t, func(o *oida.Options) { o.CaptureLogs = false })
	trace := tracer.Traces()[0]

	if len(trace.Logs) != 0 {
		t.Fatalf("a disabled tracer recorded %d log entries", len(trace.Logs))
	}
	// Error fell back to RecordError with the formatted text, so the failure
	// is still visible on the trace.
	if trace.ErrorText != "stale cache entry evicted key=user:42" {
		t.Fatalf("the trace records %q, want the formatted text", trace.ErrorText)
	}
}
