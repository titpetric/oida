#!/usr/bin/env bash
# Repeatable project checks. Anything run more than once lives here.
#
#   scripts/tool.sh ship            build, test, image, redeploy, seed
#   scripts/tool.sh deploy          docker compose up with the current image
#   scripts/tool.sh traffic [n]     seed n rounds across three virtual hosts
#   scripts/tool.sh assets          verify every embedded asset is served
#   scripts/tool.sh views           verify every view answers 200 in html/json/text
#   scripts/tool.sh stream [secs]   count events pushed on the live stream
#   scripts/tool.sh smoke           assets + views + stream, the whole surface
#   scripts/tool.sh shots [theme]   screenshot every view at three viewports
#   scripts/tool.sh ps              show the running service
#
# Never runs the binary outside docker compose.
set -euo pipefail

cd "$(dirname "$0")/.."

BASE="${OIDA_BASE:-http://localhost:8097}"
UI="${OIDA_PATH:-/debug/oida}"
HOSTS=(shop.local admin.local)

# code returns the status code of a GET against the running service.
code() {
  curl -s -o /dev/null -w "%{http_code}" "${BASE}${1}"
}

# expect checks one route, printing a line per check.
expect() {
  local route="$1" want="$2" got
  got=$(code "$route")
  if [ "$got" = "$want" ]; then
    printf '  ok    %-44s %s\n' "$route" "$got"
  else
    printf '  FAIL  %-44s %s, want %s\n' "$route" "$got" "$want"
    return 1
  fi
}

command="${1:-smoke}"
shift || true

case "$command" in
  ship)
    atkins
    docker compose up -d --force-recreate --wait
    "$0" traffic 6
    "$0" smoke
    ;;

  deploy)
    docker compose up -d --force-recreate --wait
    ;;

  ps)
    docker compose ps
    ;;

  traffic)
    rounds="${1:-6}"
    for i in $(seq 1 "$rounds"); do
      curl -s -o /dev/null "${BASE}/users/${i}"
      curl -s -o /dev/null "${BASE}/report"
      curl -s -o /dev/null -H "Host: ${HOSTS[0]}" "${BASE}/users/${i}"
      curl -s -o /dev/null -H "Host: ${HOSTS[1]}" "${BASE}/report"
    done
    curl -s -o /dev/null "${BASE}/users/0" # one failure, for the SLA tile
    echo "seeded ${rounds} rounds across $(( ${#HOSTS[@]} + 1 )) hosts"
    ;;

  assets)
    echo "assets:"
    local_status=0
    for name in oida.css oida.js input-select.svg; do
      expect "${UI}/assets/${name}" 200 || local_status=1
    done
    # The stylesheet must carry resolved asset URLs, never the raw marker.
    if curl -s "${BASE}${UI}/assets/oida.css" | grep -q 'asset:'; then
      echo "  FAIL  stylesheet still contains the asset: marker"
      local_status=1
    else
      echo "  ok    stylesheet asset urls resolved"
    fi
    if curl -s "${BASE}${UI}" | grep -q 'url("asset:'; then
      echo "  FAIL  inlined stylesheet still contains the asset: marker"
      local_status=1
    else
      echo "  ok    inlined stylesheet asset urls resolved"
    fi
    exit $local_status
    ;;

  views)
    echo "views:"
    status=0
    for route in "" / /traces /live /stats \
      "/traces?q=select" "/traces?sort=duration" "/traces?host=${HOSTS[0]}" \
      "/live?stream=off" "?format=json" "/traces?format=json" "/traces?format=text" \
      "/stats?format=json"; do
      expect "${UI}${route}" 200 || status=1
    done
    id=$(curl -s "${BASE}${UI}/traces?format=json" | jq -r '.[0].id // empty')
    if [ -n "$id" ]; then
      expect "${UI}/trace/${id}" 200 || status=1
    fi
    expect "${UI}/trace/NOPE" 404 || status=1
    expect "${UI}/nope" 404 || status=1
    exit $status
    ;;

  stream)
    secs="${1:-5}"
    out=$(mktemp)
    timeout "$secs" curl -sN "${BASE}${UI}/live/events" > "$out" &
    reader=$!
    sleep 1
    curl -s -o /dev/null "${BASE}/report"
    wait $reader || true
    events=$(grep -c '^data:' "$out" || true)
    rm -f "$out"
    echo "stream: ${events} events in ${secs}s"
    [ "$events" -ge 2 ]
    ;;

  smoke)
    "$0" assets
    "$0" views
    "$0" stream 5
    echo "smoke: ok"
    ;;

  shots)
    theme="${1:-dark}"
    for view in "" /traces /live /stats; do
      name="$(echo "${view:-list}" | tr -d /)"
      scripts/chromium.sh "${UI}${view}" "${name}-wide" "$theme" 1600x900
      scripts/chromium.sh "${UI}${view}" "${name}-medium" "$theme" 1024x800
      scripts/chromium.sh "${UI}${view}" "${name}-narrow" "$theme" 420x900
    done
    ;;

  *)
    echo "unknown command: $command" >&2
    sed -n '2,16p' "$0" >&2
    exit 2
    ;;
esac
