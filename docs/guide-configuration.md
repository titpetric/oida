# Configuration

`oida.Options` is the single configuration struct. `oida.NewOptions(serviceName)` returns the defaults; take it and override what you need, so new fields keep their sensible values when you upgrade.

```go
opts := oida.NewOptions("billing-api")
opts.SampleRate = 25
```

## 1. Reference

Every field, its default and what it does. The struct is declared in
`model/options.go` and aliased as `oida.Options`; the yaml tag of a field is
its name in snake case.

Field: `Options.Path`<br>Default: `/debug/oida`<br>Meaning: Mount path of the UI. Must be absolute; trailing slashes are trimmed. Also implicitly added to `IgnorePaths`.

Field: `Options.ServiceName`<br>Default: the name passed to `NewOptions`<br>Meaning: Shown in the header and stored on every trace.

Field: `Options.Enabled`<br>Default: `false`<br>Meaning: Recording is opt-in: enable in code or with `OIDA_ENABLED=true`. When false, the middleware passes through and the handler serves an empty snapshot. Flip at runtime with `Tracer.SetEnabled`.

Field: `Options.RingBufferSize`<br>Default: `200`<br>Meaning: Completed traces retained. 0 disables retention (live view still works).

Field: `Options.TopRequests`<br>Default: `20`<br>Meaning: Rows in the statistics view.

Field: `Options.MaxSpansPerTrace`<br>Default: `1000`<br>Meaning: Spans recorded per trace; further spans are counted in `DroppedSpans` and dropped. 0 means unlimited.

Field: `Options.SampleRate`<br>Default: `100`<br>Meaning: Percentage of requests traced, `[0,100]`.

Field: `Options.TrackMemoryUse`<br>Default: `true`<br>Meaning: Read `runtime.MemStats` around each trace.

Field: `Options.TrustRequestID`<br>Default: `false`<br>Meaning: Reuse a client-supplied `Request-Id` header instead of generating one. Only enable behind a trusted proxy.

Field: `Options.IgnorePaths`<br>Default: `/healthz`, `/readyz`, `/metrics`, `/favicon.ico`<br>Meaning: Exact paths and `/prefix/*` patterns never traced.

Field: `Options.RefreshInterval`<br>Default: `5`<br>Meaning: Fallback refresh of the live view, in seconds, used when the browser cannot stream. 0 disables it.

Field: `Options.LiveStream`<br>Default: `true`<br>Meaning: Serve the live view over server sent events, so traces appear as recorded. False falls back to the meta refresh and 404s the stream route.

Field: `Options.CaptureLogs`<br>Default: `true`<br>Meaning: Record `Info` and `Error` log entries on traces. When false, `Info` does nothing and `Error` records its text through `RecordError` on the active span.

Field: `Options.ReadEnv`<br>Default: `true` from `NewOptions`, `false` in a literal<br>Meaning: Apply the `OIDA_*` environment to these options inside `New`. Turn it off for a tracer that must ignore the deployment, such as one built in a library or a test.

Field: `Options.Sampler`<br>Default: nil<br>Meaning: Overrides `SampleRate` entirely when set.

Field: `Options.Storage`<br>Default: nil<br>Meaning: Retention backend. Nil builds the memory driver with `RingBufferSize` slots.

Field: `Options.RouteFunc`<br>Default: nil<br>Meaning: Returns the routed pattern of a request, so statistics group by route. An empty result means group by path. Nil falls back to `http.Request.Pattern`.

Field: `Options.OnError`<br>Default: nil<br>Meaning: Receives storage and rendering failures. Nil discards them; the package never prints.

Field: `Options.Authorize`<br>Default: nil<br>Meaning: Access check for the UI. Nil means "allow"; set it before exposing the route on a public listener.

Field: `Options.AllowedNetworks`<br>Default: none<br>Meaning: CIDR allow list for the UI. A peer outside it receives a 404, like a failed `Authorize`.

Field: `Options.Users`<br>Default: none<br>Meaning: Usernames to bcrypt hashes behind the sign in screen. Any user puts the UI behind `{OIDA_PATH}/login`.

Field: `Options.UsersFile`<br>Default: none<br>Meaning: An `.htpasswd` style file, one `username:hash` per line, read once and merged under `Users`.

Field: `Options.AuthorizeUser`<br>Default: nil<br>Meaning: Authenticates a login the configured users did not, so a directory can answer instead of a password file.

Field: `Options.SigningSecret`<br>Default: none<br>Meaning: Signs the session cookie and verifies `Authorization: Bearer` JWTs. Empty generates a per-process secret.

Field: `Options.Clock`<br>Default: `time.Now`<br>Meaning: Time source. Tests inject a deterministic clock.

