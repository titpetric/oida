# Using the dashboard

The dashboard is available under `Options.Path`, which defaults to
`/debug/oida`.

## Views

| Path | Contents |
| --- | --- |
| `/` | Hosts seen by the process, request counts, retained traces, failures, routes, and timing |
| `/traces` | Retained traces with filters and sorting |
| `/live` | Running and recently completed traces |
| `/stats` | Rolling statistics grouped by host and route |
| `/trace/{id}` | One trace with its spans, attributes, errors, request details, and memory use |

The landing page helps identify which host needs attention. Selecting a host
keeps the trace list and live view focused on that host.

## Trace filters

The `/traces` view accepts:

| Parameter | Values | Default | Meaning |
| --- | --- | --- | --- |
| `limit` | `20`, `50`, `100`, `200` | `20` | Maximum rows shown |
| `q` | text | empty | Match trace name or ID |
| `kind` | a span kind | empty | Keep traces containing that kind |
| `host` | hostname | empty | Keep traces from that host |
| `status` | `all`, `error` | `all` | Show every trace or failures only |
| `sort` | `age`, `duration`, `spans`, `allocated` | `age` | Sort column |
| `order` | `asc` | descending | Reverse the selected sort |

Unknown values fall back to the default.

Examples:

```text
/debug/oida/traces?q=invoice
/debug/oida/traces?kind=database&status=error
/debug/oida/traces?host=api.example&sort=duration
```

## Live activity

The live view shows running and recently completed traces in one feed. Running
traces show their current state and elapsed time. Set `Options.LiveStream` to
false to use periodic refreshes instead. `Options.RefreshInterval` controls the
interval; zero disables periodic refreshes.

## Trace details

A trace detail page shows:

- Trace ID, name, status, duration, service, and start time
- The timing and overlap of recorded spans
- Time grouped by span kind
- The span tree with names, offsets, durations, source locations, and errors
- Span attributes, including statements recorded as `query`, `sql`,
  `statement`, or `cql`
- HTTP request and response details when the trace came from a request
- Heap delta, allocated bytes, allocations, GC cycles, and GC pause

Memory values are process-wide observations. Concurrent traces can overlap, so
use them to identify trends rather than as isolated accounting.

## Statistics

Statistics group HTTP traces by host and routed pattern. With route patterns
configured, requests such as `/users/1` and `/users/2` appear under one
`GET /users/{id}` row.

Each row includes count, failures, traffic share, average and maximum duration,
average response bytes, average allocated bytes, and average span count. The
number of rows is controlled by `Options.TopRequests`.

## JSON and plain text

Use `?format=json` or request `Accept: application/json` for JSON. Use
`?format=text`, request `Accept: text/plain`, or call the endpoint with `curl`
for plain text.

JSON responses are specific to each view:

| Path | JSON value |
| --- | --- |
| `/` | `[]HostStat` |
| `/traces` | `[]Trace` after filtering, sorting, and limiting |
| `/live` | `[]Trace` containing traces currently in flight |
| `/stats` | `Stats` |
| `/trace/{id}` | `Trace` |

Examples:

```bash
curl -s 'http://localhost:8080/debug/oida/traces?format=json' | jq '.[0]'
curl -s 'http://localhost:8080/debug/oida/stats?format=json' | jq '.top'
curl -s 'http://localhost:8080/debug/oida/traces?format=text'
```

## Access control

The dashboard can expose request paths, user agents, errors, and span
attributes. Keep it on an internal listener or set `Options.Authorize` before
mounting it on a public listener. A rejected request receives a 404 response.

```go
opts.Authorize = func(r *http.Request) bool {
	return allowed(r)
}
```
