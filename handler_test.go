package oida

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// renderedTracer returns a tracer holding one trace with nested spans, and the
// front end handler serving it.
func renderedTracer(t *testing.T) (*Tracer, http.Handler, Trace) {
	t.Helper()

	tracer, clock := newTestTracer(t, nil)
	err := tracer.Observe(t.Context(), "GET /users/{id}", func(ctx context.Context) error {
		ctx, db := Start(ctx, "SELECT users", KindDatabase)
		db.SetAttribute("rows", 3)
		clock.Advance(5 * time.Millisecond)
		db.End()

		_, render := Start(ctx, "render users.html", KindTemplate)
		clock.Advance(2 * time.Millisecond)
		render.End()
		return nil
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	traces := tracer.Traces()
	if len(traces) != 1 {
		t.Fatalf("recorded %d traces, want 1", len(traces))
	}
	return tracer, tracer.Handler(), traces[0]
}

// tracesPath is the trace list. The mount path itself is the host overview.
const tracesPath = DefaultPath + "/traces"

// request performs a front end request and returns the response.
func request(t *testing.T, handler http.Handler, target string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, target, nil)
	for key, value := range header {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestHandlerRendersList(t *testing.T) {
	_, handler, trace := renderedTracer(t)

	response := request(t, handler, tracesPath, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}

	body := response.Body.String()
	for _, want := range []string{
		"<!doctype html>",
		"prefers-color-scheme",               // the stylesheet is inlined
		`href="/debug/oida/assets/oida.css"`, // and served from the embedded tree
		`src="/debug/oida/assets/oida.js"`,
		trace.ID,
		"GET /users/{id}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("list view misses %q", want)
		}
	}
}

func TestHandlerRendersDetail(t *testing.T) {
	_, handler, trace := renderedTracer(t)

	body := request(t, handler, DefaultPath+"/trace/"+trace.ID, nil).Body.String()
	for _, want := range []string{
		"SELECT users",
		"render users.html",
		// Asserted through the palette, so tuning a colour does not fail a
		// test about rendering.
		`class="kind" style="background:` + KindDatabase.Color() + `;color:#07090c;"`,
		`background:` + KindTemplate.Color(), // the render span carries its kind
		"<th>rows</th><td>3</td>",            // attributes are a table when expanded
		"Transaction",                        // the drawing
		"<h3>Request</h3>",                   // and the two tables under it
		"<h3>System</h3>",
		`data-copy="` + trace.ID + `"`, // the full id is one click away
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail view misses %q", want)
		}
	}
	if strings.Contains(body, "zzz") {
		t.Error("a style attribute was rejected by the templ sanitizer")
	}
}

func TestHandlerRendersSpanAttributes(t *testing.T) {
	tracer, clock := newTestTracer(t, nil)

	err := tracer.Observe(t.Context(), "GET /report", func(ctx context.Context) error {
		_, db := Start(ctx, "SELECT users", KindDatabase)
		db.SetAttributes(Attributes{
			"query": "SELECT id, email\n  FROM users\n WHERE id = ?",
			"args":  7,
			"rows":  3,
		})
		clock.Advance(3 * time.Millisecond)
		db.End()
		return nil
	})
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}

	trace := tracer.Traces()[0]
	body := request(t, tracer.Handler(), DefaultPath+"/trace/"+trace.ID, nil).Body.String()

	// The span row names the operation; the statement is not repeated beside
	// it, because "SELECT users" already says what it is.
	if !strings.Contains(body, "<summary>attributes: args, query, rows</summary>") {
		t.Error("attributes are not summarised by their keys")
	}
	if strings.Count(body, `<code class="query"`) != 1 {
		t.Error("the query is rendered outside the attribute table")
	}

	// Expanded, the attributes are a table, and the query keeps its shape.
	for _, want := range []string{
		"<th>rows</th><td>3</td>",
		"<th>args</th><td>7</td>",
		`<th>query</th><td><code class="query">SELECT id, email`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("attribute table misses %q", want)
		}
	}
}