Field: `Options.Tracer`<br>Default: nil<br>Meaning: The recorder `frontend.Handler` renders. The root package needs no such round trip: `Mount` and `Middleware` take the tracer itself.

### 1.1 Route patterns

Statistics group by `Trace.HTTP.Route` when the router provides one. The standard library sets `http.Request.Pattern`, which oida reads with no configuration. chi keeps the pattern in its route context, so hand it over explicitly:

```go
opts.RouteFunc = func(r *http.Request) string {
	if route := chi.RouteContext(r.Context()); route != nil {
		return route.RoutePattern()
	}
	return ""
}
```

Without it, `/users/1` and `/users/2` are two separate rows in the statistics table instead of one `GET /users/{id}` row.

### 1.2 Errors

```go
opts.OnError = func(err error) {
	slog.Error("oida", "err", err)
}
```

This is the only channel for asynchronous failures: a disk write that failed, a component that failed to render. In tests, point it at `t.Errorf`.

## 2. Sizing the ring buffer

Every retained trace holds its spans, so memory is roughly:

```
RingBufferSize × (trace overhead ≈ 400B + spans × (span overhead ≈ 200B + attributes))
```

200 traces × 30 spans ≈ 1.5 MB. A busy service with 100 spans per trace and a 1000-trace buffer is closer to 25 MB, measurable, so size it deliberately. `MaxSpansPerTrace` is the guard rail against one pathological request pinning memory until it rotates out.

## 2.1 Storage

Retention is a pluggable interface with two drivers, and the environment picks between them. The default keeps traces in a memory ring buffer sized by `RingBufferSize`. Disk storage keeps the documents after the process is gone, which is what you want when the interesting trace is the one that happened right before it died:

```bash
OIDA_STORAGE_DRIVER=disk
OIDA_STORAGE_DISK_PATH=/var/lib/myservice/traces
OIDA_STORAGE_DISK_LIMIT=5000
OIDA_STORAGE_DISK_LIST=true
OIDA_STORAGE_DISK_EXPIRE=168h
```

`New` creates the folder, verifies it is writable, and fails when it cannot. The driver then prunes the oldest documents past the limit on every save. Reads never touch the folder: it keeps the same window in a memory ring, and only `Load` falls back to a document the ring evicted. The ring holds the traces this process recorded, so that is what the dashboard lists; documents written by an earlier one are read by ID. With no path it uses a folder under `os.TempDir()`. Documents are JSON, one per trace, named after the trace ID and readable with `jq`:

```bash
jq '.spans[] | select(.kind == "database") | {name, duration_ns}' /var/lib/myservice/traces/*.json
```

The memory driver reads `OIDA_STORAGE_MEMORY_SIZE` the same way, falling back to `RingBufferSize`. A disk setting given on its own selects the disk driver, since that is the only thing it can mean, while naming one driver and configuring the other fails `New` rather than being quietly dropped.

`OIDA_STORAGE_DISK_EXPIRE` and `OIDA_STORAGE_DISK_LIST` act once, when the tracer is built, in that order. The first prunes the folder of documents older than the duration, which bounds it by age the way the limit bounds it by count; a process that wants that continuously puts `Prune` on a ticker of its own. The second reads the folder into the ring, at most `OIDA_STORAGE_DISK_LIMIT` documents newest first, so the dashboard opens on what earlier runs recorded. It is off by default because it costs a directory listing and a decode per document at startup.

Retaining traces anywhere else means implementing `oida.Storage` and setting `Options.Storage`, which wins over every variable above:

```go
opts.Storage = myStorage{db: db}
```

Add `Prune(ctx, maxAge)` to a ticker if you want an age bound as well as a count bound; it is part of the Storage interface, and a driver with nothing to prune returns nil. Keep the limit within a few thousand documents when the dashboard needs to list disk-backed traces frequently.

Anything satisfying `oida.Storage` can provide retention.

## 3. Sampling

### 3.1 Rate sampling

`SampleRate` is a percentage and uses a deterministic counter, not randomness: at 25 every fourth request is traced. That makes tests reproducible and avoids clustering.

```go
opts.SampleRate = 10  // 1 in 10
opts.SampleRate = 100 // everything (default)
opts.SampleRate = 0   // nothing; the UI still serves, the log stays empty
```

### 3.2 Custom samplers

```go
type Sampler interface {
	Sample(r *http.Request) bool
}
```

Anything satisfying it replaces the rate sampler. `SamplerFunc` adapts a plain function:

```go
opts.Sampler = oida.SamplerFunc(func(r *http.Request) bool {
	if r.Header.Get("X-Debug") == "1" {
		return true // always trace explicit debug requests
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true // always trace the API
	}
	return false
})
```

Combining a rate with an override:

