// oida-trace records a pageload, every user action and the way the page went
// away, and posts them to the collect route. See docs/guide-browser.md.
(function () {
  "use strict";

  var script = document.currentScript;
  if (!script || !window.performance) {
    return;
  }

  // The route resolves off this script's own URL, so script-src 'self' loads it.
  var endpoint = script.getAttribute("data-collect") || script.src.replace(/\/assets\/[^/]+$/, "/collect");
  var service = script.getAttribute("data-service") || "";
  var idle = Number(script.getAttribute("data-idle")) || 1000;
  var sample = Number(script.getAttribute("data-sample") || "100");

  // The session is a ULID kept in localStorage, shared by every tab of the
  // origin, and renewed once nothing has happened for the idle window.
  var storageKey = "oida.session";
  var sessionIdle = Number(script.getAttribute("data-session-idle"));
  if (isNaN(sessionIdle) || script.getAttribute("data-session-idle") === null) {
    sessionIdle = 1800;
  }

  if (sample < 100 && Math.random() * 100 >= sample) {
    return;
  }

  // hex returns the lowercase identifier W3C Trace Context fixes the length of.
  function hex(bytes) {
    var data = new Uint8Array(bytes);
    if (window.crypto && window.crypto.getRandomValues) {
      window.crypto.getRandomValues(data);
    } else {
      for (var i = 0; i < bytes; i++) {
        data[i] = Math.floor(Math.random() * 256);
      }
    }
    var out = "";
    for (var j = 0; j < data.length; j++) {
      out += (data[j] + 0x100).toString(16).slice(1);
    }
    return out;
  }

  var crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";

  // ulid returns a 26 character ULID: 48 bits of the clock, 80 random.
  function ulid() {
    var data = new Uint8Array(16);
    var now = Date.now();
    for (var i = 5; i >= 0; i--) {
      data[i] = now % 256;
      now = Math.floor(now / 256);
    }
    var random = new Uint8Array(10);
    if (window.crypto && window.crypto.getRandomValues) {
      window.crypto.getRandomValues(random);
    } else {
      for (var r = 0; r < 10; r++) {
        random[r] = Math.floor(Math.random() * 256);
      }
    }
    data.set(random, 6);
    var out = "";
    for (var n = 0; n < 26; n++) {
      var value = 0;
      for (var bit = 0; bit < 5; bit++) {
        var at = n * 5 + bit - 2;
        value = (value << 1) | (at >= 0 && at < 128 ? (data[Math.floor(at / 8)] >> (7 - (at % 8))) & 1 : 0);
      }
      out += crockford.charAt(value);
    }
    return out;
  }

  // traceOf returns the 32 hex digits a ULID encodes, so the trace of a
  // session is the session.
  function traceOf(id) {
    var bits = "";
    for (var n = 0; n < 26; n++) {
      var value = crockford.indexOf(id.charAt(n));
      bits += (value + 32).toString(2).slice(1);
    }
    bits = bits.slice(2);
    var out = "";
    for (var i = 0; i < 128; i += 4) {
      out += parseInt(bits.slice(i, i + 4), 2).toString(16);
    }
    return out;
  }

  // sessionID returns the live session, minting one when there is none or
  // the last activity is older than the idle window, and stamps the activity.
  var memory = null;
  function sessionID() {
    var now = Date.now();
    var kept = memory;
    try {
      var value = window.localStorage.getItem(storageKey);
      kept = value ? JSON.parse(value) : kept;
    } catch (err) {
      // Storage that is off: the session lives as long as this page.
    }
    if (!kept || !/^[0-9A-HJKMNP-TV-Z]{26}$/.test(kept.id || "") || (sessionIdle > 0 && now - kept.seen > sessionIdle * 1000)) {
      kept = { id: ulid(), seen: now };
    }
    kept.seen = now;
    memory = kept;
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(kept));
    } catch (err) {
      // Private mode or a quota.
    }
    return kept.id;
  }

  // renew makes the trace, the session and the tracestate follow the live
  // session, so every transaction and every call names the one it is in.
  function renew() {
    var id = sessionID();
    if (id !== session) {
      session = id;
      trace = traceOf(id);
      state = "oida=" + id + (extra ? "," + extra : "");
    }
  }

  function traceparent(span) {
    renew();
    return "00-" + trace + "-" + span + "-01";
  }

  // parse reads a traceparent, null for one this version cannot use.
  function parse(value) {
    var f = String(value || "").trim().split("-");
    if (f.length < 4 || f[0] !== "00" || f[1].length !== 32 || f[2].length !== 16) {
      return null;
    }
    if (!/^[0-9a-f]+$/.test(f[1] + f[2]) || /^0+$/.test(f[1]) || /^0+$/.test(f[2])) {
      return null;
    }
    return { trace: f[1], span: f[2] };
  }

  function meta(name) {
    var tag = document.querySelector('meta[name="' + name + '"]');
    return tag ? tag.getAttribute("content") : "";
  }

  // The server request that served this page is named by a traceparent meta
  // tag. It lives in a trace of its own, so the pageload links to it by
  // attribute and carries the session's trace.
  var served = meta("traceparent");
  var extra = served ? meta("tracestate") : "";
  var session = "";
  var trace = "";
  var state = "";
  renew();
  var pageSpan = hex(8);
  var post = typeof window.fetch === "function" ? window.fetch.bind(window) : null;
  var queue = [];
  var open = null;
  var timer = 0;
  var closed = false;

  function round(value) {
    return Math.round(value * 1000) / 1000;
  }

  function transaction(name, span, parent, attributes) {
    renew();
    return {
      session_id: session,
      trace_id: trace,
      span_id: span,
      parent_id: parent || undefined,
      tracestate: state,
      service: service || undefined,
      name: name,
      kind: "browser",
      started_at: Date.now(),
      duration_ms: 0,
      attributes: attributes,
      http: { method: "GET", uri: location.pathname + location.search, host: location.host, status: 200 },
      spans: [],
    };
  }

  // pageload turns the Navigation Timing entry into the transaction and the
  // spans the browser already measured, keeping a span only where both of its
  // ends are set: every field of that entry reads zero when it does not apply.
  function pageload() {
    var nav = (performance.getEntriesByType && performance.getEntriesByType("navigation")[0]) || null;
    var attributes = { event: "pageload", url: location.href, title: document.title, referrer: document.referrer };
    if (served) {
      attributes["served"] = served;
    }
    var tx = transaction("pageload " + location.pathname, pageSpan, "", attributes);
    if (!nav) {
      tx.duration_ms = round(performance.now());
      return tx;
    }

    attributes["navigation.type"] = nav.type;
    attributes["navigation.redirects"] = nav.redirectCount;
    if (nav.nextHopProtocol) {
      attributes["navigation.protocol"] = nav.nextHopProtocol;
    }
    if (nav.transferSize) {
      attributes["navigation.transfer_bytes"] = nav.transferSize;
    }
    if (nav.activationStart) {
      attributes["navigation.prerendered"] = true;
    }

    tx.started_at = Math.round(performance.timeOrigin);
    tx.duration_ms = round(nav.loadEventEnd || nav.domComplete || nav.responseEnd || performance.now());
    [
      ["redirect", nav.redirectStart, nav.redirectEnd],
      ["worker", nav.workerStart, nav.fetchStart],
      ["dns", nav.domainLookupStart, nav.domainLookupEnd],
      ["connect", nav.connectStart, nav.connectEnd],
      ["tls", nav.secureConnectionStart, nav.connectEnd],
      ["request", nav.requestStart, nav.responseStart],
      ["response", nav.responseStart, nav.responseEnd],
      ["dom", nav.responseEnd, nav.domContentLoadedEventEnd],
      ["interactive", nav.domContentLoadedEventEnd, nav.domComplete],
      ["load", nav.loadEventStart, nav.loadEventEnd],
    ].forEach(function (span) {
      if (span[1] > 0 && span[2] >= span[1]) {
        tx.spans.push({ name: span[0], kind: "external", start_ms: round(span[1]), duration_ms: round(span[2] - span[1]) });
      }
    });
    return tx;
  }

  // identify returns what names the element: the event, its label or text, its
  // href, id, class, name and type, and every data- attribute it carries.
  function identify(element, event) {
    var attributes = { event: event };
    if (!element || !element.getAttribute) {
      return attributes;
    }
    var label = element.getAttribute("aria-label") || element.getAttribute("title") || element.getAttribute("value") || text(element);
    if (label) {
      attributes["label"] = label;
    }
    ["href", "id", "class", "name", "type"].forEach(function (name) {
      var value = element.getAttribute(name);
      if (value) {
        attributes[name] = value;
      }
    });
    var own = element.attributes || [];
    for (var i = 0; i < own.length; i++) {
      if (own[i].name.indexOf("data-") === 0) {
        attributes[own[i].name] = own[i].value;
      }
    }
    return attributes;
  }

  function text(element) {
    var value = (element.textContent || "").replace(/\s+/g, " ").trim();
    return value.length > 80 ? value.slice(0, 80) : value;
  }

  // actionable walks up to the element a reader would say was acted on, so a
  // click on the span inside a button names the button.
  function actionable(target) {
    var selector = "a,button,summary,label,input,select,textarea,[role=button],[data-oida]";
    return target && target.closest ? target.closest(selector) || target : target;
  }

  function tagOf(element) {
    return element && element.tagName ? element.tagName.toLowerCase() : "document";
  }

  // start opens an action below the pageload's span and settles the one before
  // it, minting the span here so the action's calls carry it at once.
  function start(event, element) {
    settle();
    var span = hex(8);
    open = { tx: transaction(tagOf(element) + " event=" + event, span, pageSpan, identify(element, event)), span: span, begin: performance.now() };
    schedule();
    return open;
  }

  // schedule settles the open action after the idle period, so a click that
  // fires three calls stays one transaction.
  function schedule() {
    window.clearTimeout(timer);
    timer = window.setTimeout(settle, idle);
  }

  // settle closes the open action, posting it unless the caller is holding
  // the queue for a beacon of its own.
  function settle(hold) {
    window.clearTimeout(timer);
    if (!open) {
      return;
    }
    open.tx.duration_ms = round(performance.now() - open.begin);
    queue.push(open.tx);
    open = null;
    if (hold !== true) {
      flush(false);
    }
  }

  // current returns the span a call made now belongs to.
  function current() {
    return open ? open.span : pageSpan;
  }

  // record puts a call on the action that was open when it started.
  function record(name, span, begin, attributes, error) {
    if (!open || open.span !== span) {
      return;
    }
    open.tx.spans.push({
      name: name,
      kind: "http",
      start_ms: round(begin - open.begin),
      duration_ms: round(performance.now() - begin),
      attributes: attributes,
      error: error || undefined,
    });
    schedule();
  }

  // flush posts what is queued: a beacon from a closing page, which outlives
  // the document, and a keepalive fetch everywhere else, whose receipt says
  // what the recorder made of each transaction.
  function flush(closing) {
    if (queue.length === 0) {
      return;
    }
    var body = JSON.stringify({ transactions: queue.splice(0, queue.length) });
    if (closing && navigator.sendBeacon) {
      navigator.sendBeacon(endpoint, new Blob([body], { type: "text/plain;charset=UTF-8" }));
      return;
    }
    if (!post) {
      return;
    }
    post(endpoint, { method: "POST", body: body, keepalive: true, credentials: "same-origin", headers: { "Content-Type": "application/json" } })
      .then(function (response) {
        return response.ok ? response.json() : null;
      })
      .then(function (receipt) {
        if (receipt && receipt.transactions) {
          window.oidaTrace.recorded = receipt.transactions;
        }
      })
      .catch(function () {
        // A route that is off, or a dashboard that is gone: the page carries on.
      });
  }

  // url parses a request target against this page, null for one that will not.
  function url(value) {
    try {
      return new URL(value && value.url ? value.url : String(value), location.href);
    } catch (err) {
      return null;
    }
  }

  // traced reports whether a call is one to name: same origin, and not the
  // collect route itself.
  function traced(target) {
    return !!target && target.origin === location.origin && target.href.indexOf(endpoint) !== 0 && target.pathname.indexOf(endpoint) !== 0;
  }

  // headers returns the init of a fetch with the traceparent added, leaving
  // whatever the caller passed in place.
  function headers(input, init, span) {
    var given = (init && init.headers) || (input && input.headers) || {};
    var next = {};
    for (var key in init || {}) {
      next[key] = init[key];
    }
    if (typeof Headers === "function") {
      next.headers = new Headers(given);
      next.headers.set("traceparent", traceparent(span));
      next.headers.set("tracestate", state);
      return next;
    }
    next.headers = Object.assign({}, given);
    next.headers["traceparent"] = traceparent(span);
    next.headers["tracestate"] = state;
    return next;
  }

  // patch wraps fetch and XMLHttpRequest, so a same-origin call made while an
  // action is open carries its traceparent and lands on it as a span; a
  // cross-origin one needs the header in the other server's CORS allow list,
  // which is not this library's to arrange.
  function patch() {
    if (post) {
      window.fetch = function (input, init) {
        var target = url(input);
        if (!traced(target)) {
          return post(input, init);
        }
        var span = current();
        var begin = performance.now();
        return post(input, headers(input, init, span)).then(
          function (response) {
            record("fetch " + target.pathname, span, begin, { status: response.status });
            return response;
          },
          function (err) {
            record("fetch " + target.pathname, span, begin, {}, String(err));
            throw err;
          },
        );
      };
    }
    if (!window.XMLHttpRequest) {
      return;
    }
    var prototype = window.XMLHttpRequest.prototype;
    var openNative = prototype.open;
    var sendNative = prototype.send;
    prototype.open = function (method, target) {
      this.oidaCall = { method: String(method), url: url(target) };
      return openNative.apply(this, arguments);
    };
    prototype.send = function () {
      var call = this.oidaCall;
      if (call && traced(call.url)) {
        call.span = current();
        call.begin = performance.now();
        this.setRequestHeader("traceparent", traceparent(call.span));
        this.setRequestHeader("tracestate", state);
        this.addEventListener("loadend", function () {
          record(call.method + " " + call.url.pathname, call.span, call.begin, { status: this.status });
        });
      }
      return sendNative.apply(this, arguments);
    };
  }

  // route records a change of address the browser made without a request: a
  // fragment or a history entry of a single page app. It is a transaction of
  // the session, posted like any other, since no server will see it.
  var last = location.href;
  function route(event) {
    if (location.href === last) {
      return;
    }
    settle();
    var from = last;
    last = location.href;
    var tx = transaction("route " + location.pathname + location.search + location.hash, hex(8), "", {
      event: event,
      url: location.href,
      from: from,
      title: document.title,
    });
    tx.http.uri = location.pathname + location.search + location.hash;
    queue.push(tx);
    flush(false);
  }

  window.oidaTrace = {
    // session is the ULID every transaction of this visit is grouped by; its
    // 128 bits are the trace of every traceparent the page sends.
    session: function () {
      renew();
      return session;
    },

    // recorded is the last receipt: what the recorder made of each transaction.
    recorded: null,

    // traceparent is the header a call made now should carry.
    traceparent: function () {
      return traceparent(current());
    },

    // action opens a transaction for work the delegated listeners cannot see.
    action: function (event, element) {
      return start(event, element || document.body).tx;
    },

    // settle closes the open action, which the idle period does on its own.
    settle: function () {
      settle(false);
    },

    // flush posts what is queued.
    flush: function () {
      flush(false);
    },
  };

  patch();

  // A single page app moves without asking the server: the fragment, and the
  // history API behind its pretty links.
  window.addEventListener("hashchange", function () {
    route("hashchange");
  });
  window.addEventListener("popstate", function () {
    route("popstate");
  });
  ["pushState", "replaceState"].forEach(function (name) {
    var native = window.history && window.history[name];
    if (!native) {
      return;
    }
    window.history[name] = function () {
      var result = native.apply(this, arguments);
      route(name);
      return result;
    };
  });

  // One delegated listener per event, in the capture phase, so neither a
  // handler that stops propagation nor an event that does not bubble can hide
  // an action.
  document.addEventListener(
    "click",
    function (event) {
      var element = actionable(event.target);
      start("click", element);
    },
    true,
  );

  document.addEventListener(
    "submit",
    function (event) {
      start("submit", event.target);
    },
    true,
  );

  // A dialog opened with showModal() fires no event at all, so the open
  // attribute is watched rather than listened for.
  if (window.MutationObserver) {
    new window.MutationObserver(function (records) {
      records.forEach(function (change) {
        if (/^(dialog|details)$/.test(tagOf(change.target))) {
          start(change.target.hasAttribute("open") ? "open" : "close", change.target);
        }
      });
    }).observe(document.documentElement, { subtree: true, attributes: true, attributeFilter: ["open"] });
  }

  // The pageload is queued a tick after the load event, so loadEventEnd is a
  // number rather than the zero it reads while that event runs.
  function queuePageload() {
    window.setTimeout(function () {
      queue.unshift(pageload());
      flush(false);
    }, 0);
  }

  // The page goes away on visibilitychange, the last moment a page reliably
  // observes, with pagehide behind it for browsers reporting no visibility.
  // unload is never used: a listener on it makes the page ineligible for the
  // back/forward cache, and mobile browsers never fire it.
  function closing(reason) {
    // visibilitychange and pagehide both fire as a page is torn down, and
    // the page went away once.
    if (closed) {
      return;
    }
    closed = true;
    settle(true);
    var tx = transaction("close " + location.pathname, hex(8), pageSpan, {
      event: reason,
      url: location.href,
      visible_ms: round(performance.now()),
    });
    queue.push(tx);
    flush(true);
  }

  if (document.readyState === "complete") {
    queuePageload();
  } else {
    window.addEventListener("load", queuePageload);
  }

  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "hidden") {
      closing("hidden");
      return;
    }
    // A tab switched away from and back is one page still being read, so the
    // next teardown is recorded again.
    closed = false;
  });

  window.addEventListener("pagehide", function (event) {
    closing(event && event.persisted ? "cached" : "pagehide");
  });
})();
