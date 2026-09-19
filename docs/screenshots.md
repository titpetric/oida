# Screenshots

What `/debug/oida` looks like in a running service. Every shot below is the demo under load, captured at 1440px in the dark theme. The dashboard also supports a light theme.

## Mobile verification

These full-page captures use a 390px viewport. They include the page footer so narrow tables, notes and closing links can be checked in one image. The detail view covers its span, log and service metric disclosures.

| View                           | Capture                                                                                                   |
|--------------------------------|-----------------------------------------------------------------------------------------------------------|
| Hosts                          | [Dark](assets/mobile-hosts-dark.png), [light](assets/mobile-hosts-light.png)                              |
| Traces                         | [Dark](assets/mobile-traces-dark.png), [light](assets/mobile-traces-light.png)                            |
| Live                           | [Dark](assets/mobile-live-dark.png), [light](assets/mobile-live-light.png)                                |
| Statistics                     | [Dark](assets/mobile-stats-dark.png), [light](assets/mobile-stats-light.png)                              |
| Detail: supporting data closed | [Dark](assets/mobile-detail-closed-dark.png), [light](assets/mobile-detail-closed-light.png)              |
| Detail: spans                  | [Dark](assets/mobile-detail-dark.png), [light](assets/mobile-detail-light.png)                            |
| Detail: log                    | [Dark](assets/mobile-detail-logs-dark.png), [light](assets/mobile-detail-logs-light.png)                  |
| Detail: service data           | [Dark](assets/mobile-detail-metrics-dark.png), [light](assets/mobile-detail-metrics-light.png)            |
| Sign in                        | [Dark](assets/mobile-login-dark.png), [light](assets/mobile-login-light.png), [desktop](assets/login.png) |

## The masthead

Who this process is, what it has served, and what it is costing: service name, PID, Go version, goroutines and uptime join requests, sampling, SLA, heap, GC and the memory a trace costs on average in the service metrics. On mobile the one-line masthead carries a remembered switch for the whole group, so a closed dashboard starts directly with its navigation. The host switcher beside the wordmark narrows every view below it to one domain.

![The oida masthead: service identity, process facts, and the instrument row](assets/header.png)

## Hosts

The landing page. One row per domain this process has served, with the share of retained traces it carries, its average and worst response time, and how many spans a trace records there. Picking a host filters everything else.

![The host overview: one row per domain with retained trace share and timings](assets/hosts.png)

## Traces

Everything in the ring buffer, newest first, filterable by text, kind and status. The shape column is the trace itself: where its time went, by kind, on a scale shared with every other row, so a trace that spent itself in one place is recognisable before you read a number. Failures carry their status and colour the row.

![The trace list: filters, one row per trace, and a proportional shape column](assets/traces.png)

## Statistics

The rolling window, grouped by routed pattern rather than by URI, so `/users/{id}` is one line and not ten thousand. Count, errors, average and worst duration, response size, allocations and average span count per route.

![Rolling statistics grouped by route: share, count, errors, timings and allocations](assets/stats.png)

## One trace: the drawing

The trace read as audio. Every span is a waveform laid along the stretch it ran for and mirrored about a shared centre line, so the waves overlay and blend: a moment with four spans open is hotter than a moment with one. One envelope of five swells, modulated by how many spans were actually running, carries every wave, which is what makes them read as takes of the same music rather than unrelated noise. Loudness rises with nesting depth, so the request is a broad quiet body and the queries inside it are the bright spikes standing in it.

The shape of a single wave is not data. Its extent, its colour and how loud it runs are. Underneath sits the time axis it was drawn against.

![The trace as overlaid waveforms, blending where they cross](assets/detail-waves.png)

## One trace: the memory

Drawn when the spans reported `memory_usage`, below the span and log view. Each reading is the memory in use when a span finished, so the line holds flat and steps where a span let go: the step is the span that allocated. The line runs the whole trace, opening at the first reading's level and holding the last to the end. Hovering a stretch of the line names that span. A one-line legend compares memory at the start, middle, and end of the trace without repeating its time axis. A limit close to the readings joins the plot as a dashed reference line. The chart is closed by default and remembers the reader's choice.

![The memory graph: a step line with start, middle, and end readings](assets/detail-memory.png)

## One trace: the spans

Clicking the drawing folds its kind summary out directly beneath it, answering where the time went without making the legend permanent. A separate `Spans 6` row carries the remembered switch. Thrown, it folds out every span the trace recorded, in tree order: kind, offset from the start of the request, duration, a bar against the shared time axis, and the name. Two columns appear when the trace has them, and are not drawn when it does not: the memory in use when each span finished, drawn against the largest reading in the trace, and the source location that opened the span. The demo reports an increasing memory reading as each span finishes, making the graph and the per-span column useful as layout fixtures. Attributes fold out per span, and a recorded error is printed where it happened. The choice is remembered for the whole front end, so a reader who wants the spans shut has them shut on every trace they open.

![The legend bar and the span table folded out from behind it, one row per span with its memory reading](assets/detail-spans.png)

## One trace: the log

The log follows the spans behind its own remembered row and switch. Each entry is what the code wrote through `Info` or `Error` while the request ran: its offset from the start of the trace, its level, the span that was active when it was written wearing its kind's colour, and the message with its key=value attributes. Here a report request tells its whole story, session to audit event, with two shards blowing their budget in red. A trace that wrote nothing shows a zero and a line saying how to record an entry.

![The log disclosure: one row per entry with its offset, level, active span and message](assets/detail-logs.png)

## One trace: the request

Request, Transaction, and System each have a row and remembered switch. Opened on desktop, their tables sit side by side: request details on the left, request cost on the right, and what the transaction recorded about itself between them. Labels run down the left of each and values down the right, one row per property. A trace that recorded no attributes of its own says so behind the Transaction switch.

![The request, transaction and system fact tables, and the page footer](assets/detail-footer.png)

## The sign in screen

Drawn when the service configures authentication, which the demo does with `OIDA_AUTH=username:password`. A browser without a session lands here: one card, labels above the inputs, the one amber action, and none of the recorded data rendered around it. A failed login keeps the username and says what to do next; a successful one sets the session cookie and returns to the page the browser asked for.

![The sign in screen: one card with the username and password fields](assets/login.png)
