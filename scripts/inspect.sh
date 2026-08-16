#!/usr/bin/env bash
# Inspect the running oida service over its JSON and plain text APIs.
#
#   scripts/inspect.sh seed [n]        send n rounds of demo traffic (default 6)
#   scripts/inspect.sh json [path]     GET path as JSON, piped through jq
#   scripts/inspect.sh jq <filter>     GET the trace list and apply a jq filter
#   scripts/inspect.sh text [path]     GET path as plain text
#   scripts/inspect.sh html [path]     GET path as raw HTML
#   scripts/inspect.sh id [index]      print the id of the nth recorded trace
#   scripts/inspect.sh trace [index]   print the nth recorded trace as JSON
#   scripts/inspect.sh stream [secs]   read the live event stream for secs
#
# Override the target with OIDA_BASE and OIDA_PATH.
set -euo pipefail

BASE="${OIDA_BASE:-http://localhost:8097}"
UI="${OIDA_PATH:-/debug/oida}"

command="${1:-json}"
shift || true

case "$command" in
  seed)
    rounds="${1:-6}"
    for i in $(seq 1 "$rounds"); do
      curl -s -o /dev/null "${BASE}/users/${i}"
      curl -s -o /dev/null "${BASE}/report"
    done
    curl -s -o /dev/null "${BASE}/users/0" # records a failure
    curl -s -o /dev/null "${BASE}/"
    echo "seeded ${rounds} rounds"
    ;;

  json)
    curl -s "${BASE}${1:-$UI}?format=json" | jq .
    ;;

  jq)
    filter="${1:?usage: inspect.sh jq <filter>}"
    curl -s "${BASE}${UI}/traces?format=json" | jq "$filter"
    ;;

  text)
    curl -s "${BASE}${1:-$UI}?format=text"
    ;;

  html)
    curl -s "${BASE}${1:-$UI}?format=html"
    ;;

  id)
    curl -s "${BASE}${UI}/traces?format=json" | jq -r ".[${1:-0}].id"
    ;;

  trace)
    id=$(curl -s "${BASE}${UI}/traces?format=json" | jq -r ".[${1:-0}].id")
    curl -s "${BASE}${UI}/trace/${id}?format=json" | jq .
    ;;

  stream)
    timeout "${1:-5}" curl -sN "${BASE}${UI}/live/events" || true
    ;;

  *)
    echo "unknown command: $command" >&2
    sed -n '2,18p' "$0" >&2
    exit 2
    ;;
esac
