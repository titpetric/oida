# Instrumentation guide

## 1. The rule

```go
ctx, span := oida.Start(ctx, "what happened", oida.KindDatabase)
defer span.End()
```

Take the returned `ctx` and pass it down. That is what makes the next `Start` a child instead of a sibling. If you drop the context, the span still records; it just lands flat at the parent's depth.

Everything is nil-safe. A package instrumented with oida works in a process that never configured a tracer, in unit tests, and in requests that were not sampled.

## 2. Naming

Span names are shown verbatim and grouped by nothing, so keep them stable and low-cardinality:

| Good                  | Bad                                    | Why                                              |
|-----------------------|----------------------------------------|--------------------------------------------------|
| `SELECT users`        | `SELECT * FROM users WHERE id = 4711`  | the query is an attribute, not an identity       |
| `GET billing-api`     | `GET https://billing/v1/invoices/4711` | put the URL in an attribute                      |
| `render invoice.html` | `render`                               | a bare verb tells you nothing in a 40-span trace |

Attributes carry the specifics:

```go
span.SetAttribute("query", query)
span.SetAttribute("args", len(args))
span.SetAttribute("rows", rows)
```

`StartAuto` derives the name from a symbol, which keeps it in step with a rename and costs nothing to type:

```go
func (s *UserStore) GetUsers(ctx context.Context) ([]User, error) {
	ctx, span := oida.StartAuto(ctx, s.GetUsers)   // billing.UserStore.GetUsers
	defer span.End()
```

The name is read through reflection and the runtime symbol table, so it does not survive a stripped binary and reads oddly for an anonymous function. Use `Start` with a literal where either matters.

## 3. Kinds

Pass a `Kind` as the third argument. It drives the colour in the timeline and the grouping of the segment sweep, so a trace reads as "40% database, 30% external, 20% template" at a glance.

| Kind           | Use for                                                     |
|----------------|-------------------------------------------------------------|
| `KindHTTP`     | The inbound request span; the middleware creates it for you |
| `KindDatabase` | SQL, Redis-as-store, any query against your own data        |
| `KindExternal` | Outbound calls to services you do not own                   |
| `KindTemplate` | Rendering, serialization, response construction             |
| `KindCache`    | Cache get/set where a miss falls through to another span    |
| `KindQueue`    | Publishing to or consuming from a queue                     |
| `KindInternal` | Everything else, and the default                            |

The zero value is `KindInternal`, so `oida.Start(ctx, "compute")` is fine.

## 4. Errors

```go
if err != nil {
	span.RecordError(err)
	return err
}
```

`RecordError` marks the span *and* the trace as failed, which is what the `?status=error` filter and the `Errors` column in statistics use. It ignores nil errors, so this is also valid:

```go
defer func() { span.EndWithError(err) }() // named return
```

Sentinel errors that are expected control flow (`sql.ErrNoRows`, `context.Canceled`) are usually not worth recording as failures:

```go
if err != nil && !errors.Is(err, sql.ErrNoRows) {
	span.RecordError(err)
}
```

Code that does not hold the span, such as an error path several calls below the one that started it, calls the package function of the same name and records through the context:

```go
oida.RecordError(ctx, err)
```

It finds the innermost span in `ctx` and does nothing when there is none.

## 5. Patterns

### 5.1 Wrapping a function

```go
err := oida.Do(ctx, "reindex", func(ctx context.Context) error {
	return index.Rebuild(ctx)
}, oida.KindInternal)
```

`Do` starts the span, runs the function, records the error, ends the span and returns the error unchanged.

### 5.2 database/sql

Wrap at the repository boundary, not per row:

```go
func (r *Repo) Users(ctx context.Context, limit int) ([]User, error) {
	ctx, span := oida.Start(ctx, "SELECT users", oida.KindDatabase)
	defer span.End()
	span.SetAttribute("limit", limit)

	rows, err := r.db.QueryContext(ctx, selectUsers, limit)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	defer rows.Close()

	users, err := scanUsers(rows)
	span.SetAttribute("rows", len(users))
	if err != nil {
		span.RecordError(err)
	}
	return users, err
}
```

A `sql.Tx` is one span with the statements nested inside it:

```go
ctx, tx := oida.Start(ctx, "tx: create invoice", oida.KindDatabase)
defer tx.End()
tx.SetAttribute("isolation", "serializable")
```

### 5.3 Outbound HTTP

A `RoundTripper` records every outbound call with no change at the call sites:

```go
type Transport struct{ Base http.RoundTripper }

func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	ctx, span := oida.Start(r.Context(), r.Method+" "+r.URL.Host, oida.KindExternal)
	defer span.End()
	span.SetAttribute("url", r.URL.Redacted())

	res, err := base.RoundTrip(r.WithContext(ctx))
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	span.SetAttribute("status", res.StatusCode)
	return res, nil
}
```

Use `r.URL.Redacted()`: it strips userinfo credentials.

### 5.4 Cache lookups

```go
parent := ctx
_, span := oida.Start(parent, "cache: user", oida.KindCache)
value, hit := c.Get(key)
span.SetAttribute("hit", hit)
span.End()

if !hit {
	value, err = loadUser(parent, key)
}
```

