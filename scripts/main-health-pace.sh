#!/usr/bin/env bash
# main-health-pace.sh — start the health check once the last one is old enough.
#
# GitHub's scheduler creates about a third of the runs main-health.yml's cron
# asks for, and it never reports the ones it skips. A push to main is delivered,
# so each push asks this one question and the cadence holds while main is moving.
#
# It reads the newest run of ANY event, because a run started by hand or by the
# cron answers the same question as one started from here.
set -euo pipefail

: "${GH_TOKEN:?the pacer needs a token to read and start main-health runs}"
: "${REPO:?REPO must name the repository}"

# The one number the workflow's header and docs/reference/ci-workflows.md describe.
readonly INTERVAL_SECONDS=7200

# Injected so a case can pin the clock.
now_epoch="${NOW_EPOCH:-$(date -u +%s)}"

# BSD first, GNU second, the same order check-renovate-liveness.sh uses.
to_epoch() {
  date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$1" +%s 2>/dev/null || date -u -d "$1" +%s
}

# A failed read stops here under `set -e`. Reading it as "no run on record" would
# start a full lane on every push for as long as the read kept failing.
last_created="$(gh run list \
  --repo "$REPO" \
  --workflow main-health.yml \
  --branch main \
  --limit 1 \
  --json createdAt \
  --jq '.[0].createdAt // empty')"

if [[ -n "$last_created" ]]; then
  age_seconds=$((now_epoch - $(to_epoch "$last_created")))
  if ((age_seconds < INTERVAL_SECONDS)); then
    echo "main-health-pace: last run is ${age_seconds}s old, under ${INTERVAL_SECONDS}s; nothing to start"
    exit 0
  fi
fi

gh workflow run main-health.yml --repo "$REPO" --ref main
echo "main-health-pace: started main-health; last run ${last_created:-is not on record}"
