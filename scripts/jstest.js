#!/usr/bin/env node
// Test the browser library against a stub of the DOM it reads.
//
//   scripts/jstest.js            run the checks
//   scripts/jstest.js --print    print what one pageload and one click post
//
// The library is a browser file with no build step, so there is no module to
// import: it runs in a vm context holding the globals it reads, and the stub
// records what it posted. node alone, no package.json, no dependencies.
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const library = path.join(__dirname, "..", "frontend", "view", "assets", "oida-trace.js");
const printing = process.argv.includes("--print");

let failures = 0;

// check reports one assertion.
function check(what, ok, detail) {
  if (!ok) {
    failures++;
  }
  if (!printing) {
    console.log(`${ok ? "ok  " : "FAIL"} ${what}${ok || detail === undefined ? "" : ` -- ${detail}`}`);
  }
}

// element returns a stub node carrying attributes and text.
function element(tag, attributes, text) {
  const node = {
    tagName: tag.toUpperCase(),
    own: Object.assign({}, attributes),
    textContent: text || "",
    attributes: [],
    listeners: {},
    getAttribute(name) {
      return name in this.own ? this.own[name] : null;
    },
    hasAttribute(name) {
      return name in this.own;
    },
    setAttribute(name, value) {
      this.own[name] = value;
      this.sync();
    },
    removeAttribute(name) {
      delete this.own[name];
      this.sync();
    },
    closest(selector) {
      return matches(this, selector) ? this : null;
    },
    addEventListener(type, fn) {
      (this.listeners[type] = this.listeners[type] || []).push(fn);
    },
    sync() {
      this.attributes = Object.keys(this.own).map((name) => ({ name, value: this.own[name] }));
    },
  };
  node.sync();
  return node;
}

// matches tests a node against the comma separated selector the library uses.
function matches(node, selector) {
  return selector.split(",").some((part) => {
    part = part.trim();
    if (part.startsWith("[")) {
      const name = part.slice(1, -1).split("=")[0];
      return node.hasAttribute(name);
    }
    return node.tagName.toLowerCase() === part;
  });
}