End the cache span before the fallback so the timeline shows cache and database as separate sibling segments rather than nested.

### 5.5 Concurrent work

Spans started from goroutines are safe; the parent is whatever context you hand to the goroutine:

```go
ctx, span := oida.Start(ctx, "fan out", oida.KindInternal)
defer span.End()

g, gctx := errgroup.WithContext(ctx)
for _, id := range ids {
	g.Go(func() error {
		ctx, s := oida.Start(gctx, "load user", oida.KindDatabase)
		defer s.End()
		return load(ctx, id)
	})
}
return g.Wait()
```

Concurrent siblings overlap in the timeline; the segment sweep attributes each slice of wall-clock to the innermost span that was active, so overlapping work does not double-count.

### 5.6 Source locations

```go
span.SetSource("internal/billing/repo.go", 42)
```

Optional, and nothing in oida sets it for you: the file and line are whatever the caller passes, normally the repo-relative path and the line of the `Start` call. The span table gains a Source column showing `file:L42` when a trace has any, and omits the column when it has none. Do not compute it with `runtime.Caller` on hot paths; it is not free.

## 6. Trace-level operations

```go
trace := oida.TraceFromContext(ctx)
trace.SetState(oida.StateProcessing)
trace.RecordError(err)
id := oida.TraceID(ctx) // the ULID, also in the Request-Id header
```

`RecordError` and `Err` mean on a trace what they mean on a span, and a span records on both: an error recorded anywhere in a transaction is readable from the transaction. The value is what comes back, so `errors.Is` still works on it.

Putting the trace ID into your logs is the cheapest possible correlation:

```go
slog.ErrorContext(ctx, "charge failed", "err", err, "trace", oida.TraceID(ctx))
```

A trace carries attributes of its own, for what holds for the whole transaction rather than for one operation inside it:

```go
trace.SetAttribute(oida.AttrMemoryLimit, limit)          // bytes, int64
trace.SetAttributes(oida.Attributes{"tenant": tenantID}) // several at once
```

The detail view renders them as a table beside the request, keys read as labels, and byte-valued keys read as sizes.

### 6.1 Memory

Two keys are known to the front end:

| Key            | On             | Meaning                                  |
|----------------|----------------|------------------------------------------|
| `memory_limit` | trace          | the ceiling the transaction ran under    |
| `memory_usage` | trace and span | memory in use when it finished, in bytes |

Recorded per span, `memory_usage` is the memory curve of the request: the span table draws each reading against the largest one in the trace, and the span where the curve steps is the span that allocated.

```go
span.SetAttribute(oida.AttrMemoryUsage, runtimeUsage())
span.End()
```

This is worth recording when the runtime can charge allocations to one request, which an interpreter can and Go cannot: in Go the heap belongs to the process, and what oida can say about it is in `Trace.Memory` instead. Both keys are optional and independent: a runtime that knows the usage but not the limit records the usage.

### 6.2 Logging

`Info` and `Error` record log lines on the trace, shown on the detail page in a Log tab next to the spans:

```go
span.Info("cache miss", "key", key)         // linked to this span
trace.Error("charge failed", "invoice", id) // linked to the innermost open span
```

Args are slog-style key/value pairs, stored on the entry as attributes; the message is stored verbatim, never formatted. A key that is not a string is rendered with `fmt.Sprint`, and a dangling final argument lands under `!BADKEY`, the way `log/slog` keeps it.

No context is involved. A span logs under its own id; a trace resolves the active span itself. `Current()` returns the innermost span that has not ended, the way `Root()` returns the first recorded one. Code that holds a context rather than the trace reaches the active span with `oida.SpanFromContext(ctx)` and logs on it directly.

The entries live in one slice, `Trace.Logs`, each carrying its level, its offset on the trace clock, the request ID and the ID of the span it was written under. `MaxSpansPerTrace` bounds them the way it bounds the spans: excess entries are dropped and counted in `Trace.DroppedLogs`, and nothing is printed. Besides the Log tab they are in the plain text detail and in the trace JSON under `logs`.

`Options.CaptureLogs` gates the recording, and `NewOptions` turns it on. Disabled, `Info` does nothing, and `Error` records its text as `message key=value` through `RecordError` on the active span, so the failure still shows. Enabled, `Error` only logs: marking the transaction as failed stays with `RecordError`.

## 7. What not to instrument

- Loops with thousands of iterations. Span one loop, attribute the count. `MaxSpansPerTrace` will otherwise drop the tail and `DroppedSpans` will grow.
- Functions that take nanoseconds. The span costs more than the work.
- Anything holding a lock you also need for the work: `SetAttribute` takes the span mutex, so build the value first, then set it once.

## 8. Testing instrumented code

```go
func TestRepoSpans(t *testing.T) {
	tracer, err := oida.New(oida.NewOptions("test"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, trace, err := tracer.StartTrace(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = repo.Users(ctx, 10)
	tracer.Finish(trace)

	got := tracer.Traces()
	if len(got) != 1 || got[0].SpanCount() != 2 {
		t.Fatalf("unexpected spans: %+v", got)
	}
}
```

Each test builds its own tracer, so parallel packages do not share a ring buffer. Set `opts.ReadEnv = false` to keep the environment of whoever runs the suite out of it.
