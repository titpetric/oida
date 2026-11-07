package frontend_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/titpetric/oida"
	"github.com/titpetric/oida/frontend"
	"github.com/titpetric/oida/model"
)

// collectPath is the route a client posts transactions to.
const collectPath = oida.DefaultPath + model.CollectRoute

// pageloadJSON is the collection the browser library posts on load, written
// out the way the library writes it.
const pageloadJSON = `{"transactions":[{
	"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
	"span_id": "aaaaaaaaaaaaaaa1",
	"name": "pageload /runs",
	"kind": "browser",
	"started_at": 1700000000000,
	"duration_ms": 820,
	"attributes": {"url": "https://task.cdn.si/runs", "title": "Runs"},
	"http": {"method": "GET", "uri": "/runs", "host": "task.cdn.si", "status": 200},
	"spans": [{"name": "documentFetch", "start_ms": 12, "duration_ms": 140}]
}]}`

// collecting returns a front end with the collect route turned on, and the
// tracer behind it.
func collecting(t *testing.T) (*oida.Tracer, http.Handler) {
	t.Helper()

	tracer, _ := newTestTracer(t, func(o *oida.Options) { o.Collect = true })
	return tracer, frontend.Handler(tracer)
}

// post sends a collection and returns the receipt the route answered with.
func post(t *testing.T, handler http.Handler, body string) (*httptest.ResponseRecorder, model.Receipt) {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, collectPath, strings.NewReader(body))
	r.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	r.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	var receipt model.Receipt
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatalf("the receipt did not decode: %v", err)
		}
	}
	return w, receipt
}

func TestCollectRecordsAPageload(t *testing.T) {
	tracer, handler := collecting(t)

	response, receipt := post(t, handler, pageloadJSON)
	if response.Code != http.StatusOK {
		t.Fatalf("the route answered %d: %s", response.Code, response.Body)
	}
	if len(receipt.Transactions) != 1 {
		t.Fatalf("the receipt answers %d transactions", len(receipt.Transactions))
	}
	collected := receipt.Transactions[0]
	want := "00-4bf92f3577b34da6a3ce929d0e0e4736-aaaaaaaaaaaaaaa1-01"
	if collected.TraceParent != want {
		t.Fatalf("the receipt reads %q, want %q", collected.TraceParent, want)
	}

	trace, ok := tracer.Trace(collected.ID)
	if !ok {
		t.Fatalf("the transaction was not retained under %q", collected.ID)
	}
	if trace.Name != "pageload /runs" || len(trace.Spans) != 2 {
		t.Fatalf("unexpected trace: %+v", trace)
	}
	if trace.HTTP == nil || trace.HTTP.UserAgent != "Mozilla/5.0 (X11; Linux x86_64)" {
		t.Fatalf("the user agent the route saw was not recorded: %+v", trace.HTTP)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("the receipt is cacheable: %q", got)
	}
}

func TestCollectIsOffUntilTurnedOn(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)
	handler := frontend.Handler(tracer)

	response, _ := post(t, handler, pageloadJSON)
	if response.Code != http.StatusNotFound {
		t.Fatalf("the route answered %d with Collect off, want 404", response.Code)
	}
	if traces := tracer.Traces(); len(traces) != 0 {
		t.Fatalf("a closed route recorded %d traces", len(traces))
	}
}

func TestCollectTakesPostAlone(t *testing.T) {
	_, handler := collecting(t)

	response := request(t, handler, collectPath, nil)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("a GET answered %d, want 405", response.Code)
	}
	if got := response.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("the route allows %q", got)
	}
}

func TestCollectRefusesNonsense(t *testing.T) {
	_, handler := collecting(t)

	for _, body := range []string{"", "not json", `{"transactions":[]}`} {
		response, _ := post(t, handler, body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%q answered %d, want 400", body, response.Code)
		}
	}
}

func TestCollectBoundsTheBody(t *testing.T) {
	_, handler := collecting(t)

	body := `{"transactions":[{"name":"` + strings.Repeat("x", model.CollectLimit) + `"}]}`
	response, _ := post(t, handler, body)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("a body past the beacon limit answered %d, want 400", response.Code)
	}
}
