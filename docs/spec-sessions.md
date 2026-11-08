# Session tracing: requirements

Status: proposal, stacked on #21 (distributed tracing). This document gathers
requirements only. No code changes with it.

## Problem

#21 joins processes on one request with W3C Trace Context. A trace there is
one request, or one click and what it called. Nothing relates the traces of
one visit or one conversation. The browser library keeps a handoff in
`sessionStorage` that nests the next pageload under the click that opened it,
but a reload, a new tab, a typed URL or a link from another site starts a
trace with nothing above it.

The motivating case is task-ui. The Orb takes a note, the board dispatches a
run per agent the thread names, and a reply reopens the thread and dispatches
again. That is one conversation over hours, spanning many traces, and no
identifier names it. The same gap shows in the browser: an operator's visit
is a dozen unrelated traces.

A session is the group above traces. A trace stays one causal tree.

## Terms

- **Session**: an ordered set of traces that one actor produced for one
  purpose, with idle gaps allowed between them.
- **Session ID**: opaque identifier of a session, recorded on every
  transaction in it.
- **Actor**: a browser tab group, a process, or a task-ui thread.

## Requirements

### Model

1. `model.Trace` gains `SessionID`, an optional string. Empty means no session.
2. The ID is opaque, at most 64 bytes of `[A-Za-z0-9._:-]`, and compared by
   exact match. A value outside that set is dropped, and the trace is kept.
3. A session never replaces `TraceID`. Traces keep one root and one causal
   tree. A session holds no parent/child relation of its own, only order by
   `StartedAt`.
4. `model.Transaction` gains `SessionID` for client-posted work, validated the
   same way and bounded by the existing collect limits.

### Propagation

5. Carrier: the W3C `baggage` header, key `session.id`, per the OpenTelemetry
   semantic convention `session.id`. `traceparent` is not extended.
   Alternative considered: `tracestate`. Rejected, because a vendor may drop or
   reorder it, and #21 already treats it as pass-through.
6. Reading is opt in behind `Options.TrustSessionID`, off by default, like
   `TrustTraceContext`. The header decides which session a reader finds a
   request under, so an unauthenticated caller must not choose it by default.
7. The middleware writes no `baggage` on the response. The client owns the
   session ID, and echoing it adds nothing.
8. Outward, the `Propagate`, `Inject` and `Environ` calls of #21 carry the
   session: `Environ` writes `BAGGAGE` beside `TRACEPARENT`. `oida.New` reads
   `BAGGAGE` back, so a child process joins the parent's session.
9. Only the `session.id` member is read or written. Other `baggage` members
   pass through unread, are never recorded, and never leak into attributes.
   Baggage can carry personal data, so the library must not record what it
   does not name.
10. `Options.SessionID`, with the environment form, fixes the session for a
    process that is itself one session, such as a CLI run or a job.

### Browser library

11. `oida-trace.js` mints a session ID once per browser session: 128 random
    bits, hex, kept in `sessionStorage` under one key. A tab that opens
    another from a link does not inherit it (`sessionStorage` is per tab), so
    each tab is its own session. This is intended.
12. The ID resets after 30 minutes with no activity, the same default as
    Google Analytics and Sentry replay sessions. The idle window is
    `data-session-idle` on the script tag, in seconds, `0` for never.
13. The library sends `baggage: session.id=<id>` with the same-origin calls
    that already carry `traceparent`, and posts the ID in each transaction of
    a collection. Cross-origin calls get neither, for the reason #21 gives.
14. `navigator.doNotTrack` and `Sec-GPC` turn the session off: no storage,
    no header. The pageload transaction still posts, without an ID.
15. No cookie, no `localStorage`, no fingerprinting inputs. The ID is random
    and names no person.

### Storage and retention

16. The ring buffer needs no new retention rule. A session lives as long as
    its newest trace, and the oldest trace of a session is dropped first.
17. No session table. The list view derives sessions by grouping on
    `SessionID` over what the buffer holds.
18. A session with more traces than the buffer holds reads as truncated, not
    as a lie: the view shows the count it has and says older traces were
    dropped.

### Dashboard and API

19. The detail page's Trace section gains the session ID and the other traces
    of the session, oldest first, capped at 50, each a link. The plain text
    renderer prints the same block.
20. The list view takes a `session` filter, matched exactly, and a column
    that links to it.
21. The JSON list and detail carry `session_id`. The field is omitted when
    empty.
22. Search by session ID uses the existing list filter. No new route.

### task-ui

23. task-ui sets the session ID to the note thread ID for every run the board
    dispatches for that thread, through `Environ`. A reply in the thread
    dispatches again under the same ID, so a conversation reads as one session.
24. The run's pipeline inherits it through `BAGGAGE`, so the atkins tool calls
    in `atkins.db` carry the same session as the run.
25. The combined view the owner asked for in #20 is then one query: the
    session's traces, oldest first, each with its tool invocations beneath.

### Non goals

- Cross-device or logged-in identity. A session is not a user.
- Session replay or recording of any page content.
- Sampling per session. The local sampler stays the authority, as in #21.
- Baggage beyond `session.id`.

## Open questions

1. Is `Options.TrustSessionID` enough, or should the collect route accept a
   session only with `Options.Collect`? Proposal: the same switch, since a
   post already trusts its client's identifiers.
2. Idle timeout default: 30 minutes, or none until the page reports it?
3. Should the thread ID go out as-is (a ULID, so safe) or be hashed? Proposal:
   as-is, since the board already shows it.
4. Does a closed session need an explicit end event? Proposal: no. Idle
   time is the only close, which survives a crashed tab.

## Test plan

- `model`: ID validation (length, charset, empty), `baggage` parse with
  unrelated members, malformed members, and a header at the 8192 byte bound
  the W3C Baggage specification states.
- Root: middleware with and without `TrustSessionID`; `Environ` and
  `oida.New` round trip.
- `frontend`: collect accepts and records `session_id`; an invalid ID drops
  and records the transaction.
- `scripts/jstest.js`: ID minted once, kept across pageloads, reset after the
  idle window, absent under `doNotTrack`, sent on same-origin calls only.
- View: the session block renders, with the 50 trace cap, and in text.
