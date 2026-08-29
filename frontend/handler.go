package frontend

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/titpetric/oida/model"
)

const (
	// liveFeedRows is the number of traces the live feed holds.
	liveFeedRows = 25

	// streamDebounce is the quiet period the live stream waits out before
	// redrawing, so a burst of traces produces one event.
	streamDebounce = 250 * time.Millisecond

	// streamHeartbeat is the comment interval keeping proxies from closing an
	// idle stream.
	streamHeartbeat = 20 * time.Second
)

// handler serves the debug front end of one recorder.
type handler struct {
	opts model.Options

	// recorder is the recorder the handler was bound to. A nil recorder is
	// resolved on every request, so a dashboard mounted before the tracer is
	// configured picks it up as soon as it exists.
	recorder model.Recorder

	// auth carries the authentication options: the network allow list and
	// the credentials behind the sign in screen. Nil when none are set.
	auth *model.Auth
}

var _ http.Handler = (*handler)(nil)

// Handler returns the debug front end handler for the recorder in opts, or the
// process default when none is set. Invalid options degrade to their defaults;
// use Mount when the error matters.
func Handler(opts model.Options) http.Handler {
	opts = opts.WithDefaults()
	return newHandler(opts, opts.Tracer)
}

// HandlerFor returns the debug front end handler of one recorder, configured
// the way that recorder is. It is the shortest path from a tracer to a
// dashboard, and it is what the tracer's own ServeHTTP builds.
func HandlerFor(recorder model.Recorder) http.Handler {
	opts := model.NewOptions("")
	if recorder != nil {
		opts = recorder.Options()
	}
	opts = opts.WithDefaults()
	opts.Tracer = recorder
	return newHandler(opts, recorder)
}

// newHandler returns the front end handler bound to a recorder.
func newHandler(opts model.Options, recorder model.Recorder) *handler {
	// NewAuth reports what it dropped through Options.OnError; what loaded
	// stays in force, and a dropped allow list entry only narrows access.
	auth, _ := model.NewAuth(opts)
	return &handler{opts: opts, recorder: recorder, auth: auth}
}

// tracer returns the recorder the request reads: the bound one, the process
// default, or an empty recorder while neither exists.
func (h *handler) tracer() model.Recorder {
	if h.recorder != nil {
		return h.recorder
	}
	if recorder := model.DefaultRecorder(); recorder != nil {
		return recorder
	}
	return nopRecorder{}
}

// ServeHTTP routes one front end request.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.opts.Authorized(r) {
		http.NotFound(w, r)
		return
	}

	// The network allow list is a hard filter: a peer outside it receives
	// the same 404 a failed Authorize sends.
	if !h.auth.NetworkAllowed(r) {
		http.NotFound(w, r)
		return
	}

	route := h.relative(r)
	if h.auth.LoginRequired() {
		if route == "/login" || route == "/login/" {
			h.serveLogin(w, r)
			return
		}
		// The embedded assets stay reachable: the sign in screen needs its
		// stylesheet, and the tree holds nothing recorded.
		if _, ok := h.auth.RequestUser(r); !ok && !strings.HasPrefix(route, assetPrefix) {
			h.serveUnauthorized(w, r)
			return
		}
	}

	switch {
	case route == "" || route == "/":
		h.serveHosts(w, r)
	case route == "/traces":
		h.serveList(w, r)
	case route == "/live":
		h.serveLive(w, r)
	case route == "/live/events":
		h.serveEvents(w, r)
	case route == "/stats":
		h.serveStats(w, r)
	case strings.HasPrefix(route, assetPrefix):
		h.serveAsset(w, r, route)
	default:
		if id, ok := strings.CutPrefix(route, "/trace/"); ok {
			h.serveDetail(w, r, strings.Trim(id, "/"))
			return
		}
		http.NotFound(w, r)
	}
}

// relative strips the mount path from the request path, so the same handler
// works under chi's Mount, a ServeMux pattern and http.StripPrefix.
func (h *handler) relative(r *http.Request) string {
	path := r.URL.Path
	if rest, ok := strings.CutPrefix(path, h.opts.Path); ok {
		return rest
	}
	return path
}

// serveHosts renders the landing page: the domains this process serves.
func (h *handler) serveHosts(w http.ResponseWriter, r *http.Request) {
	snapshot := h.tracer().Snapshot()
	page := h.page(snapshot, ViewHosts, r)
	page.Refresh = 0

	switch negotiate(r) {
	case formatJSON:
		writeJSON(w, snapshot.Statistics.Hosts)
	case formatText:
		writeText(w, page)
	default:
		h.render(w, r, Hosts(page))
	}
}

