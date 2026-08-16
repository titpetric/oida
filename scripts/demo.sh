#!/usr/bin/env bash
# Take the screenshots the documentation shows.
#
#   scripts/demo.sh            write docs/assets/*.png from the dark theme
#   scripts/demo.sh light      the same shots in the light theme
#
# Seeds the running service with traffic from a few hosts, picks the /report
# trace, which is the one with a fan out worth looking at, and writes one PNG
# per component. The service has to be up:
#
#   atkins
#   docker compose up -d --force-recreate --wait
#   scripts/demo.sh
#
# Target another instance with OIDA_BASE and OIDA_PATH.
set -euo pipefail

BASE="${OIDA_BASE:-http://localhost:8097}"
UI="${OIDA_PATH:-/debug/oida}"

theme="${1:-dark}"
root="$(cd "$(dirname "$0")/.." && pwd)"

case "$theme" in
  dark|light) ;;
  *) echo "theme must be dark or light" >&2; exit 2 ;;
esac

if ! curl -fsS -o /dev/null "${BASE}${UI}?format=json"; then
  echo "no service at ${BASE}${UI}: docker compose up -d --force-recreate --wait" >&2
  exit 1
fi

# Traffic from a few domains, so the host overview has something to overview.
# The names are only Host headers: the demo router serves whatever it is asked.
for host in shop.example api.example admin.example; do
  for i in 1 2 3; do
    curl -s -o /dev/null -H "Host: ${host}" "${BASE}/users/${i}"
    curl -s -o /dev/null -H "Host: ${host}" "${BASE}/report"
  done
  curl -s -o /dev/null -H "Host: ${host}" "${BASE}/"
done

# And the traffic the detail shots come from.
for i in $(seq 1 6); do
  curl -s -o /dev/null "${BASE}/users/${i}"
  curl -s -o /dev/null "${BASE}/report"
done

# User 0 does not exist, but the demo cache answers one lookup in three from
# memory and never reaches the failure: ask often enough to record one, so the
# list has a red row and the statistics have an error count.
for _ in 1 2 3 4; do
  curl -s -o /dev/null "${BASE}/users/0"
done

curl -s -o /dev/null "${BASE}/users/7"
curl -s -o /dev/null "${BASE}/report"

# The trace to show: a report with the whole fan out recorded, three queries at
# once inside a handler inside the request.
trace=$(curl -s "${BASE}${UI}/traces?format=json" |
  jq -r 'map(select(.name == "GET /report" and (.spans | length) >= 6)) | .[0].id // empty')

if [ -z "$trace" ]; then
  echo "no /report trace with a full fan out was recorded" >&2
  exit 1
fi

detail="${UI}/trace/${trace}"

cd "$root"

shots=$(cat <<MANIFEST | node scripts/shot.js
{
  "base": "${BASE}",
  "width": 1440,
  "scale": 2,
  "theme": "${theme}",
  "pad": 14,
  "shots": [
    {
      "out": "docs/assets/header.png",
      "path": "${UI}",
      "pick": "[q('header.top'), q('.metrics'), q('nav.tabs')]"
    },
    {
      "out": "docs/assets/hosts.png",
      "path": "${UI}",
      "pick": "q('.scroll')",
      "pad": 2
    },
    {
      "out": "docs/assets/traces.png",
      "path": "${UI}/traces",
      "pick": "[q('.filters'), q('thead')].concat(qa('tbody tr').slice(0, 12))",
      "pad": 2
    },
    {
      "out": "docs/assets/stats.png",
      "path": "${UI}/stats",
      "pick": "[q('p.note'), q('thead')].concat(qa('tbody tr').slice(0, 10))",
      "pad": 2
    },
    {
      "out": "docs/assets/detail-waves.png",
      "path": "${detail}",
      "pick": "[q('.trace-head'), q('.live-head'), q('.waves'), q('.axis')]"
    },
    {
      "out": "docs/assets/detail-spans.png",
      "path": "${detail}",
      "pick": "(function(){ var f = q('.spans'); f.style.transition = 'none'; q('input[data-peek]').checked = true; f.getBoundingClientRect(); return [q('.peek'), f]; })()"
    },
    {
      "out": "docs/assets/detail-footer.png",
      "path": "${detail}",
      "pick": "[q('.facts'), q('footer')]"
    }
  ]
}
MANIFEST
)

# The console is flat colour on flat colour, so a palette holds all of it and
# the files come out a third of the size. Without imagemagick the raw captures
# are perfectly good, only heavier.
for shot in $shots; do
  if command -v magick >/dev/null 2>&1; then
    magick "$shot" -strip -colors 256 -define png:compression-level=9 "$shot"
  fi
  echo "$shot"
done
