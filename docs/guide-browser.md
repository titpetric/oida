# The browser library

`oida-trace.js` records what a browser did and posts it to the collect route. One file, no dependencies, no build step. It is served out of the embedded asset tree, so a page includes it from the mount path and nothing else is deployed:

```html
<script src="/debug/oida/assets/oida-trace.js" data-service="task-ui-web"></script>
```

The route it posts to is resolved from the script's own `src`, so a `Content-Security-Policy` of `script-src 'self'` is enough and no inline configuration is needed. Turn the route on first:

```go
opts := oida.NewOptions("task-ui")
opts.Enabled = true
opts.Collect = true
opts.TrustTraceContext = true
```

`Collect` is off by default. It is the one route of the front end that writes, and it takes its times and its identifiers from a client, which nothing else in oida does.

## What it records

Three kinds of transaction, each one trace of its own in the dashboard, all of them sharing one W3C trace identifier.

**A pageload**, once the `load` event has run. It carries the URL, the title, the referrer, the navigation type and the transfer size, and turns the `PerformanceNavigationTiming` entry into spans: `dns`, `connect`, `tls`, `request`, `response`, `dom`, `interactive`, `load`, plus `redirect` and `worker` where they happened. Every field of that entry reads zero when it does not apply, so a span is kept only where both of its ends are set.

**A user action**, one per click, submit, and dialog or details element opening and closing. Its parent is the pageload's span. It is named for the element and the event, and carries what identifies that element: the label or text, the `href`, the `id`, the `class`, the `name`, the `type` and every `data-` attribute.

```json
{
  "name": "button event=click",
  "parent_id": "aaaaaaaaaaaaaa02",
  "attributes": {
    "event": "click",
    "label": "Run tests",
    "id": "run",
    "class": "btn primary",
    "data-job": "go:test",
    "data-folder": "oida"
  }
}
```

An action stays open until it has been quiet for a second, so a click that fires three calls is one transaction with three spans rather than four transactions.

**A close**, when the page goes away: why it went, how long it was visible, and the link the user opened just before it. `visibilitychange` is the trigger, which is the last moment a page reliably observes, with `pagehide` behind it for the browsers that report no visibility. `unload` is never used: a listener on it makes the page ineligible for the back/forward cache, and mobile browsers never fire it. The flush goes out through `navigator.sendBeacon`, which outlives the document.

## How the page is observed

Two delegated listeners at the document, both in the capture phase, so neither a handler that stops propagation nor an event that does not bubble can hide an action. A click is attributed to the element a reader would name: `closest("a,button,summary,label,input,select,textarea,[role=button],[data-oida]")`, so a click on the span inside a button names the button.

One `MutationObserver` watches the `open` attribute, because a `<dialog>` opened with `showModal()` fires no event at all. Nothing else is observed and no call site is asked to instrument itself.

## How a click reaches the backend

The action mints its own span identifier before it posts anything, so there is no round trip in the way. Every same-origin `fetch` and `XMLHttpRequest` made while the action is open carries it:

```
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-aaaaaaaaaaaaaa03-01
```

With `Options.TrustTraceContext` on, the request records under the same trace with that span as its parent, and the call lands on the action as a span of its own. The receipt of each post says what the recorder made of every transaction, under `window.oidaTrace.recorded`.

A cross-origin call is not named: the other server would have to allow the header in its CORS configuration, which is not this library's to arrange. The collect route itself is never traced.

## How the trace starts

Three sources, in order:

1. A `traceparent` meta tag the server rendered into the page, which is how a pageload nests under the request that served it. This is the mechanism OpenTelemetry's document-load instrumentation and Sentry's browser tracing both use.

   ```html
   <meta name="traceparent" content="00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01">
   ```

   Write it from the handler with `oida.TraceParent(ctx)`.

2. The handoff a click left in `sessionStorage`, so the pageload that follows a link nests under the click that opened it, and both pages are one trace.

3. A trace of this page's own, 128 random bits.

## Settings

| Attribute      | Default                      | Meaning                                                 |
|----------------|------------------------------|---------------------------------------------------------|
| `data-collect` | the script's own path        | Where transactions are posted                           |
| `data-service` | the recorder's `ServiceName` | What the transactions are attributed to                 |
| `data-idle`    | `1000`                       | Milliseconds of quiet before an action settles          |
| `data-sample`  | `100`                        | Percentage of page views recorded, decided once on load |

## The API

```js
window.oidaTrace.trace;           // the trace identifier this page shares
window.oidaTrace.traceparent();   // the header a call made now should carry
window.oidaTrace.action(event, element);
window.oidaTrace.settle();
window.oidaTrace.flush();
window.oidaTrace.recorded;        // the receipt of the last post
```

`action` is for work the delegated listeners cannot see, such as a widget that handles its own keyboard.

## Bounds

A posted collection carries at most 32 transactions, a transaction at most 256 spans and 64 attributes, and the body at most 64 KiB, which is the cap a browser puts on a beacon. A client start more than a day from the recorder's clock gives way to the recorder's, so a wrong browser clock cannot reorder the log.

## Testing it

`atkins js:test` runs the library against a stub of the DOM it reads, in `node` alone with no package.json and no dependencies:

```
scripts/jstest.js            run the checks
scripts/jstest.js --print    print what one pageload and one click post
```

---

Previous: [Distributed tracing](guide-tracing.md) · [Contents](README.md)
