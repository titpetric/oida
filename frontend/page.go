package frontend

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/titpetric/oida"
)

// Page is the view model handed to every templ component. Components take
// nothing else, so they can be rendered in tests from a hand built page.
type Page struct {
	Snapshot oida.Snapshot
	View     View
	Path     string
	Title    string
	Limit    int
	Query    string
	Kind     oida.Kind

	// Host is the domain filter, empty for every host. RequestHost is the
	// domain this dashboard was reached on, which is the sensible default
	// label when no filter is set.
	Host        string
	RequestHost string

	Status    string
	Sort      SortKey
	Ascending bool
	Refresh   int
	Stream    bool
	Slowest   time.Duration

	// Feed is the live view: traces in flight and completed traces in one
	// stream, newest first.
	Feed []oida.Trace

	Trace    *oida.Trace
	Rows     []SpanRow
	Segments []Segment
}

// limitOptions are the row counts offered on the trace list.
var limitOptions = []int{20, 50, 100, 200}

// LimitOptions returns the selectable row counts.
func (p Page) LimitOptions() []int {
	return limitOptions
}

// URL builds a link to a view of the front end, preserving the active filters.
func (p Page) URL(view View) string {
	path := p.Path
	switch view {
	case ViewList:
		path += "/traces"
	case ViewLive:
		path += "/live"
	case ViewStats:
		path += "/stats"
	}

	query := url.Values{}

	// The host filter follows the reader between views: switching to the feed
	// while looking at one domain should not silently widen to all of them.
	if p.Host != "" && (view == ViewList || view == ViewLive) {
		query.Set("host", p.Host)
	}

	if view == ViewList {
		if p.Limit > 0 && p.Limit != limitOptions[0] {
			query.Set("limit", strconv.Itoa(p.Limit))
		}
		if p.Query != "" {
			query.Set("q", p.Query)
		}
		if p.Kind != "" {
			query.Set("kind", string(p.Kind))
		}
		if p.Status != "" && p.Status != "all" {
			query.Set("status", p.Status)
		}
		if p.Sort != "" && p.Sort != SortAge {
			query.Set("sort", string(p.Sort))
		}
		if p.Ascending {
			query.Set("order", "asc")
		}
	}
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

// LimitURL builds a link to the trace list with a different row count.
func (p Page) LimitURL(limit int) string {
	page := p
	page.Limit = limit
	return page.URL(ViewList)
}

// StatusURL builds a link to the trace list with a different status filter.
func (p Page) StatusURL(status string) string {
	page := p
	page.Status = status
	return page.URL(ViewList)
}

// HostURL builds a link to the trace list filtered to one host.
func (p Page) HostURL(host string) string {
	page := p
	page.Host = host
	return page.URL(ViewList)
}

// HostLiveURL builds a link to the live feed of one host.
func (p Page) HostLiveURL(host string) string {
	page := p
	page.Host = host
	return page.URL(ViewLive)
}

// SortURL builds a link that sorts by key. Clicking the active column flips
// the direction; clicking another column starts it at the useful end, which
// for every one of these is largest first.
func (p Page) SortURL(key SortKey) string {
	page := p
	page.Sort = key
	page.Ascending = p.Sort == key && !p.Ascending
	return page.URL(ViewList)
}

// SortClass marks the active sort column for the header link.
func (p Page) SortClass(key SortKey) string {
	if p.Sort != key {
		return "sort"
	}
	if p.Ascending {
		return "sort active asc"
	}
	return "sort active desc"
}

// Hosts returns the hosts seen by the process, for the filter control.
func (p Page) Hosts() []oida.HostStat {
	return p.Snapshot.Statistics.Hosts
}

// Fields returns the filter bar controls in display order. The domain is not
// among them: it is chosen once in the masthead and every view inherits it.
func (p Page) Fields() []SelectField {
	return []SelectField{p.KindField(), p.StatusField(), p.RowsField()}
}

// KindField filters the list by the span kinds a trace recorded.
func (p Page) KindField() SelectField {
	options := []SelectOption{{Value: "", Label: "every kind"}}
	for _, kind := range listKinds {
		options = append(options, SelectOption{Value: string(kind), Label: string(kind)})
	}
	return SelectField{
		ID:      "oida-kind",
		Name:    "kind",
		Label:   "Kind",
		Value:   string(p.Kind),
		Options: options,
	}
}

// StatusField filters the list down to failures.
func (p Page) StatusField() SelectField {
	return SelectField{
		ID:    "oida-status",
		Name:  "status",
		Label: "Status",
		Value: p.Status,
		Options: []SelectOption{
			{Value: "all", Label: "every status"},
			{Value: "error", Label: "failed only"},
		},
	}
}

// RowsField picks how many rows the list shows.
func (p Page) RowsField() SelectField {
	options := make([]SelectOption, 0, len(limitOptions))
	for _, limit := range limitOptions {
		options = append(options, SelectOption{
			Value: strconv.Itoa(limit),
			Label: strconv.Itoa(limit),
			Note:  "rows",
		})
	}
	return SelectField{
		ID:      "oida-limit",
		Name:    "limit",
		Label:   "Rows",
		Value:   strconv.Itoa(p.Limit),
		Options: options,
	}
}

// Domain is the label of the switcher in the masthead. Unfiltered means every
// host, not the one this dashboard happens to be reached on: the views below
// show all of them combined.
func (p Page) Domain() string {
	if p.Host != "" {
		return p.Host
	}
	return allHosts
}

// allHosts is the label of the unfiltered state.
const allHosts = "all hosts"

// Filtered reports whether the view is narrowed to one host.
func (p Page) Filtered() bool {
	return p.Host != ""
}

// SwitchHostURL keeps the reader on the current view and changes the domain.
func (p Page) SwitchHostURL(host string) string {
	page := p
	page.Host = host

	// Picking a domain from the landing page, a trace or the statistics means
	// "show me this domain's traces". From the feed it means "keep watching,
	// but this domain".
	view := p.View
	if view != ViewLive {
		view = ViewList
	}
	return page.URL(view)
}

// TracesPath is where the filter form submits. It is a path with no query,
// because a GET form replaces the query with its own fields.
func (p Page) TracesPath() string {
	return p.Path + "/traces"
}

// TraceURL builds a link to the detail view of a trace.
func (p Page) TraceURL(id string) string {
	return p.Path + "/trace/" + url.PathEscape(id)
}

// AssetURL links a file in the embedded asset tree.
func (p Page) AssetURL(name string) string {
	return p.Path + assetPrefix + name
}

// CSSURL is the link to the embedded stylesheet.
func (p Page) CSSURL() string {
	return p.AssetURL("oida.css")
}

// JSURL is the link to the embedded live view script.
func (p Page) JSURL() string {
	return p.AssetURL("oida.js")
}

// EventsURL is the link to the live view event stream.
func (p Page) EventsURL() string {
	return p.Path + "/live/events"
}

// Active reports whether view is the rendered one.
func (p Page) Active(view View) bool {
	return p.View == view
}

// Service returns the configured service name, or a placeholder.
func (p Page) Service() string {
	if p.Snapshot.Service == "" {
		return "unnamed service"
	}
	return p.Snapshot.Service
}

// DurationShare returns the duration of a trace relative to the slowest trace
// on the page.
func (p Page) DurationShare(trace oida.Trace) float64 {
	return share(trace.Duration, p.Slowest)
}

// Composition returns the timeline of a trace scaled to the slowest trace on
// the page, so one row of bars compares traces against each other rather than
// against themselves. The shape of a request — how much of it was database,
// how much was waiting on someone else — is the thing worth seeing in a list.
func (p Page) Composition(trace oida.Trace) []Segment {
	segments := Timeline(trace)
	if p.Slowest <= 0 || trace.Duration <= 0 {
		return segments
	}

	scale := float64(trace.Duration) / float64(p.Slowest)
	for i := range segments {
		segments[i].OffsetShare *= scale
		segments[i].Share *= scale
	}
	return segments
}

// Now returns the moment the snapshot was taken.
func (p Page) Now() time.Time {
	if p.Snapshot.StartedAt.IsZero() {
		return time.Time{}
	}
	return p.Snapshot.StartedAt.Add(p.Snapshot.Uptime)
}

// Age renders how long ago a timestamp is, relative to the snapshot.
func (p Page) Age(at time.Time) string {
	return ageText(at, p.Now())
}

// FormatURL links the current view in another representation, so the JSON and
// plain text renderings are discoverable from the page itself.
func (p Page) FormatURL(format string) string {
	base := p.URL(p.View)
	if p.View == ViewDetail && p.Trace != nil {
		base = p.TraceURL(p.Trace.ID)
	}
	if strings.Contains(base, "?") {
		return base + "&format=" + format
	}
	return base + "?format=" + format
}

// WaveSpans returns every span of the trace in the shape the drawing fills:
// where it ran as a fraction of the trace, and how deep it sat.
//
// The legend attributes each moment to the innermost span running, which is the
// right way to answer where the time went, and the wrong way to draw the stack:
// a request that spends its life inside three queries reports as three queries,
// and the handler and the request holding them never appear. The drawing wants
// the opposite, so it gets the spans whole. Parents included, the root
// included: at any moment, everything that was running.
func (p Page) WaveSpans() []WaveSpan {
	spans := make([]WaveSpan, 0, len(p.Rows))
	for _, row := range p.Rows {
		if row.Duration <= 0 {
			continue
		}
		spans = append(spans, WaveSpan{
			Name:   row.Name,
			Kind:   row.Kind.String(),
			Color:  row.Kind.Color(),
			Start:  row.OffsetShare / 100,
			End:    min((row.OffsetShare+row.Share)/100, 1),
			Depth:  row.Depth,
			Failed: row.Error != "",
		})
	}
	return spans
}

// WaveTrace is the span data plus the length and the depth of the trace, which
// is what the drawing scales itself to.
func (p Page) WaveTrace() WaveTrace {
	trace := WaveTrace{Spans: p.WaveSpans()}
	if p.Trace != nil {
		trace.Milliseconds = float64(p.Trace.Duration) / float64(time.Millisecond)
	}
	for _, span := range trace.Spans {
		trace.Depth = max(trace.Depth, span.Depth)
	}
	return trace
}

// KindShares totals the timeline by kind, newest share first, for the legend.
func (p Page) KindShares() []Segment {
	shares := make(map[oida.Kind]*Segment, len(p.Segments))
	order := make([]oida.Kind, 0, len(p.Segments))
	for _, segment := range p.Segments {
		total := shares[segment.Kind]
		if total == nil {
			total = &Segment{Kind: segment.Kind}
			shares[segment.Kind] = total
			order = append(order, segment.Kind)
		}
		total.Duration += segment.Duration
		total.Share += segment.Share
	}

	out := make([]Segment, 0, len(order))
	for _, kind := range order {
		out = append(out, *shares[kind])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Share > out[j].Share })
	return out
}

// Ticks returns the labelled steps of the detail view time axis.
func (p Page) Ticks() []AxisTick {
	if p.Trace == nil {
		return nil
	}
	return axisTicks(p.Trace.Duration)
}