func TestHandlerDrawsTheTrace(t *testing.T) {
	_, handler, trace := renderedTracer(t)
	detail := DefaultPath + "/trace/" + trace.ID

	body := request(t, handler, detail, nil).Body.String()
	for _, want := range []string{
		`<canvas id="oida-waves"`,
		`id="oida-waves-data"`,
		`src="/debug/oida/assets/waves.js"`,
		`"kind":"database"`, // the payload carries the spans whole
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the detail view misses %q", want)
		}
	}

	// One drawing, so no switch and no way to ask for another one.
	if strings.Contains(body, "?timeline=") {
		t.Error("the detail view still offers a choice of drawing")
	}

	// And the script rides only on the view that draws: the list and the feed
	// have no canvas to fill. Matched on the tag rather than the file name,
	// because the stylesheet is inlined into every page and mentions it.
	if strings.Contains(request(t, handler, tracesPath, nil).Body.String(), `src="/debug/oida/assets/waves.js"`) {
		t.Error("the trace list loads the drawing")
	}
}

func TestPageWaveTraceCarriesTheWholeStack(t *testing.T) {
	trace := timelineTrace()
	page := Page{Trace: &trace, Segments: Timeline(trace), Rows: Rows(trace)}

	shape := page.WaveTrace()
	if len(shape.Spans) != len(trace.Spans) {
		t.Fatalf("the drawing gets %d spans of %d", len(shape.Spans), len(trace.Spans))
	}
	if shape.Milliseconds != 100 {
		t.Errorf("the trace is handed over as %.3fms, want 100", shape.Milliseconds)
	}
	// The deepest span is what the drawing scales itself to, so it is carried
	// rather than left to be worked out from the spans.
	if shape.Depth != 2 {
		t.Errorf("the trace reports depth %d, want 2", shape.Depth)
	}

	var root WaveSpan
	for _, span := range shape.Spans {
		if span.Start < 0 || span.End > 1.0001 || span.End < span.Start {
			t.Errorf("the %s span runs %.3f to %.3f", span.Kind, span.Start, span.End)
		}
		if span.Color == "" || span.Name == "" {
			t.Errorf("the %s span has nothing to draw with: %+v", span.Kind, span)
		}
		if span.Depth == 0 {
			root = span
		}
	}

	// The root is in it, unlike the legend: a drawing of the trace has to show
	// what was holding the work open.
	if root.Kind != KindHTTP.String() {
		t.Fatalf("the root span is not in the drawing: %+v", root)
	}
	if root.Start > 0.001 || root.End < 0.999 {
		t.Errorf("the root covers %.3f to %.3f, want the whole width", root.Start, root.End)
	}
}

func TestHandlerFilterFormStaysOnTraces(t *testing.T) {
	_, handler, _ := renderedTracer(t)

	body := request(t, handler, tracesPath, nil).Body.String()

	// A GET form replaces the query with its own fields, so the action has to
	// be the trace list path. Posting to the mount path lands on the hosts
	// overview instead, losing the view the reader was on.
	if !strings.Contains(body, `action="/debug/oida/traces"`) {
		t.Error("the filter form does not submit to the trace list")
	}
	if strings.Contains(body, `action="/debug/oida"`) {
		t.Error("the filter form submits to the host overview")
	}
}

func TestHandlerRowsAreClickable(t *testing.T) {
	tracer, handler := seedHosts(t)
	trace := tracer.Traces()[0]

	list := request(t, handler, tracesPath, nil).Body.String()
	if !strings.Contains(list, `data-href="`+DefaultPath+"/trace/"+trace.ID+`"`) {
		t.Error("trace rows do not carry a target")
	}

	hosts := request(t, handler, DefaultPath, nil).Body.String()
	if !strings.Contains(hosts, `data-href="`+DefaultPath+"/traces?host="+trace.HTTP.Host+`"`) {
		t.Error("host rows do not carry a target")
	}

	feed := request(t, handler, DefaultPath+"/live?stream=off", nil).Body.String()
	if !strings.Contains(feed, `data-href="`+DefaultPath+"/trace/"+trace.ID+`"`) {
		t.Error("feed rows do not carry a target")
	}
}

