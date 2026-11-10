# Session tracing

A session groups everything one browser did in one visit. The session is an
opaque ULID. See [guide-browser.md](guide-browser.md) for the library.

## Requirements

1. `oida-trace.js` keeps the session ULID in `localStorage` (`oida.session`)
   and renews it after an idle window, 30 minutes by default.
2. The session is the trace: the 128 bits of the ULID are the trace-id of every
   `traceparent` the page sends, so `GET /`, `GET /inventory` and
   `GET /inventory#home` all carry `traceparent = session`, each with a span of
   its own.
3. Every event is a transaction posted to `/collect` with `session_id`: the
   pageload, click, submit, dialog, and the address changes a single page app
   makes without a request (`hashchange`, `popstate`, `pushState`,
   `replaceState`).
4. Same-origin calls also send `tracestate: oida=<ULID>`. With
   `Options.TrustTraceContext` the service records its request with the same
   `SessionID`; without it the header is ignored.
5. `model.Trace` and `model.Transaction` carry `SessionID` (`session_id`,
   omitted when empty). A value that is no ULID is dropped and the
   transaction is kept.
6. The dashboard shows the session in the Trace section, a Session column in
   the list, a `session` filter matched exactly, and a `Transactions: session
   roots` filter for the transactions that carry a session and have no parent.

## Not covered

- The first server request of a visit (`GET /` before the library loads) cannot
  carry the session; the pageload links it through its `served` attribute.
- No cookie, no cross-device identity: a session is not a user.

## Acceptance suite

| # | Check | Where |
|---|-------|-------|
| 1 | A pageload mints a ULID session in `localStorage`, and a second pageload reuses it | `scripts/jstest.js` |
| 2 | The trace of a session is its ULID in hex, matching the Go decoding | `scripts/jstest.js`, `session_test.go` |
| 3 | A session idle past the window is replaced; `data-session-idle="0"` never expires | `scripts/jstest.js` |
| 4 | `hashchange`, `pushState` post a transaction in the same session and trace, with no parent; a repeated address posts nothing | `scripts/jstest.js` |
| 5 | A click and a close are in the session | `scripts/jstest.js` |
| 6 | The pageload and a route change posted to `/collect` and a back-end call carrying `tracestate` all record the same `SessionID` and trace | `session_test.go` |
| 7 | Without `TrustTraceContext` the session is not read | `session_test.go` |
| 8 | An invalid session is dropped without failing the transaction | `model/session_test.go` |
| 9 | The list filters by session, and `session roots` keeps only parentless session transactions | `frontend/view/related_test.go` |