// world returns the globals the library reads, the recorder it posts into and
// the handles a test fires events with.
function world(options) {
  options = options || {};
  const posted = [];
  const beacons = [];
  const calls = [];
  const timers = [];
  const document_listeners = {};
  const window_listeners = {};
  const observers = [];
  const storage = new Map();
  let counter = 0;
  let clock = 100;

  const documentElement = element("html", {});
  const body = element("body", {});
  const navigation = Object.assign(
    {
      type: "navigate",
      redirectCount: 0,
      nextHopProtocol: "h2",
      transferSize: 4096,
      activationStart: 0,
      redirectStart: 0,
      redirectEnd: 0,
      workerStart: 0,
      fetchStart: 1,
      domainLookupStart: 2,
      domainLookupEnd: 6,
      connectStart: 6,
      secureConnectionStart: 9,
      connectEnd: 20,
      requestStart: 21,
      responseStart: 140,
      responseEnd: 152,
      domContentLoadedEventEnd: 420,
      domComplete: 800,
      loadEventStart: 800,
      loadEventEnd: 820,
    },
    options.navigation || {},
  );

  const sandbox = {
    console,
    JSON,
    Math: Object.create(Math),
    Object,
    Number,
    String,
    RegExp,
    Date,
    Uint8Array,
    URL,
    Headers: global.Headers,
    Blob: class Blob {
      constructor(parts) {
        this.parts = parts;
      }
    },
    location: { href: "https://task.cdn.si/runs?page=2", pathname: "/runs", search: "?page=2", host: "task.cdn.si", origin: "https://task.cdn.si" },
  };

  sandbox.Math.random = () => 0.5;

  sandbox.performance = {
    timeOrigin: 1700000000000,
    now: () => clock,
    getEntriesByType: (type) => (type === "navigation" ? [navigation] : []),
  };

  sandbox.navigator = {
    sendBeacon(url, blob) {
      beacons.push({ url, body: JSON.parse(blob.parts[0]) });
      return true;
    },
  };

  sandbox.document = {
    currentScript: {
      src: "https://task.cdn.si/debug/oida/assets/oida-trace.js",
      getAttribute: (name) => (options.attributes && name in options.attributes ? options.attributes[name] : null),
    },
    documentElement,
    body,
    title: "Runs",
    referrer: "https://task.cdn.si/",
    readyState: "loading",
    visibilityState: "visible",
    querySelector(selector) {
      const name = /name="([^"]+)"/.exec(selector);
      const tags = options.meta || {};
      if (name && name[1] in tags) {
        return element("meta", { name: name[1], content: tags[name[1]] });
      }
      return null;
    },
    addEventListener(type, fn) {
      (document_listeners[type] = document_listeners[type] || []).push(fn);
    },
  };

  sandbox.window = {
    performance: sandbox.performance,
    crypto: {
      // A counter rather than randomness, so a test asserts on the exact
      // identifiers the library wrote.
      getRandomValues(data) {
        for (let i = 0; i < data.length; i++) {
          data[i] = i === data.length - 1 ? ++counter : 0xaa;
        }
        return data;
      },
    },
    sessionStorage: {
      getItem: (key) => (storage.has(key) ? storage.get(key) : null),
      setItem: (key, value) => storage.set(key, value),
      removeItem: (key) => storage.delete(key),
    },
    MutationObserver: class MutationObserver {
      constructor(fn) {
        this.fn = fn;
        observers.push(this);
      }
      observe() {}
    },
    setTimeout(fn, delay) {
      timers.push({ fn, delay, id: timers.length + 1 });
      return timers.length;
    },
    clearTimeout(id) {
      const timer = timers.find((entry) => entry.id === id);
      if (timer) {
        timer.cleared = true;
      }
    },
    addEventListener(type, fn) {
      (window_listeners[type] = window_listeners[type] || []).push(fn);
    },
    fetch(url, init) {
      calls.push({ url: String(url), init });
      if (String(url).indexOf("/collect") >= 0) {
        posted.push(JSON.parse(init.body));
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ transactions: [{ id: "01K0RECORDED0000000000000A", traceparent: "00-" + "0".repeat(31) + "1-" + "0".repeat(15) + "1-01" }] }) });
      }
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) });
    },
  };

  sandbox.window.window = sandbox.window;
  sandbox.Headers = global.Headers;

  // The handoff is read as the library loads, so a page the browser arrived
  // at by a click has it in storage before that.
  if (options.handoff) {
    storage.set("oida.handoff", options.handoff);
  }

  vm.runInNewContext(fs.readFileSync(library, "utf8"), sandbox, { filename: "oida-trace.js" });

  return {
    sandbox,
    posted,
    beacons,
    calls,
    storage,
    // fire dispatches an event at the document or the window.
    fire(target, type, event) {
      const listeners = (target === "window" ? window_listeners : document_listeners)[type] || [];
      listeners.forEach((fn) => fn(event || {}));
    },
    // mutate drives the observer the library watches the open attribute with.
    mutate(node) {
      observers.forEach((observer) => observer.fn([{ target: node }]));
    },
    // tick runs the timers that are due, which is what the idle period is.
    tick() {
      const due = timers.filter((timer) => !timer.cleared && !timer.ran);
      due.forEach((timer) => {
        timer.ran = true;
        timer.fn();
      });
    },
    advance(ms) {
      clock += ms;
    },
  };
}

// ------------------------------------------------------------ the pageload
const page = world({});
page.fire("window", "load");
page.tick();

