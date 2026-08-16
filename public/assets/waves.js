// EXPERIMENT: the waves timeline.
//
// Every span in the trace is drawn as one waveform, in the colour of its kind,
// laid over a single shared centre line. A wave begins where its span began and
// ends where it ended, so the width is time and the stack of waves standing at
// any point is what was open at that moment. Six spans running at once is six
// waves crossing, and where they cross the ink piles up: the busy stretch of a
// trace is visibly the hot one, without a legend saying so.
//
// The shape inside a wave is not data, and this file would rather say so than
// imply otherwise. A span knows when it ran, how deep it nested and whether it
// failed; it does not carry a per-millisecond signal, and inventing one and
// plotting it as though it were measured would be a lie told in a pretty font.
// So the hair is generated, from a hash of the span, and it is generated the
// same way every time: the same trace draws the same picture, on every render
// and at every width. Nothing here shuffles when the window moves.
//
// What every wave does agree on is the rhythm. One envelope of five swells is
// built for the whole chart and every wave is modulated by it, so the drawing
// reads as several takes of the same music rather than a heap of unrelated
// noise. That envelope is in turn modulated by how many spans were open at each
// point, which is measured: the loud parts of the chart are the busy parts of
// the trace, and the two agreeing is what makes the picture worth looking at.
//
// Depth is loudness. The request covers the whole width, and drawn as loud as
// the query inside it, it would drown everything under one green wash. A span's
// gain rises with how deep it nests and falls with how much of the width it
// holds, so the request is a quiet broad body and the work inside it is the
// bright spikes standing in it.
//
// Blending is the other half of the drawing, and it cannot be the same on both
// palettes. On the dark one the waves are added, the way light adds, and two
// colours crossing give a hotter third. Adding on the light palette gives white
// mud, so there the waves are multiplied, the way ink on paper does, and the
// same crossing gives a deeper third. One regime of alphas and tints per theme,
// chosen when the palette is read, and the whole thing redrawn when it changes.
//
// A failure is marked, not shouted: a dashed rule in the bad colour along the
// centre for exactly the stretch the failed span held, with a tick at each end.
//
// Static. It draws once, and again on a resize or a change of palette. Nothing
// animates.
//
// Delete this file with view_waves.templ to remove the experiment.
(function () {
  "use strict";

  var canvas = document.getElementById("oida-waves");
  var payload = document.getElementById("oida-waves-data");
  if (!canvas || !payload) {
    return;
  }

  var ctx = canvas.getContext ? canvas.getContext("2d") : null;
  if (!ctx) {
    return;
  }

  var trace = { ms: 0, depth: 0, spans: [] };
  try {
    trace = JSON.parse(payload.textContent) || trace;
  } catch (err) {
    return;
  }
  var spans = trace.spans || [];

  var STEP = 1.15; // css pixels between one hairline sample and the next
  var HAIR = 0.8; // and how wide one is drawn, so they never quite close up
  var PAD = 4; // clear air above and below the tallest wave
  var PEAKS = 5; // big swells shared by every wave across the whole chart
  var MIN_LEN = 0.005; // a span too fast to have a width still gets a spike
  var BUSY = 0.12; // of the shared envelope standing whatever the load
  var FLOOR = 0.52; // and of a wave surviving the lull between the swells
  var ROLL = 0.018; // of the chart a wave takes to fade in, and to fade out
  var BLOOM = 4; // most spotlights lit on one wave
  var TICKS = 32; // ruler marks along the top and bottom edge
  var SEED = 1; // rolled into every hash, so one nudge reshapes the lot

  // draw is the whole of it: read the box, read the palette, build the shared
  // rhythm, then lay one wave per span over it.
  function draw() {
    var ratio = window.devicePixelRatio || 1;
    var box = canvas.getBoundingClientRect();
    canvas.width = Math.max(1, Math.round(box.width * ratio));
    canvas.height = Math.max(1, Math.round(box.height * ratio));
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);

    var width = box.width;
    var height = box.height;
    ctx.globalCompositeOperation = "source-over";
    ctx.globalAlpha = 1;
    ctx.clearRect(0, 0, width, height);

    if (!spans.length || !(trace.ms > 0) || width < 8 || height < 8) {
      return; // an empty trace draws an empty box, and nothing else happens
    }

    var skin = palette();
    var count = Math.max(2, Math.round(width / STEP));
    var reach = Math.max(2, height / 2 - PAD);
    var middle = height / 2;

    // The waves are multiplied on the light palette, and multiplying against a
    // transparent canvas is the same as covering it. So the ground the ink
    // needs is painted first, in the colour the box behind the canvas already
    // is: invisible, and the difference between blended waves and stacked ones.
    ctx.fillStyle = ink(skin.back, 1);
    ctx.fillRect(0, 0, width, height);
    ruler(skin, width, height, middle, reach);

    var shared = rhythm(count);
    var levels = Math.max(1, trace.depth || deepest());

    // Shallow first: the broad quiet bodies go down before the bright spikes
    // that stand in them, which is the order that reads on the light palette
    // where the blending is not commutative in the eye even where it is in the
    // arithmetic.
    var order = spans.slice().sort(function (a, b) {
      return (a.depth || 0) - (b.depth || 0);
    });

    // Measured before anything is drawn, so the whole drawing can be lifted to
    // fill the box it was given. A trace where no two spans ever overlap builds
    // a quieter chart than a busy one, and the quiet one should still look like
    // a recording rather than a scratch along the middle. The lift is capped,
    // or a trace with nothing in it would be amplified into pure hash.
    var waves = [];
    var loudest = 0;
    order.forEach(function (span, index) {
      var one = shape(span, index, shared, count, levels);
      if (one) {
        waves.push(one);
        loudest = Math.max(loudest, one.top);
      }
    });
    var lift = loudest > 0 ? Math.min(0.97 / loudest, 2.4) : 1;

    ctx.globalCompositeOperation = skin.blend;
    waves.forEach(function (one) {
      paint(one, skin, count, width, middle, reach * lift);
    });

    // Failures are drawn back in plain covering ink. A mark that says something
    // went wrong is the one mark in here that must not be softened by whatever
    // happens to be underneath it.
    ctx.globalCompositeOperation = "source-over";
    order.forEach(function (span) {
      if (span.failed) {
        fault(span, skin, width, middle, reach);
      }
    });

    ctx.globalAlpha = 1;
    ctx.setLineDash([]);
    ctx.shadowBlur = 0;
  }

  // shape works out the body of one wave: how tall it is at every sample column
  // of the chart, and nothing about how it is drawn.
  function shape(span, index, shared, count, levels) {
    var start = clamp(span.start || 0, 0, 1);
    var end = clamp(span.end || 0, 0, 1);
    var length = Math.max(end - start, MIN_LEN);
    if (start + length > 1) {
      start = Math.max(0, 1 - length);
    }

    var color = parse(span.color) || parse("#6b7785");
    var seed = index * 137.31 + start * 911.7 + length * 331.1 + SEED;

    // How loud this one gets: deeper is louder, broader is quieter. The root
    // holds the whole width and comes out around a fifth of the height, which
    // is a wash; a query three levels down holding a tenth of the width comes
    // out at the ceiling, which is a spike.
    var deep = clamp((span.depth || 0) / levels, 0, 1);
    var gain = (0.42 + 0.58 * deep) * (0.62 + 0.38 * (1 - clamp(length, 0, 1)));

    var from = Math.max(0, Math.floor(start * count) - 1);
    var to = Math.min(count - 1, Math.ceil((start + length) * count) + 1);
    if (to <= from) {
      return null;
    }

    // The body of the wave, sampled once and then used four times over. Three
    // grains go into it over the shared swells: a broad one that gives the wave
    // its own hills between them, a finer one for the small peaks and drops in
    // between, and a slow gate that opens real dropouts, because a waveform
    // that never falls silent reads as a hedge rather than a recording.
    var body = new Array(count);
    var top = 0;
    for (var i = from; i <= to; i++) {
      var t = (i + 0.5) / count;
      var u = (t - start) / length;
      var open = taper(u, length);
      if (open <= 0) {
        body[i] = 0;
        continue;
      }
      var broad = harden(fbm(t * 7 + seed, seed, 2));
      var fine = fbm(t * 34 + seed, seed + 27, 3);
      var gate = smoothstep(0.17, 0.56, wobble(t * 5.3 + seed * 0.5, seed + 61));
      // The grains swing half again as far as the steady part of the body they
      // ride on, which is the difference between a wave that breathes and a
      // wave that hums. The mean is held where it was, so widening the swing
      // makes the quiet parts quieter and the loud parts louder rather than
      // just turning the whole thing up.
      var value =
        shared.at[i] *
        open *
        gain *
        (0.3 + 0.48 * broad + 0.36 * fine) *
        (0.07 + 0.93 * gate);
      body[i] = value;
      if (value > top) {
        top = value;
      }
    }
    if (top <= 0.004) {
      return null; // too quiet to be worth an argument with the antialiaser
    }
    return { body: body, top: top, from: from, to: to, color: color, seed: seed, span: span };
  }

  // paint draws one wave: the smooth body, the hair inside it, a hot core along
  // the centre, the catching edge on the outline, the spotlights at its peaks
  // and the spray of sparks around them.
  function paint(wave, skin, count, width, middle, reach) {
    var body = wave.body;
    var from = wave.from;
    var to = wave.to;
    var top = wave.top;
    var color = wave.color;
    var seed = wave.seed;

    var x0 = from * (width / count);
    var x1 = (to + 1) * (width / count);
    var tint = spectrum(color, x0, x1, seed, skin);

    // The body: a smooth translucent shape mirrored about the centre, which is
    // what gives the drawing something to be dense against. The hair alone is
    // a cloud; the hair over a body is a waveform.
    ctx.globalAlpha = skin.fill;
    ctx.fillStyle = tint;
    ctx.beginPath();
    for (var a = from; a <= to; a++) {
      var xa = (a + 0.5) * (width / count);
      var ya = middle - body[a] * reach;
      if (a === from) {
        ctx.moveTo(xa, ya);
      } else {
        ctx.lineTo(xa, ya);
      }
    }
    for (var b = to; b >= from; b--) {
      ctx.lineTo((b + 0.5) * (width / count), middle + body[b] * reach);
    }
    ctx.closePath();
    ctx.fill();

    // The hair: one upright hairline per sample column, cut short by a per
    // column roll so no two neighbours agree. The roll leans hard toward the
    // full height, because the tips of the hair are what the eye reads as the
    // shape of the wave: a roll spread evenly leaves the outline hovering over
    // a thin fuzz, which reads as two drawings rather than one. Top and bottom
    // roll separately, since a waveform that is exactly symmetric looks printed
    // rather than recorded, and neither reaches past the body, so the outline
    // stays a true edge.
    ctx.globalAlpha = skin.hair;
    ctx.strokeStyle = tint;
    ctx.lineWidth = HAIR;
    ctx.lineCap = "butt";
    ctx.beginPath();
    for (var h = from; h <= to; h++) {
      if (body[h] <= 0) {
        continue;
      }
      var xh = (h + 0.5) * (width / count);
      var up = body[h] * bite(h * 7.71 + seed);
      var down = body[h] * bite(h * 3.13 + seed + 90);
      ctx.moveTo(xh, middle - up * reach);
      ctx.lineTo(xh, middle + down * reach);
    }
    ctx.stroke();

    // The core: the same hair again, kept near the centre and drawn in a hotter
    // tint. Every waveform worth looking at has a bright seam down the middle
    // where the samples are always present. It is held to a slice of the body
    // rather than a fixed height, so a quiet stretch does not get the same hot
    // line as a loud one, which is what turns the whole centre white.
    ctx.globalAlpha = skin.core;
    ctx.strokeStyle = ink(skin.hot(color), 1);
    ctx.lineWidth = HAIR;
    ctx.beginPath();
    for (var c = from; c <= to; c++) {
      if (body[c] <= top * 0.42) {
        continue;
      }
      var xc = (c + 0.5) * (width / count);
      var seam = body[c] * (0.02 + 0.06 * noise(c * 11.7 + seed + 5));
      ctx.moveTo(xc, middle - seam * reach);
      ctx.lineTo(xc, middle + seam * reach);
    }
    ctx.stroke();

    // The edge: the outline of the body, in a tint that catches the light,
    // with a short glow hung off it. This is the line that makes a wave read
    // as a solid thing rather than a smudge, and it is the one place a little
    // shadow work is worth what it costs.
    ctx.globalAlpha = skin.edge;
    ctx.strokeStyle = ink(skin.rim(color), 1);
    ctx.lineWidth = 0.9;
    ctx.lineJoin = "round";
    ctx.shadowBlur = skin.glow;
    ctx.shadowColor = ink(skin.hot(color), skin.halo);
    for (var side = 0; side < 2; side++) {
      ctx.beginPath();
      for (var e = from; e <= to; e++) {
        var xe = (e + 0.5) * (width / count);
        var ye = middle + (side ? body[e] : -body[e]) * reach;
        if (e === from) {
          ctx.moveTo(xe, ye);
        } else {
          ctx.lineTo(xe, ye);
        }
      }
      ctx.stroke();
    }
    ctx.shadowBlur = 0;

    crests(body, from, to, count, top).forEach(function (peak, rank) {
      var xp = (peak + 0.5) * (width / count);
      // Held under the height of the box and under the width of the wave that
      // lit it. A very short span comes out loud, and a bloom scaled straight
      // off its height would be a headlight sitting in open ground.
      var room = Math.min(reach * 1.1, (x1 - x0) * 0.34);
      var size = clamp(body[peak] * reach * skin.spot, 8, Math.max(8, room));

      // The spotlight: a soft round bloom sitting on the loudest points, the
      // part of the picture that is pure pleasure and no information. It fades
      // to nothing at its edge, which is the identity for adding and for
      // multiplying both, so the same gradient serves either palette.
      var bloom = ctx.createRadialGradient(xp, middle, 0, xp, middle, size);
      bloom.addColorStop(0, ink(skin.hot(color), skin.bloom));
      bloom.addColorStop(0.45, ink(color, skin.bloom * 0.5));
      bloom.addColorStop(1, ink(color, 0));
      ctx.globalAlpha = 1;
      ctx.fillStyle = bloom;
      ctx.beginPath();
      ctx.arc(xp, middle, size, 0, Math.PI * 2);
      ctx.fill();

      sparks(peak, rank, body, count, width, middle, reach, color, skin, seed);
    });

    ctx.globalAlpha = 1;
  }

  // sparks scatters a handful of tiny dots around one peak, some inside the
  // body and some thrown clear of it. A waveform this loud should look like it
  // is shedding something.
  function sparks(peak, rank, body, count, width, middle, reach, color, skin, seed) {
    var key = seed + peak * 17.3 + rank * 53.9;
    var many = 5 + Math.floor(noise(key) * 6);

    ctx.fillStyle = ink(skin.hot(color), skin.spark);
    for (var i = 0; i < many; i++) {
      var drift = (noise(key + i * 3.7) - 0.5) * 2;
      var at = Math.round(peak + drift * count * 0.012);
      if (at < 0 || at >= count || !body[at]) {
        continue;
      }
      var lift = (noise(key + i * 9.1 + 4) - 0.5) * 2;
      var far = 0.35 + 1.05 * noise(key + i * 5.3 + 8);
      ctx.beginPath();
      ctx.arc(
        (at + 0.5) * (width / count),
        middle + Math.sign(lift || 1) * body[at] * far * reach,
        0.55 + noise(key + i * 2.1 + 12) * 0.75,
        0,
        Math.PI * 2
      );
      ctx.fill();
    }
  }

  // fault marks a span that recorded an error: a dashed rule along the centre
  // for exactly the stretch it held, ticked at both ends. It says where and for
  // how long, which is what a reader wants, and it says it in one colour.
  function fault(span, skin, width, middle, reach) {
    var x0 = clamp(span.start || 0, 0, 1) * width;
    var x1 = Math.max(x0 + 2, clamp(span.end || 0, 0, 1) * width);
    var tick = Math.min(6, reach * 0.16);

    // Laid over a hot core seam, a hairline in any colour is a hairline nobody
    // sees. It gets a wider stroke of the ground colour under it first, which
    // is the cheapest way to keep one mark readable over anything at all.
    ctx.globalAlpha = 0.45;
    ctx.strokeStyle = ink(skin.back, 1);
    ctx.lineWidth = 3;
    ctx.setLineDash([]);
    ctx.beginPath();
    ctx.moveTo(x0, middle);
    ctx.lineTo(x1, middle);
    ctx.stroke();

    ctx.globalAlpha = 0.95;
    ctx.strokeStyle = ink(skin.bad, 1);
    ctx.lineWidth = 1.25;
    ctx.setLineDash([3, 3]);
    ctx.beginPath();
    ctx.moveTo(x0, middle);
    ctx.lineTo(x1, middle);
    ctx.stroke();

    ctx.setLineDash([]);
    ctx.beginPath();
    ctx.moveTo(x0 + 0.5, middle - tick);
    ctx.lineTo(x0 + 0.5, middle + tick);
    ctx.moveTo(x1 - 0.5, middle - tick);
    ctx.lineTo(x1 - 0.5, middle + tick);
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  // ruler draws the chrome: the centre line the waves are mirrored about, and
  // marks along both edges. It is the only thing here taking its colour from
  // the stylesheet, and it is drawn under everything.
  function ruler(skin, width, height, middle, reach) {
    ctx.strokeStyle = ink(skin.line, 1);
    ctx.lineWidth = 1;
    ctx.beginPath();
    for (var i = 0; i <= TICKS; i++) {
      var x = Math.round((i / TICKS) * (width - 1)) + 0.5;
      var long = i % 4 === 0 ? 5 : 2.5;
      ctx.moveTo(x, 0);
      ctx.lineTo(x, long);
      ctx.moveTo(x, height);
      ctx.lineTo(x, height - long);
    }
    ctx.stroke();

    ctx.strokeStyle = ink(skin.line2, 1);
    ctx.globalAlpha = 0.7;
    ctx.beginPath();
    ctx.moveTo(0, Math.round(middle) + 0.5);
    ctx.lineTo(width, Math.round(middle) + 0.5);
    ctx.stroke();
    ctx.globalAlpha = 1;

    // Two faint rails at the ceiling the waves are scaled against, so a loud
    // stretch has something to be measured by.
    ctx.strokeStyle = ink(skin.line, 1);
    ctx.globalAlpha = 0.55;
    ctx.beginPath();
    ctx.moveTo(0, Math.round(middle - reach) + 0.5);
    ctx.lineTo(width, Math.round(middle - reach) + 0.5);
    ctx.moveTo(0, Math.round(middle + reach) + 0.5);
    ctx.lineTo(width, Math.round(middle + reach) + 0.5);
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  // rhythm builds the envelope every wave is modulated by: five gaussian swells
  // laid across the width, multiplied by how many spans were open at each
  // point. The swells are what make the waves read as takes of one piece of
  // music; the concurrency is what keeps the loud parts honest.
  function rhythm(count) {
    var seed = spans.length * 7.7 + trace.ms * 0.31 + SEED;
    var crest = [];
    for (var p = 0; p < PEAKS; p++) {
      crest.push({
        at: (p + 0.5) / PEAKS + (noise(seed + p * 17.1) - 0.5) * (0.62 / PEAKS),
        wide: 0.038 + noise(seed + p * 29.3) * 0.05,
        gain: 0.5 + noise(seed + p * 53.7) * 0.5,
      });
    }

    var swell = new Array(count);
    var busy = new Array(count);
    var loudest = 0;
    var fullest = 0;

    for (var i = 0; i < count; i++) {
      var t = (i + 0.5) / count;
      var sum = 0;
      for (var c = 0; c < crest.length; c++) {
        var d = (t - crest[c].at) / crest[c].wide;
        sum += crest[c].gain * Math.exp(-0.5 * d * d);
      }
      swell[i] = sum;
      loudest = Math.max(loudest, sum);

      var open = 0;
      for (var s = 0; s < spans.length; s++) {
        if (t >= (spans[s].start || 0) && t <= (spans[s].end || 0)) {
          open++;
        }
      }
      busy[i] = open;
      fullest = Math.max(fullest, open);
    }

    busy = soften(busy, Math.max(1, Math.round(count / 90)));
    var at = new Array(count);
    for (var k = 0; k < count; k++) {
      var peak = loudest > 0 ? swell[k] / loudest : 0;
      var load = fullest > 0 ? clamp(busy[k] / fullest, 0, 1) : 1;
      at[k] = (BUSY + (1 - BUSY) * peak) * (FLOOR + (1 - FLOOR) * load);
    }
    return { at: at };
  }

  // crests picks the points a wave is loudest at, keeping them apart so four
  // spotlights do not land on one hill. Only the tall ones are lit.
  function crests(body, from, to, count, top) {
    var apart = Math.max(3, Math.round(count / 26));
    var found = [];
    for (var i = from + 1; i < to; i++) {
      if (body[i] < top * 0.62 || body[i] < body[i - 1] || body[i] < body[i + 1]) {
        continue;
      }
      var clear = true;
      for (var f = 0; f < found.length; f++) {
        if (Math.abs(found[f] - i) < apart) {
          clear = body[i] > body[found[f]];
          if (clear) {
            found.splice(f, 1);
          }
          break;
        }
      }
      if (clear) {
        found.push(i);
      }
    }
    found.sort(function (a, b) {
      return body[b] - body[a];
    });
    return found.slice(0, BLOOM);
  }

  // spectrum tints a wave along its own width. The colour stays the kind's
  // colour, but the hue is walked a little either side of it and the middle is
  // lifted, which is what stops a long wave from reading as one flat swatch and
  // gives the drawing the run of colour a spectrogram has.
  function spectrum(color, x0, x1, seed, skin) {
    var turn = (noise(seed + 3.3) - 0.5) * 2;
    var grad = ctx.createLinearGradient(x0, 0, Math.max(x0 + 1, x1), 0);
    var stops = [0, 0.28, 0.5, 0.72, 1];
    var walk = [-1, -0.35, 0.15, 0.6, 1.1];
    for (var i = 0; i < stops.length; i++) {
      var lift = 1 - Math.abs(stops[i] - 0.5) * 2;
      grad.addColorStop(
        stops[i],
        ink(shift(color, walk[i] * turn * 0.075, 0.08 * lift, skin.lift * lift), 1)
      );
    }
    return grad;
  }

  // palette reads the stylesheet once and settles every question the theme
  // decides: how the ink combines, how much of it there is, and which way a
  // tint has to move to look hot. Adding is right on the dark palette and turns
  // the light one to mud, so the light palette multiplies and its hot tints go
  // deeper and more saturated rather than paler.
  function palette() {
    var style = getComputedStyle(canvas);
    var dark = !media || media.matches;

    var skin = {
      back: parse(behind()) || parse(varOf(style, "--ink-2")) || parse("#10161d"),
      line: parse(varOf(style, "--line")) || parse("#1c2630"),
      line2: parse(varOf(style, "--line-2")) || parse("#2a3844"),
      bad: parse(varOf(style, "--bad")) || parse("#e06c6c"),
    };

    if (dark) {
      skin.blend = "lighter";
      skin.fill = 0.13;
      skin.hair = 0.3;
      skin.core = 0.17;
      skin.edge = 0.5;
      skin.bloom = 0.19;
      skin.spot = 2.2;
      skin.spark = 0.75;
      skin.glow = 7;
      skin.halo = 0.7;
      skin.lift = 0.1;
      skin.hot = function (color) {
        return shift(color, 0, -0.14, 0.16);
      };
      skin.rim = function (color) {
        return shift(color, 0, -0.12, 0.16);
      };
      return skin;
    }

    skin.blend = "multiply";
    skin.fill = 0.17;
    skin.hair = 0.28;
    skin.core = 0.26;
    skin.edge = 0.5;
    skin.bloom = 0.075;
    skin.spot = 1.1;
    skin.spark = 0.55;
    skin.glow = 5;
    skin.halo = 0.35;
    skin.lift = -0.05;
    skin.hot = function (color) {
      return shift(color, 0, 0.16, -0.2);
    };
    skin.rim = function (color) {
      return shift(color, 0, 0.2, -0.26);
    };
    return skin;
  }

  // behind is the colour of the box the canvas sits in, which is the ground the
  // multiplying needs. The stylesheet variable is the fallback, but asking the
  // element is truthful whatever the view decides its background should be.
  function behind() {
    var parent = canvas.parentElement;
    if (!parent) {
      return "";
    }
    var found = getComputedStyle(parent).backgroundColor || "";
    return found.indexOf("rgba(0, 0, 0, 0)") === 0 ? "" : found;
  }

  function varOf(style, name) {
    return (style.getPropertyValue(name) || "").trim();
  }

  // taper fades a wave in and out instead of standing it up against a wall. The
  // roll is a fixed slice of the chart rather than of the wave, so every wave
  // fades over the same handful of pixels: otherwise a long span is one slow
  // breath from edge to edge and every wave in the drawing comes out a lens.
  function taper(u, length) {
    if (u <= 0 || u >= 1) {
      return 0;
    }
    var roll = clamp(ROLL / Math.max(length, 1e-6), 0.02, 0.4);
    if (u < roll) {
      return 0.5 - 0.5 * Math.cos((u / roll) * Math.PI);
    }
    if (u > 1 - roll) {
      return 0.5 - 0.5 * Math.cos(((1 - u) / roll) * Math.PI);
    }
    return 1;
  }

  // soften runs a box mean over a row, which is how the concurrency count stops
  // being a staircase and starts being a swell.
  function soften(row, span) {
    var out = new Array(row.length);
    for (var i = 0; i < row.length; i++) {
      var sum = 0;
      var seen = 0;
      for (var o = -span; o <= span; o++) {
        var at = i + o;
        if (at < 0 || at >= row.length) {
          continue;
        }
        sum += row[at];
        seen++;
      }
      out[i] = seen ? sum / seen : 0;
    }
    return out;
  }

  // fbm stacks a few octaves of value noise: one broad shape with progressively
  // finer detail folded into it, at half the weight each time. Four octaves is
  // where a waveform stops looking drawn and starts looking recorded.
  function fbm(x, seed, octaves) {
    var sum = 0;
    var weight = 0;
    var amp = 1;
    var freq = 1;
    for (var i = 0; i < octaves; i++) {
      sum += amp * wobble(x * freq, seed + i * 19.7);
      weight += amp;
      amp *= 0.5;
      freq *= 2.07;
    }
    return weight ? sum / weight : 0;
  }

  // wobble is value noise: one roll per whole number, eased between them, so
  // the result moves smoothly and still never repeats.
  function wobble(x, seed) {
    var low = Math.floor(x);
    var f = x - low;
    var ease = f * f * (3 - 2 * f);
    var a = noise(low * 1.7 + seed);
    var b = noise((low + 1) * 1.7 + seed);
    return a + (b - a) * ease;
  }

  // noise is a stable hash in the range 0 to 1: the same input always gives the
  // same roll, so the drawing does not shuffle between renders.
  function noise(n) {
    var value = Math.sin(n * 12.9898) * 43758.5453;
    return value - Math.floor(value);
  }

  // bite is how much of the body one hairline keeps: mostly all of it, with the
  // occasional deep cut. Raising the roll to a power is what skews it that way.
  function bite(key) {
    return 1 - 0.86 * Math.pow(noise(key), 2.2);
  }

  // harden pulls a roll away from its middle. Averaged octaves of noise crowd
  // around a half, and a wave built straight off them has no dynamics at all.
  function harden(value) {
    return clamp((value - 0.5) * 1.85 + 0.5, 0, 1);
  }

  function smoothstep(low, high, x) {
    var t = clamp((x - low) / Math.max(high - low, 1e-6), 0, 1);
    return t * t * (3 - 2 * t);
  }

  function clamp(value, low, high) {
    return value < low ? low : value > high ? high : value;
  }

  function deepest() {
    var found = 0;
    spans.forEach(function (span) {
      found = Math.max(found, span.depth || 0);
    });
    return found;
  }

  // parse takes the colours the payload carries and the ones the stylesheet
  // gives back, which are not written the same way, and returns neither if it
  // cannot read what it was handed.
  function parse(value) {
    if (!value) {
      return null;
    }
    var text = String(value).trim();
    if (text.charAt(0) === "#") {
      var hex = text.slice(1);
      if (hex.length === 3) {
        hex = hex.charAt(0) + hex.charAt(0) + hex.charAt(1) + hex.charAt(1) + hex.charAt(2) + hex.charAt(2);
      }
      if (hex.length !== 6 || /[^0-9a-fA-F]/.test(hex)) {
        return null;
      }
      return {
        r: parseInt(hex.slice(0, 2), 16),
        g: parseInt(hex.slice(2, 4), 16),
        b: parseInt(hex.slice(4, 6), 16),
      };
    }
    var parts = text.match(/-?\d*\.?\d+/g);
    if (text.indexOf("rgb") === 0 && parts && parts.length >= 3) {
      return { r: +parts[0], g: +parts[1], b: +parts[2] };
    }
    return null;
  }

  function ink(color, alpha) {
    if (!color) {
      return "rgba(0,0,0,0)";
    }
    return (
      "rgba(" +
      Math.round(clamp(color.r, 0, 255)) +
      "," +
      Math.round(clamp(color.g, 0, 255)) +
      "," +
      Math.round(clamp(color.b, 0, 255)) +
      "," +
      clamp(alpha, 0, 1) +
      ")"
    );
  }

  // shift moves a colour in hue, saturation and lightness while keeping it
  // recognisably the kind's colour. Every tint in the drawing is a shift of the
  // one colour the payload gave, which is what stops the spectrum work from
  // quietly inventing a kind that does not exist.
  function shift(color, dh, ds, dl) {
    var c = toHSL(color);
    return fromHSL(c.h + dh, clamp(c.s + ds, 0, 1), clamp(c.l + dl, 0, 1));
  }

  function toHSL(color) {
    var r = color.r / 255;
    var g = color.g / 255;
    var b = color.b / 255;
    var high = Math.max(r, g, b);
    var low = Math.min(r, g, b);
    var l = (high + low) / 2;
    if (high === low) {
      return { h: 0, s: 0, l: l };
    }
    var d = high - low;
    var s = l > 0.5 ? d / (2 - high - low) : d / (high + low);
    var h;
    if (high === r) {
      h = (g - b) / d + (g < b ? 6 : 0);
    } else if (high === g) {
      h = (b - r) / d + 2;
    } else {
      h = (r - g) / d + 4;
    }
    return { h: h / 6, s: s, l: l };
  }

  function fromHSL(h, s, l) {
    h = h - Math.floor(h);
    if (s <= 0) {
      return { r: l * 255, g: l * 255, b: l * 255 };
    }
    var q = l < 0.5 ? l * (1 + s) : l + s - l * s;
    var p = 2 * l - q;
    return {
      r: band(p, q, h + 1 / 3) * 255,
      g: band(p, q, h) * 255,
      b: band(p, q, h - 1 / 3) * 255,
    };
  }

  function band(p, q, t) {
    if (t < 0) {
      t += 1;
    }
    if (t > 1) {
      t -= 1;
    }
    if (t < 1 / 6) {
      return p + (q - p) * 6 * t;
    }
    if (t < 1 / 2) {
      return q;
    }
    if (t < 2 / 3) {
      return p + (q - p) * (2 / 3 - t) * 6;
    }
    return p;
  }

  var media = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  if (media) {
    if (media.addEventListener) {
      media.addEventListener("change", draw);
    } else if (media.addListener) {
      media.addListener(draw);
    }
  }

  draw();
  window.addEventListener("resize", draw);
})();
