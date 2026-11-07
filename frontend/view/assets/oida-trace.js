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

  // handoff is where a click leaves the link it opened, for the next pageload.
  var handoff = "oida.handoff";

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

  function traceparent(span) {
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

  // stored reads the handoff and clears it, null when there is none.
  function stored() {
    try {
      var value = window.sessionStorage.getItem(handoff);
      window.sessionStorage.removeItem(handoff);
      return value ? JSON.parse(value) : null;
    } catch (err) {
      return null;
    }
  }

  // store leaves the handoff for the page the browser goes to next.
  function store(span, href) {
    try {
      window.sessionStorage.setItem(handoff, JSON.stringify({ trace: trace, span: span, tracestate: state, href: href }));
    } catch (err) {
      // Private mode or a quota: the next pageload opens a trace of its own.
    }
  }

  // A traceparent meta tag names the request that served this page, which is
  // how a pageload nests under it; failing that the link a click handed over,
  // failing that a trace of this page's own.
  var served = parse(meta("traceparent"));
  var handed = stored();
  var trace = served ? served.trace : handed && handed.trace ? handed.trace : hex(16);
  var state = served ? meta("tracestate") : handed ? handed.tracestate || "" : "";
  var pageSpan = hex(8);
  var post = typeof window.fetch === "function" ? window.fetch.bind(window) : null;
  var queue = [];
  var open = null;
  var timer = 0;

  function round(value) {
    return Math.round(value * 1000) / 1000;
  }

  function transaction(name, span, parent, attributes) {
    return {
      trace_id: trace,
      span_id: span,
      parent_id: parent || undefined,
      tracestate: state || undefined,
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
    if (handed && handed.href) {
      attributes["from"] = handed.href;
    }
    var tx = transaction("pageload " + location.pathname, pageSpan, served ? served.span : handed ? handed.span : "", attributes);
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
      if (state) {
        next.headers.set("tracestate", state);
      }
      return next;
    }
    next.headers = Object.assign({}, given);
    next.headers["traceparent"] = traceparent(span);
    if (state) {
      next.headers["tracestate"] = state;
    }
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
        if (state) {
          this.setRequestHeader("tracestate", state);
        }
        this.addEventListener("loadend", function () {
          record(call.method + " " + call.url.pathname, call.span, call.begin, { status: this.status });
        });
      }
      return sendNative.apply(this, arguments);
    };
  }

  // navigating reports whether a link takes the browser off this page.
  function navigating(href) {
    return !!href && href.indexOf("#") !== 0 && href.indexOf("javascript:") !== 0;
  }

  window.oidaTrace = {
    // trace is the identifier every transaction of this page shares.
    trace: trace,

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

  // One delegated listener per event, in the capture phase, so neither a
  // handler that stops propagation nor an event that does not bubble can hide
  // an action.
  document.addEventListener(
    "click",
    function (event) {
      var element = actionable(event.target);
      var action = start("click", element);
      var href = element && element.getAttribute ? element.getAttribute("href") : "";
      if (navigating(href)) {
        store(action.span, href);
      }
    },
    true,
  );

  document.addEventListener(
    "submit",
    function (event) {
      var form = event.target;
      store(start("submit", form).span, (form && form.getAttribute && form.getAttribute("action")) || location.pathname);
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
    settle(true);
    var tx = transaction("close " + location.pathname, hex(8), pageSpan, {
      event: reason,
      url: location.href,
      visible_ms: round(performance.now()),
    });
    var handing = stored();
    if (handing) {
      tx.attributes["to"] = handing.href;
      tx.attributes["to.traceparent"] = traceparent(handing.span);
      store(handing.span, handing.href);
    }
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
    }
  });

  window.addEventListener("pagehide", function (event) {
    closing(event && event.persisted ? "cached" : "pagehide");
  });
})();
