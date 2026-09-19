#!/usr/bin/env bash
# Capture full-page mobile views for visual verification.
#
#   scripts/mobile.sh
#
# Writes docs/assets/mobile-*.png at a 390 CSS pixel viewport. The service has
# to be up with the current image:
#
#   atkins --final default
#   docker compose up -d --force-recreate --wait
#   scripts/mobile.sh
#
# Capture the protected entry state from an authenticated one-off service:
#
#   docker compose down
#   docker compose run -d --rm --service-ports -e OIDA_AUTH=review:mobile oida
#   scripts/mobile.sh
#   docker compose down --remove-orphans
#
# Target another instance with OIDA_BASE and OIDA_PATH.
set -euo pipefail

BASE="${OIDA_BASE:-http://localhost:8097}"
UI="${OIDA_PATH:-/debug/oida}"
root="$(cd "$(dirname "$0")/.." && pwd)"

# An authenticated instance cannot expose the dashboard fixtures, but it can
# provide the protected entry state. Capture that state in both palettes and
# stop before trying to seed traffic behind it.
if curl -fsS -o /dev/null "${BASE}${UI}/login"; then
  cd "$root"
  shots=$(cat <<MANIFEST | node scripts/shot.js
{
  "base": "${BASE}",
  "width": 390,
  "height": 844,
  "scale": 2,
  "pad": 0,
  "shots": [
    {
      "out": "docs/assets/mobile-login-dark.png",
      "path": "${UI}/login",
      "theme": "dark",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-login-light.png",
      "path": "${UI}/login",
      "theme": "light",
      "pick": "q('body')"
    }
  ]
}
MANIFEST
)
  for shot in $shots; do
    if command -v magick >/dev/null 2>&1; then
      magick "$shot" -strip -colors 128 -define png:compression-level=9 "$shot"
    fi
    echo "$shot"
  done
  exit 0
fi

if ! curl -fsS -o /dev/null "${BASE}${UI}?format=json"; then
  echo "no service at ${BASE}${UI}: docker compose up -d --force-recreate --wait" >&2
  exit 1
fi

# A populated feed makes truncation and wrapping visible. Reusing the smoke
# traffic keeps this capture on the same demo paths as the other checks.
"${root}/scripts/tool.sh" traffic 6

# The selected detail host is deliberately longer than the masthead affords.
# It proves the switcher ellipsizes rather than wrapping the one-line header.
curl -s -o /dev/null -H "Host: telemetry-for-a-very-long-service-name.example.internal" "${BASE}/report"

trace=$(curl -s "${BASE}${UI}/traces?format=json" |
  jq -r 'map(select(.name == "GET /report" and (.spans | length) >= 6)) | .[0].id // empty')

if [ -z "$trace" ]; then
  echo "no /report trace with a full fan out was recorded" >&2
  exit 1
fi

cd "$root"

shots=$(cat <<MANIFEST | node scripts/shot.js
{
  "base": "${BASE}",
  "width": 390,
  "height": 844,
  "scale": 2,
  "pad": 0,
  "shots": [
    {
      "out": "docs/assets/mobile-hosts-dark.png",
      "path": "${UI}",
      "theme": "dark",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-hosts-light.png",
      "path": "${UI}",
      "theme": "light",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-traces-dark.png",
      "path": "${UI}/traces",
      "theme": "dark",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-traces-light.png",
      "path": "${UI}/traces",
      "theme": "light",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-live-dark.png",
      "path": "${UI}/live?stream=off",
      "theme": "dark",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-live-light.png",
      "path": "${UI}/live?stream=off",
      "theme": "light",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-stats-dark.png",
      "path": "${UI}/stats",
      "theme": "dark",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-stats-light.png",
      "path": "${UI}/stats",
      "theme": "light",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-closed-dark.png",
      "path": "${UI}/trace/${trace}",
      "theme": "dark",
      "prepare": "qa('.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = input.dataset.peek === 'spans'); q('#oida-wave-summary').checked = false",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-closed-light.png",
      "path": "${UI}/trace/${trace}",
      "theme": "light",
      "prepare": "qa('.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = input.dataset.peek === 'spans'); q('#oida-wave-summary').checked = false",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-dark.png",
      "path": "${UI}/trace/${trace}",
      "theme": "dark",
      "prepare": "qa('.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = input.dataset.peek !== 'metrics'); q('#oida-wave-summary').checked = true",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-light.png",
      "path": "${UI}/trace/${trace}",
      "theme": "light",
      "prepare": "qa('.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = input.dataset.peek !== 'metrics'); q('#oida-wave-summary').checked = true",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-logs-dark.png",
      "path": "${UI}/trace/${trace}",
      "theme": "dark",
      "prepare": "qa('.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = input.dataset.peek !== 'metrics' && input.dataset.peek !== 'spans'); q('#oida-wave-summary').checked = true",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-logs-light.png",
      "path": "${UI}/trace/${trace}",
      "theme": "light",
      "prepare": "qa('.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = input.dataset.peek !== 'metrics' && input.dataset.peek !== 'spans'); q('#oida-wave-summary').checked = true",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-metrics-dark.png",
      "path": "${UI}/trace/${trace}",
      "theme": "dark",
      "prepare": "qa('.metrics-fold,.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = true); q('#oida-wave-summary').checked = true",
      "pick": "q('body')"
    },
    {
      "out": "docs/assets/mobile-detail-metrics-light.png",
      "path": "${UI}/trace/${trace}",
      "theme": "light",
      "prepare": "qa('.metrics-fold,.wave-summary,.spans,.logs-panel,.detail-fold').forEach((fold) => fold.style.transition = 'none'); qa('input[data-peek]').forEach((input) => input.checked = true); q('#oida-wave-summary').checked = true",
      "pick": "q('body')"
    }
  ]
}
MANIFEST
)

for shot in $shots; do
  if command -v magick >/dev/null 2>&1; then
    magick "$shot" -strip -colors 128 -define png:compression-level=9 "$shot"
  fi
  echo "$shot"
done