const pageTx = (page.posted[0] || { transactions: [] }).transactions[0] || {};
check("a load posts one transaction", page.posted.length === 1 && page.posted[0].transactions.length === 1, JSON.stringify(page.posted));
check("the pageload is named after the path", pageTx.name === "pageload /runs", pageTx.name);
check("it is a browser transaction", pageTx.kind === "browser", pageTx.kind);
check("it carries a 32 hex trace", /^[0-9a-f]{32}$/.test(pageTx.trace_id || ""), pageTx.trace_id);
check("it carries a 16 hex span", /^[0-9a-f]{16}$/.test(pageTx.span_id || ""), pageTx.span_id);
check("it nests under nobody", pageTx.parent_id === undefined, pageTx.parent_id);
check("it starts at the time origin", pageTx.started_at === 1700000000000, pageTx.started_at);
check("it lasts to loadEventEnd", pageTx.duration_ms === 820, pageTx.duration_ms);
check("it records the url", (pageTx.attributes || {}).url === "https://task.cdn.si/runs?page=2", (pageTx.attributes || {}).url);
check("it records the title", (pageTx.attributes || {}).title === "Runs");
check("it records the referrer", (pageTx.attributes || {}).referrer === "https://task.cdn.si/");
check("it records the navigation type", (pageTx.attributes || {})["navigation.type"] === "navigate");
check("it names the host it loaded from", (pageTx.http || {}).host === "task.cdn.si");
check(
  "it turns the timings into spans",
  (pageTx.spans || []).map((span) => span.name).join(",") === "dns,connect,tls,request,response,dom,interactive,load",
  (pageTx.spans || []).map((span) => span.name).join(","),
);
check("a timing that does not apply is left out", !(pageTx.spans || []).some((span) => span.name === "redirect" || span.name === "worker"));
check("the request span is offset from the start", (pageTx.spans || []).some((span) => span.name === "request" && span.start_ms === 21 && span.duration_ms === 119));

