#!/usr/bin/env node
// Screenshot components of the running oida front end.
//
// Reads a JSON manifest on stdin and writes one PNG per shot:
//
//   {
//     "base": "http://localhost:8097",
//     "width": 1440, "scale": 2, "theme": "dark", "pad": 14,
//     "shots": [
//       { "out": "docs/assets/header.png", "path": "/debug/oida",
//         "pick": "[q('header.top'), q('.metrics'), q('nav.tabs')]" }
//     ]
//   }
//
// pick is a JavaScript expression run in the page. It returns an element or a
// list of them, and the shot is their bounding box plus a little padding. Three
// helpers are in scope: q(selector), qa(selector) and heading(text), which finds
// an h1 or h2 by what it says. A pick that finds nothing is an error, not an
// empty file: a screenshot of the wrong thing is worse than no screenshot.
//
// Chromium is driven over the DevTools protocol rather than the --screenshot
// flag, because the flag can only capture a whole viewport, and what the
// documentation wants is a component. scripts/chromium.sh is still the right
// tool for looking at a page while working on it.
"use strict";

const { spawn } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const CHROMIUM = process.env.CHROMIUM || "chromium";

main().catch((err) => {
  process.stderr.write(String(err && err.message ? err.message : err) + "\n");
  process.exit(1);
});

async function main() {
  const manifest = JSON.parse(await read(process.stdin));
  const browser = await launch();

  try {
    const page = await connect(browser.target);
    await page.send("Page.enable");
    await page.send("Runtime.enable");

    for (const shot of manifest.shots || []) {
      await capture(page, manifest, shot);
      process.stdout.write(shot.out + "\n");
    }
    page.close();
  } finally {
    browser.stop();
  }
}

// capture takes one shot: load the page, grow the viewport to the whole
// document so nothing is cut off or lazily left undrawn, measure what was asked
// for, and clip to it.
async function capture(page, manifest, shot) {
  const width = shot.width || manifest.width || 1440;
  const scale = shot.scale || manifest.scale || 2;
  const theme = shot.theme || manifest.theme || "dark";
  const pad = shot.pad === undefined ? (manifest.pad === undefined ? 14 : manifest.pad) : shot.pad;

  await page.send("Emulation.setEmulatedMedia", {
    features: [{ name: "prefers-color-scheme", value: theme }],
  });
  await page.send("Emulation.setDeviceMetricsOverride", {
    width: width,
    height: 1200,
    deviceScaleFactor: scale,
    mobile: false,
  });

  const loaded = page.once("Page.loadEventFired", 30000);
  await page.send("Page.navigate", { url: manifest.base + shot.path });
  await loaded;
  await settle(page);

  // The full document as the viewport: every canvas redraws at its final size,
  // and every rect is measured against the same origin.
  const tall = await evaluate(page, "document.documentElement.scrollHeight");
  await page.send("Emulation.setDeviceMetricsOverride", {
    width: width,
    height: Math.max(600, Math.ceil(tall)),
    deviceScaleFactor: scale,
    mobile: false,
  });
  await settle(page);

  const box = await evaluate(page, frame(shot.pick, pad, width));
  if (!box) {
    throw new Error(shot.out + ": nothing matched " + shot.pick);
  }

  const png = await page.send("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
    clip: { x: box.x, y: box.y, width: box.width, height: box.height, scale: 1 },
  });

  fs.mkdirSync(path.dirname(shot.out), { recursive: true });
  fs.writeFileSync(shot.out, Buffer.from(png.data, "base64"));
}

// frame is the expression that measures a shot: the union of what pick found,
// padded, and held inside the page.
function frame(pick, pad, width) {
  return `(() => {
    const q = (s) => document.querySelector(s);
    const qa = (s) => Array.from(document.querySelectorAll(s));
    const heading = (t) => qa("h1,h2").find((h) => h.textContent.trim() === t);
    const found = [].concat(${pick}).filter(Boolean);
    if (!found.length) return null;

    const boxes = found.map((el) => el.getBoundingClientRect());
    const left = Math.min(...boxes.map((b) => b.left)) + window.scrollX - ${pad};
    const top = Math.min(...boxes.map((b) => b.top)) + window.scrollY - ${pad};
    const right = Math.max(...boxes.map((b) => b.right)) + window.scrollX + ${pad};
    const bottom = Math.max(...boxes.map((b) => b.bottom)) + window.scrollY + ${pad};

    return {
      x: Math.max(0, Math.round(left)),
      y: Math.max(0, Math.round(top)),
      width: Math.min(${width}, Math.round(right - Math.max(0, left))),
      height: Math.round(bottom - Math.max(0, top)),
    };
  })()`;
}

