#!/usr/bin/env bash
# Screenshot a page of the running oida front end.
#
#   scripts/chromium.sh <path> [name] [dark|light] [WxH]
#
#   scripts/chromium.sh /debug/oida
#   scripts/chromium.sh /debug/oida/live live dark 1440x900
#   scripts/chromium.sh /debug/oida/trace/$(scripts/inspect.sh id) detail
#
# Writes /tmp/oida-<name>.png and prints the path.
set -euo pipefail

BASE="${OIDA_BASE:-http://localhost:8097}"

path="${1:-/debug/oida}"
name="${2:-shot}"
theme="${3:-dark}"
size="${4:-1440x1000}"

case "$theme" in
  dark) scheme=0 ;;
  light) scheme=1 ;;
  *) echo "theme must be dark or light" >&2; exit 2 ;;
esac

out="/tmp/oida-${name}.png"
rm -f "$out"

# The live view holds an event stream open, so the load event never fires and
# chromium waits forever. ?stream=off renders the same markup as a static page.
case "$path" in
  */live) path="${path}?stream=off" ;;
  */live\?*) path="${path}&stream=off" ;;
esac

timeout 45 chromium \
  --headless \
  --no-sandbox \
  --disable-gpu \
  --hide-scrollbars \
  --blink-settings=preferredColorScheme=${scheme} \
  --virtual-time-budget=3000 \
  --window-size="${size/x/,}" \
  --screenshot="$out" \
  "${BASE}${path}" 2>/dev/null || true

if [ ! -s "$out" ]; then
  echo "no screenshot written for ${BASE}${path}" >&2
  exit 1
fi

echo "$out"