// serveList renders the completed trace log.
func (h *handler) serveList(w http.ResponseWriter, r *http.Request) {
	snapshot := h.tracer().Snapshot()
	page := h.page(snapshot, ViewList, r)
	snapshot.Log = filterTraces(snapshot.Log, page)
	sortTraces(snapshot.Log, page.Sort, page.Ascending)
	if page.Limit > 0 && len(snapshot.Log) > page.Limit {
		snapshot.Log = snapshot.Log[:page.Limit]
	}
	page.Snapshot = snapshot
	page.Slowest = slowest(snapshot.Log)
	page.Refresh = 0

	switch negotiate(r) {
	case formatJSON:
		writeJSON(w, snapshot.Log)
	case formatText:
		writeText(w, page)
	default:
		h.render(w, r, List(page))
	}
}

// serveLive renders the live feed.
func (h *handler) serveLive(w http.ResponseWriter, r *http.Request) {
	page := h.livePage(r)
	snapshot := page.Snapshot

	switch negotiate(r) {
	case formatJSON:
		writeJSON(w, snapshot.Live)
	case formatText:
		writeText(w, page)
	default:
		h.render(w, r, Live(page))
	}
}

// serveStats renders the rolling statistics.
func (h *handler) serveStats(w http.ResponseWriter, r *http.Request) {
	snapshot := h.tracer().Snapshot()
	page := h.page(snapshot, ViewStats, r)
	page.Refresh = 0

	switch negotiate(r) {
	case formatJSON:
		writeJSON(w, snapshot.Statistics)
	case formatText:
		writeText(w, page)
	default:
		h.render(w, r, Statistics(page))
	}
}

// serveDetail renders one trace.
func (h *handler) serveDetail(w http.ResponseWriter, r *http.Request, id string) {
	if !model.ValidID(id) {
		http.NotFound(w, r)
		return
	}
	trace, ok := h.tracer().Trace(id)
	if !ok {
		http.NotFound(w, r)
		return
	}

	page := h.page(h.tracer().Snapshot(), ViewDetail, r)
	// A trace belongs to a host whether or not the reader arrived through a
	// host filter, so the masthead keeps naming it and the views around it stay
	// narrowed to the same domain.
	if page.Host == "" {
		page.Host = model.TraceHost(trace)
	}
	page.Trace = &trace
	page.Rows = Rows(trace)
	page.Segments = Timeline(trace)
	page.Memory = TraceMemory(trace, page.Rows)
	page.Sources = hasSources(page.Rows)
	page.Title = trace.Name
	page.Refresh = 0

	switch negotiate(r) {
	case formatJSON:
		writeJSON(w, trace)
	case formatText:
		writeText(w, page)
	default:
		h.render(w, r, Detail(page))
	}
}

// serveAsset serves a file from the embedded asset tree. The file server
// resolves content types and rejects traversal, so anything dropped into
// frontend/assets is served without further wiring.
func (h *handler) serveAsset(w http.ResponseWriter, r *http.Request, route string) {
	w.Header().Set("Cache-Control", "max-age=3600")

	// The stylesheet carries asset URLs of its own, which have to point at this
	// mount path.
	if strings.HasSuffix(route, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = io.WriteString(w, styleSheetFor(h.opts.Path+assetPrefix))
		return
	}

	request := r.Clone(r.Context())
	request.URL.Path = route
	http.FileServerFS(assets()).ServeHTTP(w, request)
}

// serveEvents streams the live view over server sent events. The stream is
// driven by trace activity, not by a timer: the tracer wakes it when a trace
// starts or completes, and a burst of traces produces one redraw.
func (h *handler) serveEvents(w http.ResponseWriter, r *http.Request) {
	if !h.opts.LiveStream {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	controller := http.NewResponseController(w)
	// The stream outlives any sane write deadline.
	_ = controller.SetWriteDeadline(time.Time{})

	events, cancel := h.tracer().Subscribe()
	defer cancel()

	ctx := r.Context()
	if !h.writeLiveEvent(ctx, w, r, controller) {
		return
	}

	heartbeat := time.NewTicker(streamHeartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		case _, ok := <-events:
			if !ok {
				return
			}
			// Coalesce a burst into one redraw.
			select {
			case <-ctx.Done():
				return
			case <-time.After(streamDebounce):
			}
			drain(events)

			if !h.writeLiveEvent(ctx, w, r, controller) {
				return
			}
		}
	}
}