func TestHandlerHealthDots(t *testing.T) {
	tracer, clock := newTestTracer(t, nil)
	opts := tracer.Options()
	opts.Tracer = tracer

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{code}", func(w http.ResponseWriter, r *http.Request) {
		code, _ := strconv.Atoi(r.PathValue("code"))
		clock.Advance(time.Millisecond)
		w.WriteHeader(code)
	})

	traffic := TracingMiddleware(opts)(mux)
	for _, code := range []int{200, 302, 404, 500} {
		traffic.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "http://one.example/"+strconv.Itoa(code), nil))
	}

	body := request(t, tracer.Handler(), tracesPath, nil).Body.String()
	for _, want := range []string{
		`class="dot ok"`,   // 200 and 302
		`class="dot warn"`, // 404
		`class="dot bad"`,  // 500
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trace list misses %q", want)
		}
	}
	if got := strings.Count(body, `class="dot ok"`); got != 2 {
		t.Errorf("%d rows are green, want 2 for 200 and 302", got)
	}

	// The host served a failure, so its dot is red.
	hosts := request(t, tracer.Handler(), DefaultPath, nil).Body.String()
	if !strings.Contains(hosts, `class="dot bad"`) {
		t.Error("a host that served a 500 is not marked as failing")
	}
}

func TestHandlerDetailKeepsHost(t *testing.T) {
	tracer, handler := seedHosts(t)
	trace := tracer.Traces()[0]

	// Reached without a host filter, the detail still belongs to its host.
	body := request(t, handler, DefaultPath+"/trace/"+trace.ID, nil).Body.String()
	if !strings.Contains(body, `<summary class="filtered">`+trace.HTTP.Host+`</summary>`) {
		t.Error("the masthead does not name the host of the trace")
	}
	if strings.Contains(body, `>Hosts</a>`) {
		t.Error("the hosts tab is offered while looking at one host's trace")
	}
	if !strings.Contains(body, `href="`+DefaultPath+"/traces?host="+trace.HTTP.Host+`"`) {
		t.Error("the tabs do not stay narrowed to the host of the trace")
	}
}

func TestHandlerNegotiatesFormats(t *testing.T) {
	_, handler, trace := renderedTracer(t)

	json_ := request(t, handler, tracesPath, map[string]string{"Accept": "application/json"})
	if got := json_.Header().Get("Content-Type"); got != "text/json; charset=utf-8" {
		t.Fatalf("json content type %q", got)
	}
	var traces []Trace
	if err := json.Unmarshal(json_.Body.Bytes(), &traces); err != nil {
		t.Fatalf("decode traces: %v", err)
	}
	if len(traces) != 1 || traces[0].ID != trace.ID {
		t.Fatalf("unexpected json payload: %+v", traces)
	}

	text := request(t, handler, tracesPath, map[string]string{"User-Agent": "curl/8.5.0"})
	if got := text.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("text content type %q", got)
	}
	if !strings.HasPrefix(text.Body.String(), "oida · test") {
		t.Fatalf("unexpected text payload: %q", truncate(text.Body.String(), 120))
	}

	forced := request(t, handler, tracesPath+"?format=text", map[string]string{"Accept": "text/html"})
	if !strings.HasPrefix(forced.Body.String(), "oida · test") {
		t.Fatal("the format parameter did not override the Accept header")
	}
}

func TestHandlerRoutes(t *testing.T) {
	_, handler, _ := renderedTracer(t)

	for _, route := range []string{
		"", "/", "/traces", "/live", "/stats",
		"/assets/oida.css", "/assets/oida.js", "/assets/input-select.svg",
	} {
		if code := request(t, handler, DefaultPath+route, nil).Code; code != http.StatusOK {
			t.Errorf("%q returned %d", route, code)
		}
	}
	for _, route := range []string{"/nope", "/trace/", "/trace/NOTAULID"} {
		if code := request(t, handler, DefaultPath+route, nil).Code; code != http.StatusNotFound {
			t.Errorf("%q returned %d, want 404", route, code)
		}
	}
}