```go
rate := oida.NewRateSampler(5)
opts.Sampler = oida.SamplerFunc(func(r *http.Request) bool {
	return r.Header.Get("X-Debug") == "1" || rate.Sample(r)
})
```

An unsampled request does not create a trace or spans. `oida.Start` inside an unsampled request returns a nil span.

## 4. Multiple tracers

One tracer per process is the intended shape. You can compose multiple tracers to isolate the buffers between tenants, and thus support multi-tenancy in your app: build one per tenant, give each its own `Options` with its own `Path` and `RingBufferSize`, and wire each tenant's router with its own `tracer.Middleware` and `oida.Mount`.

Nothing is shared between them. A trace belongs to the tracer that started it, `oida.Start` follows the trace in the context, and the package holds no process wide tracer for either to fall back on. `RingBufferSize` is a per tracer budget, so ten tenants at 500 traces each retain 5000.

## 5. Loading from YAML

Every configurable field has a `yaml` tag; the function fields do not and must be set in code.

```yaml
oida:
  path: /debug/oida
  service_name: billing-api
  enabled: true
  ring_buffer_size: 500
  top_requests: 20
  max_spans_per_trace: 1000
  sample_rate: 25
  track_memory_use: true
  trust_request_id: false
  refresh_interval: 5
  live_stream: true
  ignore_paths:
    - /healthz
    - /readyz
    - /metrics
```

```go
type Config struct {
	Oida oida.Options `yaml:"oida"`
}

cfg := Config{Oida: oida.NewOptions("billing-api")} // defaults first
if err := yaml.Unmarshal(data, &cfg); err != nil {
	return err
}
cfg.Oida.Authorize = adminOnly
tracer, err := oida.New(cfg.Oida)
```

Note the ordering: unmarshal *into* the defaults, so keys absent from the file keep their default rather than becoming zero.

## 6. Validation

`Options.Validate() error` is called by `New` and `Mount`:

| Condition                                                                         | Error                  |
|-----------------------------------------------------------------------------------|------------------------|
| `Path` empty or not starting with `/`                                             | `ErrInvalidPath`       |
| `SampleRate` outside `[0,100]` or NaN                                             | `ErrInvalidSampleRate` |
| `RingBufferSize`, `TopRequests`, `MaxSpansPerTrace` or `RefreshInterval` negative | `ErrInvalidOptions`    |

Start with `NewOptions(serviceName)` before applying overrides. This keeps defaults for fields you do not set while preserving meaningful zero values such as `SampleRate = 0`, `RingBufferSize = 0`, and `RefreshInterval = 0`.

## 7. Turning it off

```go
opts.Enabled = false     // at construction
tracer.SetEnabled(false) // at runtime
```

A disabled tracer stops recording immediately; existing traces stay in the ring buffer until `Reset()` clears them. Nothing else in the process changes: spans started against a disabled tracer are nil.

## 8. Authentication

Besides `Authorize`, the front end has three access mechanisms: a network allow list, users behind a login screen, and bearer tokens. They stack in this order: `Authorize` first, then the allow list, then credentials. A request rejected by `Authorize` or the allow list receives a 404, and a request missing credentials is redirected to the login screen (a browser) or receives a 401 (a JSON or plain text request). With none of the fields set, every request is served, as before.

### 8.1 Allowed networks

```go
opts.AllowedNetworks = []string{"127.0.0.0/8", "10.0.0.0/8", "fd00::/8"}
```

Entries are CIDR prefixes, IPv4 and IPv6 both. The peer is `http.Request.RemoteAddr`, so behind a reverse proxy the list sees the proxy address, not the client's. An allow list on its own asks for no credentials.

### 8.2 Users and the login screen

```go
opts.Users = map[string]string{
	"admin": "$2b$05$.CFyywyis4bpZ5xVynOfRO9K0cpkOEOym43FeIPXz23bwvQ3wEEOm", // htpasswd -nbB admin secret
}
opts.UsersFile = "/etc/billing-api/htpasswd"
```

Password hashes are bcrypt: `htpasswd -nbB admin secret` produces one, and a hash in any other format never matches. The file holds one `username:hash` per line, with blank lines and `#` comments skipped, and is read once when the front end is built. Entries in `Users` override entries from the file.

Any configured user puts the front end behind `{OIDA_PATH}/login`. A successful login sets the `oida_session` cookie: an HS256 JWT with the username as its `sub` claim, valid for twelve hours, scoped to `Options.Path` and `HttpOnly`.

`AuthorizeUser` authenticates logins the configured users did not, so an existing directory can answer instead of a password file:

```go
opts.AuthorizeUser = func(ctx context.Context, username, password string) error {
	return directory.Bind(ctx, username, password)
}
```

Returning nil grants a session naming the given username.

### 8.3 Signing secret and bearer tokens