// settle waits for the page to stop moving: two frames for anything the layout
// still owes, and a beat for the canvas timelines to finish drawing.
function settle(page) {
  return evaluate(
    page,
    "new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(() => setTimeout(done, 150))))",
    true
  );
}

async function evaluate(page, expression, awaitPromise) {
  const result = await page.send("Runtime.evaluate", {
    expression: expression,
    returnByValue: true,
    awaitPromise: !!awaitPromise,
  });
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.text + ": " + expression.slice(0, 120));
  }
  return result.result.value;
}

// launch starts a headless chromium and returns the socket of its first page.
async function launch() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "oida-shot-"));
  const child = spawn(
    CHROMIUM,
    [
      "--headless",
      "--no-sandbox",
      "--disable-gpu",
      "--hide-scrollbars",
      "--remote-debugging-port=0",
      "--user-data-dir=" + dir,
      "about:blank",
    ],
    { stdio: "ignore" }
  );

  // Chromium is still flushing its profile as it goes down, so the directory
  // takes a second attempt often enough to be worth not failing over.
  const stop = () => {
    child.kill();
    for (let attempt = 0; attempt < 5; attempt++) {
      try {
        fs.rmSync(dir, { recursive: true, force: true, maxRetries: 5, retryDelay: 200 });
        return;
      } catch (err) {
        // still holding it open
      }
    }
  };

  try {
    const port = await waitFor(() => {
      const file = path.join(dir, "DevToolsActivePort");
      return fs.existsSync(file) ? fs.readFileSync(file, "utf8").split("\n")[0] : null;
    }, "chromium did not open a debugging port");

    const target = await waitFor(async () => {
      const response = await fetch("http://127.0.0.1:" + port + "/json/list");
      const page = (await response.json()).find((entry) => entry.type === "page");
      return page ? page.webSocketDebuggerUrl : null;
    }, "chromium opened no page to drive");

    return { target: target, stop: stop };
  } catch (err) {
    stop();
    throw err;
  }
}

// waitFor polls until the probe returns something, or gives up saying what it
// was waiting for.
async function waitFor(probe, complaint) {
  for (let attempt = 0; attempt < 100; attempt++) {
    try {
      const value = await probe();
      if (value) {
        return value;
      }
    } catch (err) {
      // not up yet
    }
    await new Promise((done) => setTimeout(done, 100));
  }
  throw new Error(complaint);
}

// connect opens the DevTools socket and wraps it as request and response, plus
// a way to wait for one event.
function connect(url) {
  return new Promise((resolve, reject) => {
    const socket = new WebSocket(url);
    const pending = new Map();
    const waiting = [];
    let last = 0;

    const api = {
      send(method, params) {
        const id = ++last;
        return new Promise((done, fail) => {
          pending.set(id, { done: done, fail: fail });
          socket.send(JSON.stringify({ id: id, method: method, params: params || {} }));
        });
      },
      once(method, timeout) {
        return new Promise((done, fail) => {
          const waiter = { method: method, done: done };
          waiting.push(waiter);
          setTimeout(() => {
            const at = waiting.indexOf(waiter);
            if (at >= 0) {
              waiting.splice(at, 1);
              fail(new Error("timed out waiting for " + method));
            }
          }, timeout || 30000);
        });
      },
      close() {
        socket.close();
      },
    };

    socket.addEventListener("open", () => resolve(api));
    socket.addEventListener("error", () => reject(new Error("devtools socket refused")));
    socket.addEventListener("message", (event) => {
      const message = JSON.parse(event.data);

      if (message.id !== undefined) {
        const call = pending.get(message.id);
        if (!call) {
          return;
        }
        pending.delete(message.id);
        if (message.error) {
          call.fail(new Error(message.error.message));
        } else {
          call.done(message.result);
        }
        return;
      }

      for (let at = waiting.length - 1; at >= 0; at--) {
        if (waiting[at].method === message.method) {
          waiting.splice(at, 1)[0].done(message.params);
        }
      }
    });
  });
}

function read(stream) {
  return new Promise((done, fail) => {
    let text = "";
    stream.setEncoding("utf8");
    stream.on("data", (chunk) => (text += chunk));
    stream.on("end", () => done(text));
    stream.on("error", fail);
  });
}
