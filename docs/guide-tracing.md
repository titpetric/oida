# Distributed tracing

A trace is one piece of work seen end to end. A transaction is what one process recorded of it: a request, a run, a pipeline step, a browser pageload. oida records transactions, and W3C Trace Context is what makes several of them one trace.

Four fields of `model.Trace` carry it:

| Field          | Shape            | What it is                                                         |
|----------------|------------------|--------------------------------------------------------------------|
| `TraceID`      | 32 lowercase hex | The trace every transaction of one piece of work shares            |
| `SpanID`       | 16 lowercase hex | This transaction's own span, which a caller names to nest under it |
| `ParentSpanID` | 16 lowercase hex | The span that called this transaction, empty for one nobody called |
| `TraceState`   | key=value list   | Vendor state, carried through untouched                            |

`Trace.ID` stays what it was, a ULID, and stays the `Request-Id` header. It names the transaction, not the trace. A transaction that nobody called takes the hex form of its own ULID as its `TraceID`, so every trace has one whether or not anything joined it.

## Over HTTP

The middleware always writes `traceparent` on the response, naming the trace and the transaction's span:

```
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-7a3f0b19cf4d2e68-01
```

Reading the header is opt in, the way `TrustRequestID` is:

```go
opts.TrustTraceContext = true
```

With it on, a request carrying a valid `traceparent` records under that trace with the caller's span as its parent, and its `tracestate` travels on untouched. A header that does not parse restarts the trace and drops the state, which is what the specification asks of a receiver. Version `00` is read in full; a later version is read as its first three fields, which the version format reserves for exactly that, and the fields after them are dropped.

Propagating outward is four calls:

```go
oida.Propagate(ctx)   // the TraceContext of the innermost span
oida.TraceParent(ctx) // that context as a header value
oida.Inject(ctx, request.Header)
oida.Environ(ctx) // TRACEPARENT and TRACESTATE for a child process
```

```go
request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
oida.Inject(ctx, request.Header)
```

## Over a process boundary

A run, a pipeline step and a cron tick have no HTTP boundary. The environment carries the context instead:

```go
cmd := exec.CommandContext(ctx, "atkins", "build")
cmd.Env = append(os.Environ(), oida.Environ(ctx)...)
```

`Environ` writes `TRACEPARENT`, and `TRACESTATE` when there is state to carry. The names have no `OIDA_` prefix on purpose: a parent process does not know which library its child was built with, and the bare pair is the convention the OpenTelemetry tooling settled on.

The child reads them in `oida.New`, through `Options.TraceParent` and `Options.TraceState`, and every trace `StartTrace` and `Observe` open joins that context:

```go
tracer, err := oida.New(oida.NewOptions("atkins"))
if err != nil {
	return err
}

err = tracer.Observe(ctx, "docker build", func(ctx context.Context) error {
	return build(ctx)
})
```

The outer process recorded `atkins build`; this one recorded `docker build` under it, with the same `TraceID`. A value that does not parse fails `New` rather than being dropped, so a broken variable is a startup error and not a silently orphaned trace.

## Into a browser

Render the context into the page and the browser's pageload nests under the request that served it:

```go
func handler(w http.ResponseWriter, r *http.Request) {
	view.Render(w, view.Page{TraceParent: oida.TraceParent(r.Context())})
}
```

```html
<meta name="traceparent" content="{{ .TraceParent }}">
<script src="/debug/oida/assets/oida-trace.js"></script>
```

[The browser library](guide-browser.md) is the other end of that.

## From a client

A client posts what it measured to `{Path}/collect`, which `Options.Collect` turns on:

```json
{"transactions": [{
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "aaaaaaaaaaaaaa03",
  "parent_id": "aaaaaaaaaaaaaa02",
  "name": "button event=click",
  "kind": "browser",
  "started_at": 1700000000000,
  "duration_ms": 30,
  "attributes": {"event": "click", "label": "Run tests", "data-job": "go:test"},
  "spans": [{"name": "fetch /api/runs", "kind": "http", "start_ms": 2, "duration_ms": 24}]
}]}
```

The answer names each transaction, so the client can nest its next one and its next call below it:

```json
{"transactions": [{
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "aaaaaaaaaaaaaa03",
  "id": "01K3XJ7N2M8QWERTYUIOPASDFG",
  "traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-aaaaaaaaaaaaaa03-01"
}]}
```

A client names its own span because a `parent-id` names a span inside the trace, not inside the transaction: a sequence number would have every transaction of a trace claiming span one. `model.SpanIDFor` derives a server-side span the same way, from the trace's ULID and the span's sequence number.

`Tracer.Record` and `Tracer.Collect` are the same path in Go, for a process reporting work it measured elsewhere.

## In the dashboard

A trace detail page carries a **Trace** section: the trace identifier, this transaction's span, the transaction that called it as a link, and every other transaction of the same trace, oldest first. That is the one view where a click, the request it caused and the run that request started read as one chain.

The log is what one process retained, so a trace spanning two processes reads whole where both record into the same storage. Point them at one disk folder with `OIDA_STORAGE_DRIVER=disk` and `OIDA_STORAGE_DISK_PATH`.

## What this follows

[W3C Trace Context](https://www.w3.org/TR/trace-context/), the Recommendation of 23 November 2021: the `traceparent` and `tracestate` header names in lowercase, the 32 and 16 hex digit identifiers, the all-zero identifiers refused, version `ff` refused, the `sampled` bit at `0x01`, and the version format's rule that a later version is read as its first three fields.

`tracestate` is carried through rather than written to: at most 32 list members, in arrival order, with a malformed member dropped instead of the header failing. oida adds no entry of its own, so nothing has to be moved to the front.

Not implemented: [W3C Baggage](https://www.w3.org/TR/baggage/), which is a Candidate Recommendation and carries application data rather than trace identity, and the `b3` single header [Zipkin](https://github.com/openzipkin/b3-propagation) propagates with.

---

Previous: [Instrumentation](guide-instrumentation.md) · Next: [The browser library](guide-browser.md) · [Contents](README.md)