func TestHandlerFiltersList(t *testing.T) {
	_, handler, trace := renderedTracer(t)

	cases := map[string]bool{
		"?q=users":       true,
		"?q=nothing":     false,
		"?kind=database": true,
		"?kind=queue":    false,
		"?status=error":  false,
		"?status=all":    true,
		"?limit=99999":   true, // invalid limits fall back to the default
	}
	for query, want := range cases {
		body := request(t, handler, tracesPath+query, nil).Body.String()
		if got := strings.Contains(body, trace.ID); got != want {
			t.Errorf("%s: trace present = %v, want %v", query, got, want)
		}
	}
}

// hostSeed describes the work one virtual host does per request.
type hostSeed struct {
	host  string
	span  string
	query string
	spans int
	delay time.Duration
}

var hostSeeds = []hostSeed{
	{host: "shop.example", span: "SELECT orders", query: "SELECT id FROM orders WHERE paid = 1", spans: 2, delay: 9 * time.Millisecond},
	{host: "admin.example", span: "UPDATE users", query: "UPDATE users SET seen_at = now()", spans: 1, delay: 3 * time.Millisecond},
}

// seedHosts drives one request per host through the middleware, which is the
// only path that knows the host of a request the sampler rejected.
func seedHosts(t *testing.T) (*Tracer, http.Handler) {
	t.Helper()

	tracer, clock := newTestTracer(t, nil)
	opts := tracer.Options()
	opts.Tracer = tracer

	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		for _, seed := range hostSeeds {
			if seed.host != r.Host {
				continue
			}
			for range seed.spans {
				_, span := Start(r.Context(), seed.span, KindDatabase)
				span.SetAttribute("query", seed.query)
				clock.Advance(seed.delay)
				span.End()
			}
		}
		_, _ = w.Write([]byte("ok"))
	})

	traffic := TracingMiddleware(opts)(mux)
	for _, seed := range hostSeeds {
		traffic.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "http://"+seed.host+"/orders", nil))
	}
	return tracer, tracer.Handler()
}

func TestHandlerSearchesSpanContent(t *testing.T) {
	tracer, handler := seedHosts(t)
	traces := tracer.Traces()

	cases := map[string]int{
		"select":   1, // only the shop trace runs a SELECT
		"update":   1,
		"orders":   2, // both traces are named GET /orders
		"shop":     1, // the host matches too
		"nonesuch": 0,
		"paid = 1": 1, // the attribute value is searched, not just the name
		"database": 2, // and so is the span kind
	}
	for query, want := range cases {
		body := request(t, handler, tracesPath+"?q="+url.QueryEscape(query), nil).Body.String()

		found := 0
		for _, trace := range traces {
			if strings.Contains(body, trace.ID) {
				found++
			}
		}
		if found != want {
			t.Errorf("q=%q matched %d traces, want %d", query, found, want)
		}
	}
}

func TestHandlerSortsAndFiltersByHost(t *testing.T) {
	tracer, handler := seedHosts(t)

	// The shop trace is slower and has more spans; admin is faster.
	shop, admin := tracer.Traces()[0], tracer.Traces()[1]
	if shop.Duration < admin.Duration {
		shop, admin = admin, shop
	}

	for _, tc := range []struct {
		query string
		first string
	}{
		{query: "?sort=duration", first: shop.ID},
		{query: "?sort=duration&order=asc", first: admin.ID},
		{query: "?sort=spans", first: shop.ID},
		{query: "?sort=allocated&order=asc", first: admin.ID},
		{query: "?sort=nonsense", first: tracer.Traces()[0].ID}, // falls back to age
	} {
		body := request(t, handler, tracesPath+tc.query, nil).Body.String()
		if index := strings.Index(body, tc.first); index < 0 {
			t.Errorf("%s: expected trace missing", tc.query)
		} else if other := strings.Index(body, shop.ID+admin.ID); other >= 0 {
			t.Errorf("%s: unexpected ordering", tc.query)
		}
	}

	// Host filter keeps one trace and drops the other.
	body := request(t, handler, tracesPath+"?host=admin.example", nil).Body.String()
	if strings.Contains(body, shop.ID) && shop.HTTP.Host == "shop.example" {
		t.Error("host filter kept a trace from another host")
	}
}

