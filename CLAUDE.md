# oida

In-process telemetry for Go: traces and spans in a ring buffer, with a
server-side rendered front end at `/debug/oida`. See [README.md](README.md) and
[docs/](docs/).

## Verifying changes

Run one command per step. Never chain with `;` or `&&`, and never inline
`$(...)` around another command: use the scripts below, which are allowlisted.

**Front end (anything visual) uses `scripts/chromium.sh`:**

```
scripts/chromium.sh /debug/oida list dark
scripts/chromium.sh /debug/oida/live live light 1440x900
```

It writes `/tmp/oida-<name>.png` and prints the path. Read that file to look at
the result. Chromium renders the light theme by default, so pass `dark`
explicitly when checking the dark palette. Always check both.

The script appends `?stream=off` to `/live` automatically: the event stream
keeps the connection open, so `load` never fires and a screenshot would hang.

**APIs, JSON and plain text use `scripts/inspect.sh`:**

```
scripts/inspect.sh seed 8            send demo traffic
scripts/inspect.sh json              trace list as JSON
scripts/inspect.sh jq '.[0].spans'   jq filter over the trace list
scripts/inspect.sh id 2              id of the third recorded trace
scripts/inspect.sh trace 2           that trace in full
scripts/inspect.sh text /debug/oida/stats
scripts/inspect.sh stream 5          read the SSE stream for five seconds
```

Target another instance with `OIDA_BASE` and `OIDA_PATH`.

## Pipeline

`atkins` runs everything: `go mod tidy`, `templ generate`, formatting,
`go:test`, `go:build`, `docker:build`. Individual steps: `atkins templ`,
`atkins templ:verify`, `atkins go:test`, `atkins docker:build`.

**The service only ever runs under docker compose.** Never `go run ./cmd/oida`,
never a stray binary on another port, never `pkill`. One loop, always:

```
atkins
docker compose up -d --force-recreate --wait
scripts/inspect.sh seed 8
```

The demo service is `cmd/oida`, published as `titpetric/oida:latest` and served
by `compose.yml` on <http://localhost:8097>.

## Conventions

- One struct per file, data models in `model.go`. See
  [docs/conventions.md](docs/conventions.md).
- Nothing writes to stdout or stderr. Failures return, or go to
  `Options.OnError`.
- The front end makes no external requests: no webfonts, no CDN, no analytics.
  Assets live in `public/assets/`, are embedded with `//go:embed all:public`,
  and are served at `{Options.Path}/assets/`. Dropping a file in that folder is
  all it takes to serve it.
- Generated templ output (`view_*_templ.go`) is committed. Regenerate after
  editing a `.templ` file.
- Instrumentation is nil-safe: every `*Span` and `*Trace` method tolerates a nil
  receiver.

## Front end rules

- Dark and light are both first-class, driven by `prefers-color-scheme` and one
  set of custom properties. One theme per page, never per section.
- One accent (`--signal`, amber). Span kinds are the only other saturated ink,
  because kind is the data. One corner radius (`--radius`).
- No em-dashes or en-dashes in visible copy. Middle dots are rationed to one per
  line.
- Labels sit above inputs. Placeholder-as-label is never acceptable.
- Motion has to mean something: state, feedback or hierarchy. Everything is
  wrapped in `prefers-reduced-motion`.
- Empty states say what to do next, not just that something is empty.
