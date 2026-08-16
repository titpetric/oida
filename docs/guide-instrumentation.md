# Instrumentation guide

## 1. The rule

```go
ctx, span := oida.Start(ctx, "what happened", oida.KindDatabase)
defer span.End()
```

Take the returned `ctx` and pass it down. That is what makes the next `Start`
a child instead of a sibling. If you drop the context, the span still records —
it just lands flat at the parent's depth.

Everything is nil-safe. A package instrumented with oida works in a process that
never configured a tracer, in unit tests, and in requests that were not sampled.

## 2. Naming

Span names are shown verbatim and grouped by nothing, so keep them stable and
low-cardinality:

| Good | Bad | Why |
| --- | --- | --- |
| `SELECT users` | `SELECT * FROM users WHERE id = 4711` | the query is an attribute, not an identity |
| `GET billing-api` | `GET https://billing/v1/invoices/4711` | put the URL in an attribute |
| `render invoice.html` | `render` | a bare verb tells you nothing in a 40-span trace |

Attributes carry the specifics:

```go
span.SetAttribute("query", query)
span.SetAttribute("args", len(args))
span.SetAttribute("rows", rows)
```

## 3. Kinds

Pass a `Kind` as the third argument. It drives the colour in the timeline and
the grouping of the segment sweep, so a trace reads as "40% database, 30%
external, 20% template" at a glance.

| Kind | Use for |
| --- | --- |
| `KindHTTP` | The inbound request span — the middleware creates it for you |
| `KindDatabase` | SQL, Redis-as-store, any query against your own data |
| `KindExternal` | Outbound calls to services you do not own |
| `KindTemplate` | Rendering, serialization, response construction |
| `KindCache` | Cache get/set where a miss falls through to another span |
| `KindQueue` | Publishing to or consuming from a queue |
| `KindInternal` | Everything else — the default |

The zero value is `KindInternal`, so `oida.Start(ctx, "compute")` is fine.

## 4. Errors

```go
if err != nil {
	span.RecordError(err)
	return err
}
```

`RecordError` marks the span *and* the trace as failed, which is what the
`?status=error` filter and the `Errors` column in statistics use. It ignores nil
errors, so this is also valid:

```go
defer func() { span.EndWithError(err) }()   // named return
```

Sentinel errors that are expected control flow (`sql.ErrNoRows`,
`context.Canceled`) are usually not worth recording as failures:

```go
if err != nil && !errors.Is(err, sql.ErrNoRows) {
	span.RecordError(err)
}
```

## 5. Patterns

### 5.1 Wrapping a function

```go
err := oida.Do(ctx, "reindex", func(ctx context.Context) error {
	return index.Rebuild(ctx)
}, oida.KindInternal)
```

`Do` starts the span, runs the function, records the error, ends the span and
returns the error unchanged.

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

A `RoundTripper` gives you every outbound call for free:

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

Use `r.URL.Redacted()` — it strips userinfo credentials.

### 5.4 Cache lookups

```go
ctx, span := oida.Start(ctx, "cache: user", oida.KindCache)
value, hit := c.Get(key)
span.SetAttribute("hit", hit)
span.End()

if !hit {
	value, err = loadUser(ctx, key)   // its own database span, sibling of the cache span
}
```

End the cache span before the fallback so the timeline shows cache and database
as separate segments rather than nested.

### 5.5 Concurrent work

Spans started from goroutines are safe; the parent is whatever context you hand
to the goroutine:

```go
ctx, span := oida.Start(ctx, "fan out", oida.KindInternal)
defer span.End()

g, gctx := errgroup.WithContext(ctx)
for _, id := range ids {
	g.Go(func() error {
		_, s := oida.Start(gctx, "load user", oida.KindDatabase)
		defer s.End()
		return load(gctx, id)
	})
}
return g.Wait()
```

Concurrent siblings overlap in the timeline; the segment sweep attributes each
slice of wall-clock to the innermost span that was active, so overlapping work
does not double-count.

### 5.6 Source locations

```go
span.SetSource("internal/billing/repo.go", 42)
```

Optional. The span table shows `file:L42` when set. Do not compute it with
`runtime.Caller` on hot paths — it is not free.

## 6. Trace-level operations

```go
trace := oida.TraceFromContext(ctx)
trace.SetState(oida.StateProcessing)
trace.Fail(err)
id := oida.TraceID(ctx)          // the ULID, also in the Request-Id header
```

Putting the trace ID into your logs is the cheapest possible correlation:

```go
slog.ErrorContext(ctx, "charge failed", "err", err, "trace", oida.TraceID(ctx))
```

## 7. What not to instrument

- Loops with thousands of iterations. Span one loop, attribute the count.
  `MaxSpansPerTrace` will otherwise drop the tail and `DroppedSpans` will grow.
- Functions that take nanoseconds. The span costs more than the work.
- Anything holding a lock you also need for the work — `SetAttribute` takes the
  span mutex, so build the value first, then set it once.

## 8. Testing instrumented code

```go
func TestRepoSpans(t *testing.T) {
	tracer, err := oida.New(oida.NewOptions())
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

Use an explicit `New` rather than `Default()` in tests so parallel packages do
not share a ring buffer.