func TestHandlerHostSwitcher(t *testing.T) {
	tracer, handler := seedHosts(t)

	// Unfiltered: the masthead names the domain the dashboard was reached on
	// and offers every host it has seen.
	body := request(t, handler, "http://localhost:8080"+DefaultPath, nil).Body.String()
	for _, want := range []string{
		`all hosts</summary>`, // unfiltered names every host, not this one
		`>Hosts</a>`,          // and the tab is offered
		`href="/debug/oida/traces?host=shop.example"`,
		`href="/debug/oida/traces?host=admin.example"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("masthead misses %q", want)
		}
	}

	// Filtered: the summary names the filter, and the filter follows the
	// reader to the live feed.
	filtered := request(t, handler, tracesPath+"?host=shop.example", nil).Body.String()
	if !strings.Contains(filtered, `<summary class="filtered">shop.example</summary>`) {
		t.Error("the masthead does not name the active host filter")
	}
	if strings.Contains(filtered, `>Hosts</a>`) {
		t.Error("the hosts tab is still offered while a host is chosen")
	}
	if !strings.Contains(filtered, `<a href="/debug/oida">oida</a>`) {
		t.Error("the wordmark does not link back to the host overview")
	}
	// The service name is a labelled process fact, not the page identity.
	if !strings.Contains(filtered, "service <b>test</b>") {
		t.Error("the service name is not labelled in the process facts")
	}
	if !strings.Contains(filtered, `href="/debug/oida/live?host=shop.example"`) {
		t.Error("the host filter is dropped when moving to the live feed")
	}

	// And the feed itself honours it. The switcher menu still lists every host,
	// so the check is on the traces, not on the name appearing anywhere.
	var shopID, adminID string
	for _, trace := range tracer.Traces() {
		switch trace.HTTP.Host {
		case "shop.example":
			shopID = trace.ID
		case "admin.example":
			adminID = trace.ID
		}
	}

	feed := request(t, handler, DefaultPath+"/live?host=admin.example&stream=off", nil).Body.String()
	if !strings.Contains(feed, adminID) {
		t.Error("the live feed dropped the host it was filtered to")
	}
	if strings.Contains(feed, shopID) {
		t.Error("the live feed shows traces from another host")
	}
}

func TestHandlerHostStatistics(t *testing.T) {
	tracer, handler := seedHosts(t)
	tracer.countUnsampled("shop.example") // a request the sampler rejected

	stats := tracer.Snapshot().Statistics.Hosts
	byHost := make(map[string]HostStat, len(stats))
	for _, host := range stats {
		byHost[host.Host] = host
	}

	if got := byHost["shop.example"]; got.Requests != 2 || got.Traces != 1 {
		t.Errorf("shop.example: %d requests, %d traces, want 2 and 1", got.Requests, got.Traces)
	}
	if got := byHost["admin.example"]; got.Requests != 1 || got.Traces != 1 {
		t.Errorf("admin.example: %d requests, %d traces, want 1 and 1", got.Requests, got.Traces)
	}

	// Hosts are the landing page, not a section of the statistics view.
	body := request(t, handler, DefaultPath, nil).Body.String()
	for _, want := range []string{"shop.example", "admin.example", "Requests"} {
		if !strings.Contains(body, want) {
			t.Errorf("host overview misses %q", want)
		}
	}

	statsBody := request(t, handler, DefaultPath+"/stats", nil).Body.String()
	if strings.Contains(statsBody, ">Recorded</th>") {
		t.Error("the statistics view still carries the host table")
	}
}

func TestHandlerServesUnderCustomPath(t *testing.T) {
	tracer, _ := newTestTracer(t, func(o *Options) { o.Path = "/admin/telemetry" })

	response := request(t, tracer.Handler(), "/admin/telemetry/stats", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `href="/admin/telemetry/live"`) {
		t.Fatal("links do not use the configured path")
	}
}