```go
opts.SigningSecret = os.Getenv("OIDA_SIGNING_SECRET")
```

The secret signs the session cookie and verifies `Authorization: Bearer` tokens, so a script can mint a token instead of driving the form:

```go
auth, err := oida.NewAuth(opts)
if err != nil {
	return err
}
token, err := auth.Session("ci")
```

```bash
curl -s -H "Authorization: Bearer $TOKEN" 'http://localhost:8080/debug/oida/traces?format=json'
```

Any HS256 JWT signed with the secret is accepted; a `sub` claim names the user and an `exp` claim bounds its life. With `SigningSecret` empty and users configured, a per-process secret is generated: logins keep working, but sessions do not survive a restart and no externally minted token verifies. `SigningSecret` set on its own also puts the front end behind credentials; the login form has no user to accept, so a bearer token is the way in.

### 8.4 In YAML

The four fields carry yaml tags like the rest; `AuthorizeUser` is a function and is set in code.

```yaml
oida:
  allowed_networks:
    - 127.0.0.0/8
    - 10.0.0.0/8
  users:
    admin: $2b$05$.CFyywyis4bpZ5xVynOfRO9K0cpkOEOym43FeIPXz23bwvQ3wEEOm
  users_file: /etc/billing-api/htpasswd
  signing_secret: change-me
```

## 9. From the environment

`oida.New` applies the environment to the options when `Options.ReadEnv` is set, which is what `NewOptions` returns. A variable applies only where the code left the field at its `NewOptions` default, so options set in code win. Lists are comma separated.

A variable set to nothing is a variable unset. Values are trimmed before they are read, so an empty string, whitespace, and a list of separators with no entries all leave the option at the default in the table below rather than at the zero value. That is what a compose file writes for a setting left alone. A value that is set to something is strict: anything that does not parse fails `New` instead of falling back.

| Variable                   | Option             | When unset                               |
|----------------------------|--------------------|------------------------------------------|
| `OIDA_SERVICE_NAME`        | `ServiceName`      | the name passed to `NewOptions`          |
| `OIDA_PATH`                | `Path`             | `/debug/oida`                            |
| `OIDA_ENABLED`             | `Enabled`          | `false`: recording stays off             |
| `OIDA_RING_BUFFER_SIZE`    | `RingBufferSize`   | `200`                                    |
| `OIDA_TOP_REQUESTS`        | `TopRequests`      | `20`                                     |
| `OIDA_MAX_SPANS_PER_TRACE` | `MaxSpansPerTrace` | `1000`                                   |
| `OIDA_SAMPLE_RATE`         | `SampleRate`       | `100`; out of `[0,100]` clamps           |
| `OIDA_TRACK_MEMORY_USE`    | `TrackMemoryUse`   | `true`                                   |
| `OIDA_TRUST_REQUEST_ID`    | `TrustRequestID`   | `false`                                  |
| `OIDA_REFRESH_INTERVAL`    | `RefreshInterval`  | `5`                                      |
| `OIDA_LIVE_STREAM`         | `LiveStream`       | `true`                                   |
| `OIDA_CAPTURE_LOGS`        | `CaptureLogs`      | `true`                                   |
| `OIDA_IGNORE_PATHS`        | `IgnorePaths`      | `/healthz,/readyz,/metrics,/favicon.ico` |
| `OIDA_ALLOWED_NETWORKS`    | `AllowedNetworks`  | none: every peer is served               |
| `OIDA_STORAGE_DRIVER`      | `Storage`          | `memory`; `disk` is the other driver     |
| `OIDA_STORAGE_MEMORY_SIZE` | `Storage`          | `OIDA_RING_BUFFER_SIZE`                  |
| `OIDA_STORAGE_DISK_PATH`   | `Storage`          | none: a folder under the temp directory  |
| `OIDA_STORAGE_DISK_LIMIT`  | `Storage`          | `OIDA_RING_BUFFER_SIZE`                  |
| `OIDA_STORAGE_DISK_LIST`   | `Storage`          | `false`: the ring starts with this run   |
| `OIDA_STORAGE_DISK_EXPIRE` | `Storage`          | none: documents are dropped by count     |
| `OIDA_AUTH`                | `Users`            | none: no sign in screen                  |
| `OIDA_USERS_FILE`          | `UsersFile`        | none                                     |
| `OIDA_SIGNING_SECRET`      | `SigningSecret`    | none: a per-process secret is generated  |

`OIDA_AUTH` holds one `username:password` pair, hashed at startup so the options carry a bcrypt hash the way a configured deployment would. This is how the demo serves its sign in screen without a line of auth code.

The demo binary reads two more variables of its own: `OIDA_ADDR` (listen address, `:8080` when unset) and `OIDA_DEMO_INTERVAL` (background trace interval, `5m`; `0` records only real requests).
