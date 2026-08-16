// Progressive enhancement only: every page works with this file blocked.
(function () {
  "use strict";

  // ------------------------------------------------------------ live stream
  var root = document.getElementById("oida-live");
  if (root && typeof window.EventSource === "function" && root.getAttribute("data-events")) {
    var status = document.getElementById("oida-stream-status");
    var source = new EventSource(root.getAttribute("data-events"));
    var first = true;

    source.onmessage = function (event) {
      if (first) {
        root.innerHTML = event.data;
        first = false;
      } else {
        apply(root, event.data);
      }
      setStatus("streaming", "live");
    };

    source.onerror = function () {
      // EventSource reconnects on its own; only the label changes.
      setStatus("reconnecting", "down");
    };

    window.addEventListener("beforeunload", function () {
      source.close();
    });
  }

  // The server renders the whole section every time, but replacing it wholesale
  // makes the page blink and throws away scroll position, hover and any open
  // disclosure. So: parse it, merge the feed row by row against its trace id,
  // and touch nothing that has not changed.
  function apply(root, html) {
    var incoming = document.createElement("div");
    incoming.innerHTML = html;

    mergeRows(root.querySelector("#oida-feed"), incoming.querySelector("#oida-feed"));
    replaceIfChanged(root.querySelector("#oida-state"), incoming.querySelector("#oida-state"));
  }

  function mergeRows(current, next) {
    if (!current || !next) {
      return;
    }

    var rows = Array.prototype.slice.call(next.children);
    var keyed = rows.length > 0 && rows[0].hasAttribute("data-key");
    if (!keyed) {
      // The empty state, or markup this merge does not understand.
      current.innerHTML = next.innerHTML;
      return;
    }

    var existing = {};
    Array.prototype.forEach.call(current.children, function (row) {
      var key = row.getAttribute("data-key");
      if (key) {
        existing[key] = row;
      }
    });

    rows.forEach(function (row, index) {
      var key = row.getAttribute("data-key");
      var node = existing[key];

      if (node) {
        // A running trace changes as it goes; a finished one never does.
        if (node.outerHTML !== row.outerHTML) {
          node.replaceWith(row);
          node = row;
        }
        delete existing[key];
      } else {
        node = row;
        node.classList.add("fresh");
      }

      if (current.children[index] !== node) {
        current.insertBefore(node, current.children[index] || null);
      }
    });

    Object.keys(existing).forEach(function (key) {
      existing[key].remove();
    });
  }

  function replaceIfChanged(current, next) {
    if (current && next && current.innerHTML !== next.innerHTML) {
      current.innerHTML = next.innerHTML;
    }
  }

  function setStatus(text, state) {
    // The section is replaced wholesale, so the node is looked up every time.
    var node = document.getElementById("oida-stream-status");
    if (node) {
      node.textContent = text;
      node.className = "stream-status " + state;
    }
  }

  // -------------------------------------------------------- copy trace ids
  document.addEventListener("click", function (event) {
    var button = event.target.closest("[data-copy]");
    if (!button) {
      return;
    }
    event.preventDefault();

    var value = button.getAttribute("data-copy");
    var done = function () {
      var label = button.textContent;
      button.textContent = "✓";
      button.classList.add("done");
      setTimeout(function () {
        button.textContent = label;
        button.classList.remove("done");
      }, 900);
    };

    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(value).then(done, fallback);
    } else {
      fallback();
    }

    function fallback() {
      var field = document.createElement("textarea");
      field.value = value;
      field.setAttribute("readonly", "");
      field.style.position = "fixed";
      field.style.opacity = "0";
      document.body.appendChild(field);
      field.select();
      try {
        document.execCommand("copy");
        done();
      } catch (err) {
        /* leave the value selected for a manual copy */
      }
      document.body.removeChild(field);
    }
  });

  // ------------------------------------------------------------ row links
  // The whole row is the target. Links, buttons and selected text still win,
  // so copying a trace id does not navigate away from it.
  document.addEventListener("click", function (event) {
    var row = event.target.closest("tr[data-href]");
    if (!row || event.target.closest("a, button, summary, input, label")) {
      return;
    }
    if (String(window.getSelection())) {
      return;
    }

    var href = row.getAttribute("data-href");
    if (event.metaKey || event.ctrlKey || event.button === 1) {
      window.open(href, "_blank");
    } else {
      window.location.href = href;
    }
  });

  // --------------------------------------------------------- linked spans
  // A span appears twice on the detail page: as a block on the timeline and as
  // a row in the table. Hovering either one lights both.
  document.addEventListener("mouseover", function (event) {
    var node = event.target.closest("[data-span]");
    link(node && node.getAttribute("data-span"));
  });

  document.addEventListener("mouseleave", function () {
    link(null);
  });

  function link(id) {
    document.querySelectorAll("[data-span].linked").forEach(function (node) {
      node.classList.remove("linked");
    });
    if (!id) {
      return;
    }
    document.querySelectorAll('[data-span="' + id + '"]').forEach(function (node) {
      node.classList.add("linked");
    });
  }

  // -------------------------------------------------------------- selects
  // The markup is rendered by the server; this only opens the list, moves the
  // highlight, and writes the chosen value back into the hidden input.
  var selects = document.querySelectorAll("[data-select]");

  selects.forEach(function (select) {
    var button = select.querySelector(".select-button");
    var value = select.querySelector("input[type=hidden]");
    var menu = select.querySelector(".select-menu");
    var options = Array.prototype.slice.call(menu.querySelectorAll("li"));

    button.addEventListener("click", function (event) {
      event.stopPropagation();
      toggle(select, menu.hidden);
    });

    button.addEventListener("keydown", function (event) {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        toggle(select, true);
        highlight(options, event.key === "ArrowDown" ? 0 : options.length - 1);
      }
    });

    options.forEach(function (option, index) {
      option.addEventListener("click", function () {
        choose(select, value, option);
      });
      option.addEventListener("mouseenter", function () {
        highlight(options, index);
      });
      option.addEventListener("keydown", function (event) {
        switch (event.key) {
          case "Enter":
          case " ":
            event.preventDefault();
            choose(select, value, option);
            break;
          case "ArrowDown":
            event.preventDefault();
            highlight(options, Math.min(index + 1, options.length - 1));
            break;
          case "ArrowUp":
            event.preventDefault();
            if (index === 0) {
              toggle(select, false);
              button.focus();
            } else {
              highlight(options, index - 1);
            }
            break;
          case "Escape":
            toggle(select, false);
            button.focus();
            break;
          case "Tab":
            toggle(select, false);
            break;
        }
      });
    });
  });

  function toggle(select, open) {
    // One menu at a time.
    selects.forEach(function (other) {
      if (other !== select) {
        close(other);
      }
    });

    var menu = select.querySelector(".select-menu");
    var button = select.querySelector(".select-button");
    menu.hidden = !open;
    button.setAttribute("aria-expanded", open ? "true" : "false");
    if (open) {
      select.setAttribute("data-open", "");
    } else {
      select.removeAttribute("data-open");
    }
  }

  function close(select) {
    select.querySelector(".select-menu").hidden = true;
    select.querySelector(".select-button").setAttribute("aria-expanded", "false");
    select.removeAttribute("data-open");
  }

  function highlight(options, index) {
    options.forEach(function (option) {
      option.classList.remove("active");
    });
    if (options[index]) {
      options[index].classList.add("active");
      options[index].focus();
    }
  }

  function choose(select, value, option) {
    value.value = option.getAttribute("data-value");
    close(select);

    var form = select.closest("form");
    if (form) {
      form.submit();
    }
  }

  document.addEventListener("click", function () {
    selects.forEach(close);
  });

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") {
      selects.forEach(close);
    }
  });

  // ---------------------------------------------------------------- peeking
  // Long tables are behind a switch. The markup and the stylesheet do the
  // opening; this only remembers the choice, and remembers it for the whole
  // front end rather than for the page it was made on, so a reader who wants
  // the spans shut has them shut on every trace they open.
  Array.prototype.forEach.call(document.querySelectorAll("input[data-peek]"), function (input) {
    var name = "oida.peek." + input.getAttribute("data-peek");
    input.checked = remember(name) === "on";
    input.addEventListener("change", function () {
      remember(name, input.checked ? "on" : "off");
    });
  });

  // remember reads a setting, or writes one when it is given a value. Storage
  // is not always there: a private window, a sandboxed frame, a browser with it
  // switched off. The front end works without it, it just forgets.
  function remember(name, value) {
    try {
      if (value === undefined) {
        return window.localStorage.getItem(name);
      }
      window.localStorage.setItem(name, value);
    } catch (err) {
      // Nothing to do about it: the setting lives for this page only.
    }
    return null;
  }

  // ------------------------------------------------------------- shortcuts
  document.addEventListener("keydown", function (event) {
    var field = document.getElementById("oida-filter");
    var typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName);

    if (event.key === "/" && !typing && field) {
      event.preventDefault();
      field.focus();
      field.select();
      return;
    }

    if (event.key === "Escape" && document.activeElement === field) {
      field.blur();
    }
  });
})();