// writeLiveEvent renders the live section and writes it as one event. It
// reports whether the stream is still usable.
func (h *handler) writeLiveEvent(ctx context.Context, w io.Writer, r *http.Request, controller *http.ResponseController) bool {
	page := h.livePage(r)

	var buffer bytes.Buffer
	if err := liveSection(page).Render(ctx, &buffer); err != nil {
		h.tracer().ReportError(err)
		return false
	}
	if err := writeSSE(w, buffer.String()); err != nil {
		return false
	}
	return controller.Flush() == nil
}

// writeSSE writes one server sent event, splitting the payload across the data
// lines the protocol requires.
func writeSSE(w io.Writer, payload string) error {
	for line := range strings.SplitSeq(payload, "\n") {
		if _, err := io.WriteString(w, "data: "+line+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// drain empties pending notifications so a burst redraws once.
func drain(events <-chan struct{}) {
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// render writes an HTML component, reporting failures to the error handler.
func (h *handler) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		h.tracer().ReportError(err)
	}
}

// livePage builds the live view: traces in flight and completed traces merged
// into one feed, newest first, so a request appears the moment it starts and
// stays put as it finishes rather than jumping between two tables.
func (h *handler) livePage(r *http.Request) Page {
	snapshot := h.tracer().Snapshot()
	page := h.page(snapshot, ViewLive, r)

	feed := make([]model.Trace, 0, len(snapshot.Live)+len(snapshot.Log))
	feed = append(feed, snapshot.Live...)
	feed = append(feed, snapshot.Log...)
	if page.Host != "" {
		feed = filterTraces(feed, Page{Host: page.Host})
	}
	sort.SliceStable(feed, func(i, j int) bool {
		return feed[i].StartedAt.After(feed[j].StartedAt)
	})
	if len(feed) > liveFeedRows {
		feed = feed[:liveFeedRows]
	}

	page.Feed = feed
	page.Slowest = slowest(feed)
	return page
}

// page builds the view model shared by every component.
func (h *handler) page(snapshot model.Snapshot, view View, r *http.Request) Page {
	page := Page{
		Snapshot: snapshot,
		View:     view,
		Path:     h.opts.Path,
		Title:    "oida",
		Limit:    requestedLimit(r),
		Query:    strings.TrimSpace(r.URL.Query().Get("q")),
		Status:   requestedStatus(r),
		Refresh:  h.opts.RefreshInterval,
		Stream:   h.opts.LiveStream,
	}
	if kind := strings.TrimSpace(r.URL.Query().Get("kind")); kind != "" {
		page.Kind = model.Kind(kind)
	}
	page.Host = strings.TrimSpace(r.URL.Query().Get("host"))
	page.RequestHost = r.Host
	page.Sort = requestedSort(r)
	page.Ascending = r.URL.Query().Get("order") == "asc"
	// ?stream=off renders the live view as a static page. It makes the view
	// screenshottable and scrapeable, and it is the escape hatch when a proxy
	// mangles the event stream.
	if r.URL.Query().Get("stream") == "off" {
		page.Stream = false
	}
	return page
}

// filterTraces applies the list filters of a page.
func filterTraces(traces []model.Trace, page Page) []model.Trace {
	if page.Query == "" && page.Kind == "" && page.Host == "" &&
		(page.Status == "" || page.Status == "all") {
		return traces
	}

	out := make([]model.Trace, 0, len(traces))
	for _, trace := range traces {
		if !matches(trace, page.Query) {
			continue
		}
		if page.Kind != "" && !trace.HasKind(page.Kind) {
			continue
		}
		if page.Host != "" && model.TraceHost(trace) != page.Host {
			continue
		}
		if page.Status == "error" && trace.Error == "" && trace.State != model.StateError {
			continue
		}
		out = append(out, trace)
	}
	return out
}

// requestedLimit returns the row count selected on the list view.
func requestedLimit(r *http.Request) int {
	value, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		return limitOptions[0]
	}
	for _, option := range limitOptions {
		if option == value {
			return value
		}
	}
	return limitOptions[0]
}

// requestedSort returns the column the list is ordered by, defaulting to age.
func requestedSort(r *http.Request) SortKey {
	value := SortKey(r.URL.Query().Get("sort"))
	for _, key := range sortKeys {
		if key == value {
			return key
		}
	}
	return SortAge
}

// requestedStatus returns the status filter selected on the list view.
func requestedStatus(r *http.Request) string {
	switch value := r.URL.Query().Get("status"); value {
	case "error", "all":
		return value
	default:
		return "all"
	}
}

// writeJSON encodes a document as JSON.
func writeJSON(w http.ResponseWriter, document any) {
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(document)
}
