# Specification: front end

The UI is server-side rendered with [templ](https://templ.guide). Components are
compiled into the `oida` package (`view_*.templ` → `view_*_templ.go`), so there
is no template parsing at runtime, no `FuncMap`, and no external asset hosting.

## 1. Routes

All routes are relative to `Options.Path` (default `/debug/oida`). The handler
strips the mount prefix itself, so the table holds for `r.Mount("/admin/oida",
…)` too.

| Route | View | Contents |
| --- | --- | --- |
| `/` | list | Retained traces, newest first |
| `/live` | live | One feed of running and completed traces, streaming |
| `/live/events` | — | Server sent event stream of the live section |
| `/stats` | stats | Rolling statistics over the retention window |
| `/trace/{id}` | detail | One trace: timeline, span tree, attributes, memory |
| `/assets/*` | asset | The embedded `public/` tree, `Cache-Control: max-age=3600` |
| anything else | — | 404 |

Assets live in `public/assets/`, embedded with `//go:embed all:public` and
served through `http.FileServerFS` over an `fs.Sub` of that tree. Adding a file
to `public/assets/` serves it at `{Path}/assets/<name>` with no further wiring;
content types and traversal are the file server's problem, not ours.

Query parameters on the list view:

| Parameter | Values | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | 20, 50, 100, 200 | 20 | Rows shown |
| `q` | free text | — | Case-insensitive substring filter on trace name and ID |
| `kind` | a `Kind` value | — | Only traces containing a span of that kind |
| `status` | `error`, `all` | `all` | `error` keeps only failed traces |
| `format` | `html`, `json`, `text` | negotiated | Overrides content negotiation |
| `stream` | `off` | — | Renders the live view static, without opening an event stream |

Unknown or out-of-range values fall back to the default; the handler never
errors on bad query input.

## 2. Content negotiation

Identical to the read path of the original status page:

| Condition on the request | Response |
| --- | --- |
| `Accept` contains `application/json` or `text/json` | `text/json; charset=utf-8`, the `Snapshot` (or the single `Trace` on the detail route) |
| `Accept` contains `text/plain`, or `User-Agent` starts with `curl/` | `text/plain; charset=utf-8`, fixed-width table |
| otherwise | `text/html; charset=utf-8`, rendered templ component |

The `?format=json`, `?format=text` and `?format=html` query parameters override
negotiation, which is what tests use.

## 3. Components

```
view_layout.templ   Layout(page Page)             <html>, <head>, chrome, children
                    pageHeader(page Page)         service identity and process facts
                    pageMetrics(page Page)        uptime / traces / heap / GC / pool tiles
                    pageNav(page Page)            tab bar with the active tab marked
                    pageFooter(page Page)         caveats and the dropped-trace count
                    stateBar(d []StateDuration)   lifetime time-in-state bar + legend
view_list.templ     List(page Page)               recorded trace table + filter bar
                    traceRow(page Page, t Trace)  one row with a proportional bar
view_live.templ     Live(page Page)               live wrapper + stream script
                    liveSection(page Page)        what the event stream replaces
view_stats.templ    Statistics(page Page)         top-N table with share meters
view_detail.templ   Detail(page Page)             trace header, drawing, span tree
                    traceFacts(page, trace)       request and system, label and value
                    timelineFoot(page Page)       the time axis under the drawing
                    timelineLegend(page Page)     exclusive time per kind
                    peekToggle(name, n, label)    the switch that folds a table away
                    spanRow(row SpanRow)          indented span row with attributes
view_waves.templ    waves(page Page)              the trace as overlaid waveforms
```

The detail view draws a trace one way: `waves`, a canvas filled by
`public/assets/waves.js` from the `WaveTrace` payload the page embeds. There is
no switch and no `?timeline=` parameter. `waves.js` is served only on the detail
view, so the list and the live feed do not carry it.

The span table under the drawing is folded away behind `peekToggle`: the
stylesheet does the folding with `:has()` and a grid row from `0fr` to `1fr`, so
it works with scripting off, and `oida.js` only remembers the choice in
`localStorage` under `oida.peek.spans` so it holds across every trace opened.
The legend sits in the bar above the fold and is always shown, because it is the
answer and the spans are the working.

The component is named `Statistics`, not `Stats`, because `Stats` is the data
model it renders.

Every component takes the single `Page` view model (`page.go`); components never
reach for a tracer, a request, or global state, which keeps them renderable in
tests with a hand-built `Page`.

```go
type Page struct {
    Snapshot Snapshot
    View     View          // "list" | "live" | "stats" | "detail"
    Path     string        // mount path, used to build links
    Title    string
    Limit    int
    Query    string
    Kind     Kind
    Status   string
    Refresh  int           // seconds; 0 disables the refresh fallback
    Stream   bool          // live view streams over SSE
    Trace    *Trace        // detail view only
    Rows     []SpanRow     // detail view only
    Segments []Segment     // detail view only
}
```

`Page` carries link builders so components never construct URLs by hand:
`URL(View)`, `LimitURL(int)`, `StatusURL(string)`, `TraceURL(id)`, `CSSURL()`,
`JSURL()`, `EventsURL()`, plus `Active(View)`, `Service()`, `Slowest()` and
`DurationShare(Trace)` for the proportional bars.

`SpanRow` is the flattened render model for one span:

```go
type SpanRow struct {
    Span
    Offset      time.Duration // start relative to the trace start
    OffsetShare float64       // percentage, for the inline bar
    Share       float64       // duration as percentage of the trace
    Open        bool          // never ended
    Last        bool          // last child at its depth, for tree glyphs
}
```

## 4. Layout

```
┌───────────────────────────────────────────────────────────────────────┐
│ oida · <service>                                    <pid> · <go ver>  │
│ ┌──────────┬──────────┬──────────┬──────────┬───────────────────────┐ │
│ │ Uptime   │ Traces   │ Heap     │ GC       │ Pool estimate         │ │
│ └──────────┴──────────┴──────────┴──────────┴───────────────────────┘ │
│ [ Traces ] [ Live ] [ Statistics ]                                    │
├───────────────────────────────────────────────────────────────────────┤
│ view body                                                             │
└───────────────────────────────────────────────────────────────────────┘
```

### 4.1 List view

Columns: ID (link to detail), time, name, status, duration (with a proportional
bar relative to the slowest trace in the window), spans, bytes, heap delta,
allocated, remote. Failed traces get the `error` row class. Empty state: *"No
traces recorded yet."*

### 4.2 Detail view

1. **Header** — trace name, ID, HTTP status, duration, service, timestamp.
2. **Transaction** — the drawing: a canvas carrying every span as a waveform
   along the stretch it ran for, over a time axis. The payload is `WaveTrace`,
   which is the spans whole, root and parents included, because the drawing is
   of what was open. The legend below it is the other question and comes from
   `Timeline(trace)`: exclusive time per kind.
3. **Spans** — folded away behind the legend bar, a table of `SpanRow`,
   indented by depth with a tree glyph, each
   row showing kind badge, offset, duration, an inline proportional bar, source
   location and name. Errors render as a red block.

   Attributes fold into a `<details>` whose summary is the key list,
   `( attributes: args, query )`, so a span row reads as a name and what is
   known about it rather than a wall of chips. One exception: a statement
   attribute (`query`, `sql`, `statement`, `cql`) is collapsed onto one line and
   printed in a `<code>` block on the row itself, because a database span is
   mostly its query.
4. **Request and system** — two tables side by side. What was asked for on the
   left, and on the right what it cost this process: service, span count, heap
   delta, allocated bytes and allocations, GC cycles and pause, with the caveat
   that the memory numbers are process-wide.

### 4.3 Live view

One feed inside `liveSection`, plus the lifetime state bar. Running and
completed traces share a single table, newest first, capped at `liveFeedRows`:
a request appears the moment it starts and stays in place as it finishes,
instead of jumping between an "in flight" table and a "completed" one. Requests
that take milliseconds would otherwise never be seen at all.

Running rows are tinted, carry their scoreboard state in place of an HTTP
status, and show elapsed time so far. `Trace.InFlight` carries the distinction;
`Trace.Clone` sets it, so it holds for JSON consumers too. New traces arrive
over the event stream, see [§7](#7-live-streaming).

Without scripting, the page falls back to
`<noscript><meta http-equiv="refresh" content="{Options.RefreshInterval}"></noscript>`,
and with `Options.LiveStream` off the meta refresh moves out of the `noscript`
and becomes the only mechanism.

### 4.4 Statistics view

Top-N table: share meter, count, errors, name, host, average duration, max
duration, average bytes, average allocated, average spans. Header states the
window: *"Top 20 of 137 traces in a 200-trace window."*

### 4.5 Viewports

Three tiers, full width at every one of them. There is no `max-width`: a trace
table is worth every pixel it is given, and the shape column absorbs the slack
so wide screens stretch the data rather than the gutters. What changes between
tiers is how many columns survive.

| Tier | Width | Behaviour |
| --- | --- | --- |
| wide | ≥ 1280px | Every column. Six metric tiles across. |
| medium | 720–1279px | `.c-wide` columns drop: response bytes, remote address, user agent, host, source. Three metric tiles across. |
| narrow | < 720px | `.c-medium` also drops: trace id, span count, allocations. Filters stack one per row, metrics go two across, process facts lose their rules. |

Columns carry a `c-wide` or `c-medium` class on both the `th` and the `td`, so
a dropped column leaves no stray header. Everything is CSS; nothing about the
layout depends on JavaScript.

The root is `font-size: 125%`, so the whole interface scales from one dial and
still honours the reader's own base size. Every other length is in `rem`.

Two columns are exceptions to "the flexible column absorbs the slack":

- `.flex` marks the one column per table that takes the remaining width (shape
  on the feed and the list, name on the span table).
- `.plot` caps the span timeline at `15%`, min 140px, max 300px. Past that the
  extra width buys no precision, and span names want the room.

## 5. Styling

- One embedded stylesheet, served from `{Path}/oida.css` and also inlined into
  `<head>` (via `templ.Raw` over the embedded constant) so the page renders
  standalone when the asset route is blocked.
- System font stack, no web fonts, no external requests of any kind.
- Light and dark via `@media (prefers-color-scheme: dark)` using CSS custom
  properties, so both schemes share one rule set.
- Kind colours are the fixed palette from [spec-model.md](spec-model.md) and are
  emitted as inline `style` attributes (`templ.SafeCSS`) because they are data,
  not theme.
- Tables use `white-space: nowrap` and a horizontal scroll container; the page
  is usable from about 900px wide.

## 6. Security

- `Options.Authorize func(*http.Request) bool` gates every route. When it
  returns false the handler responds 404 with an empty body.
- All dynamic text goes through templ's contextual escaping. The only non-escaped
  values are the generated CSS strings for bar geometry, which are built with
  `fmt.Sprintf` from float64 shares and never from user input.
- Span names and attribute values come from application code, but attribute
  values are rendered with `%v` into escaped text nodes; a hostile value cannot
  break out.
- The endpoint is not mounted by default. Mounting it on a public listener is a
  deliberate act and the getting-started guide says so.

## 7. Live streaming

`GET {Path}/live/events` is a `text/event-stream` that pushes the rendered live
section as traces are recorded. It is push, not poll: the tracer notifies
subscribers, the handler renders, the browser swaps the section in.

```
Browser                      handler                       Tracer
  │  GET /live/events          │                             │
  ├───────────────────────────►│  Subscribe()                │
  │                            ├────────────────────────────►│
  │  data: <live section>      │                             │
  │◄───────────────────────────┤  (initial render)           │
  │                            │                             │
  │                            │◄──── notify() ──── trace starts / finishes
  │                            │  debounce 250ms, drain      │
  │  data: <live section>      │  render liveSection         │
  │◄───────────────────────────┤                             │
  │  : ping (every 20s)        │                             │
```

Mechanics:

| Concern | Behaviour |
| --- | --- |
| Notification | `Tracer.Subscribe()` returns a channel woken by `begin` and `Finish`. Sends are non-blocking on a 1-slot buffer, so recording never waits on a reader and a burst coalesces into one wake-up. |
| Debounce | 250ms quiet period, then pending notifications are drained, so a load spike produces one redraw rather than one per request. |
| Heartbeat | `: ping` comment every 20s, so idle streams survive proxies. |
| Headers | `Cache-Control: no-cache, no-transform`, `Connection: keep-alive`, `X-Accel-Buffering: no` (nginx), and the write deadline is cleared through `http.ResponseController`. |
| Payload | The rendered `liveSection` component, emitted as one `data:` line per line of HTML. The client assigns it to `innerHTML`. |
| Teardown | The handler returns when the request context is cancelled and releases the subscription; the release function is idempotent. |
| Disabled | With `Options.LiveStream` false the route 404s, the script is not loaded, and the meta refresh takes over. |
| Per request | `?stream=off` renders the same markup static. A client that holds the stream open never fires `load`, so screenshot tools, scrapers and proxies that mangle SSE use this. |

`Tracer.Subscribe()` is public, so the same signal can drive a websocket, a
terminal dashboard or a test:

```go
events, cancel := tracer.Subscribe()
defer cancel()
for range events {
	render(tracer.Live())
}
```

### 7.1 Client script

`oida.js` is 30 lines and does exactly one thing: open an `EventSource` on the
URL in `data-events`, assign `event.data` to the section's `innerHTML`, and
update the status pill. There is no framework, no bundler, and no state on the
client — the server still renders every byte of the markup. If the browser has
no `EventSource`, or the asset is blocked, the page still works.

## 8. Generation

```
templ generate
go build ./...
```

Generated files (`view_*_templ.go`) are committed so `go build` works without
the templ CLI. Regenerate whenever a `.templ` file changes. The pipeline does it
for you (`atkins` runs `templ generate` before the tests), and
`atkins templ:verify` fails when the committed output is stale.

Two rules the components must keep, both enforced by `handler_test.go`:

- Styles that carry data (bar geometry, kind colours) are built by `format.go`
  helpers returning `templ.SafeCSS`. A malformed value would be replaced by
  templ's sanitizer with a placeholder, so the tests assert the real values are
  present.
- The stylesheet is inlined through `styleElement()`, a `templ.Raw` over the
  embedded constant. templ treats the body of a literal `<style>` element as
  text, so an expression inside one renders as visible garbage rather than CSS.