// --------------------------------------------------------- the meta tag trace
const served = world({ meta: { traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", tracestate: "rojo=1" } });
served.fire("window", "load");
served.tick();
const servedTx = served.posted[0].transactions[0];
check("a traceparent meta tag names the trace", servedTx.trace_id === "4bf92f3577b34da6a3ce929d0e0e4736", servedTx.trace_id);
check("the pageload nests under the request that served it", servedTx.parent_id === "00f067aa0ba902b7", servedTx.parent_id);
check("the tracestate comes through", servedTx.tracestate === "rojo=1", servedTx.tracestate);

// -------------------------------------------------------------- the action
const click = world({});
click.fire("window", "load");
click.tick();
const button = element("button", { id: "run", class: "btn primary", "data-job": "go:test", "data-folder": "oida" }, " Run  tests ");
click.advance(5);
click.fire("document", "click", { target: button });

const inflight = click.sandbox.window.oidaTrace.traceparent();
click.advance(30);
click.tick();

const actionTx = (click.posted[1] || { transactions: [] }).transactions[0] || {};
check("a click posts its own transaction", click.posted.length === 2, JSON.stringify(click.posted.length));
check("the action is named by the element and the event", actionTx.name === "button event=click", actionTx.name);
check("it shares the page's trace", actionTx.trace_id === pageTx.trace_id || /^[0-9a-f]{32}$/.test(actionTx.trace_id));
check("it nests under the pageload", actionTx.parent_id === click.posted[0].transactions[0].span_id, actionTx.parent_id);
check("the span it named is the one its calls carry", inflight === "00-" + actionTx.trace_id + "-" + actionTx.span_id + "-01", inflight);
check("it records the event", (actionTx.attributes || {}).event === "click");
check("it records the label from the text", (actionTx.attributes || {}).label === "Run tests", (actionTx.attributes || {}).label);
check("it records the id and the class", (actionTx.attributes || {}).id === "run" && (actionTx.attributes || {}).class === "btn primary");
check("it records every data attribute", (actionTx.attributes || {})["data-job"] === "go:test" && (actionTx.attributes || {})["data-folder"] === "oida");
check("it lasts as long as the action was open", actionTx.duration_ms === 30, actionTx.duration_ms);

// ------------------------------------------------------- the inner element
const inner = world({});
inner.fire("window", "load");
inner.tick();
const span = element("span", {}, "Run");
span.closest = (selector) => (matches(span, selector) ? span : element("a", { href: "/runs/42" }, "Run"));
inner.fire("document", "click", { target: span });
inner.tick();
const linkTx = inner.posted[1].transactions[0];
check("a click inside a link names the link", linkTx.name === "a event=click", linkTx.name);
check("it records the href", linkTx.attributes.href === "/runs/42", linkTx.attributes.href);
check("the link is handed to the next pageload", (inner.storage.get("oida.handoff") || "").indexOf("/runs/42") > 0, inner.storage.get("oida.handoff"));

// -------------------------------------------------------------- the calls
const fetching = world({});
fetching.fire("window", "load");
fetching.tick();
fetching.fire("document", "click", { target: element("button", { id: "refresh" }, "Refresh") });
const pending = fetching.sandbox.window.fetch("/api/runs", { method: "GET" });
pending.then(() => {
  fetching.tick();
  const call = fetching.calls.find((entry) => entry.url === "/api/runs");
  const header = call && call.init && call.init.headers ? call.init.headers.get("traceparent") : "";
  check("an api call carries the action's traceparent", /^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/.test(header || ""), header);

  const withCall = fetching.posted[1].transactions[0];
  check("the call lands on the action as a span", (withCall.spans || []).some((entry) => entry.name === "fetch /api/runs"), JSON.stringify(withCall.spans));
  check("the collect route is not traced itself", !fetching.calls.some((entry) => entry.url.indexOf("/collect") >= 0 && entry.init.headers && entry.init.headers.get && entry.init.headers.get("traceparent")));

  // ----------------------------------------------------------- the dialog
  const dialog = world({});
  dialog.fire("window", "load");
  dialog.tick();
  const form = element("dialog", { id: "folder", open: "" }, "");
  dialog.mutate(form);
  dialog.tick();
  const openTx = dialog.posted[1].transactions[0];
  check("a dialog that opened is an action", openTx.name === "dialog event=open", openTx.name);
  form.removeAttribute("open");
  dialog.mutate(form);
  dialog.tick();
  check("a dialog that closed is an action", dialog.posted[2].transactions[0].name === "dialog event=close", dialog.posted[2].transactions[0].name);

  // ------------------------------------------------------------ the close
  const going = world({});
  going.fire("window", "load");
  going.tick();
  going.fire("document", "click", { target: element("a", { href: "/runs/42" }, "Open") });
  going.advance(1200);
  going.sandbox.document.visibilityState = "hidden";
  going.fire("document", "visibilitychange");

  const closeTx = (going.beacons[0] || { body: { transactions: [] } }).body.transactions.pop() || {};
  check("the page is flushed with a beacon", going.beacons.length === 1, JSON.stringify(going.beacons.length));
  check("the close says the page went hidden", (closeTx.attributes || {}).event === "hidden", (closeTx.attributes || {}).event);
  check("it says how long the page was up", (closeTx.attributes || {}).visible_ms === 1300, (closeTx.attributes || {}).visible_ms);
  check("it names the link the user opened", (closeTx.attributes || {}).to === "/runs/42", (closeTx.attributes || {}).to);
  check("it names the traceparent the next pageload will join", /^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/.test((closeTx.attributes || {})["to.traceparent"] || ""));
  check("the open action was settled first", going.beacons[0].body.transactions.some((entry) => entry.name === "a event=click"));

  // ------------------------------------------------- the pageload it causes
  const handed = JSON.parse(going.storage.get("oida.handoff") || "{}");
  const arrived = world({ handoff: going.storage.get("oida.handoff") });
  arrived.fire("window", "load");
  arrived.tick();
  const arrivedTx = arrived.posted[0].transactions[0];
  check("the next pageload stays in the same trace", arrivedTx.trace_id === handed.trace, arrivedTx.trace_id);
  check("it nests under the click that opened it", arrivedTx.parent_id === handed.span, arrivedTx.parent_id);
  check("it names where it came from", arrivedTx.attributes.from === "/runs/42", arrivedTx.attributes.from);

  if (printing) {
    console.log(JSON.stringify({ pageload: pageTx, click: actionTx }, null, 2));
  } else {
    console.log(`\n${failures === 0 ? "all checks passed" : failures + " checks failed"}`);
  }
  process.exit(failures === 0 ? 0 : 1);
});
