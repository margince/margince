#!/usr/bin/env bash
# pace-workflow.sh <workflow file> <interval seconds> — start a workflow on main
# once its newest run is at least that old.
#
# GitHub's scheduler can run a cron hours late or not at all, and it never
# reports a run it skips. A push to main is delivered, so each push asks this one
# question and the cadence holds while main is moving. pace.yml names each
# workflow it paces and that workflow's interval.
#
# It reads the newest run of ANY event, because a run started by hand or by the
# cron answers the same question as one started from here.
set -euo pipefail

: "${GH_TOKEN:?the pacer needs a token to read and start workflow runs}"
: "${REPO:?REPO must name the repository}"
(($# == 2)) && [[ "$2" =~ ^[1-9][0-9]*$ ]] || {
  echo "usage: pace-workflow.sh <workflow file> <interval seconds>" >&2
  exit 2
}
readonly WORKFLOW="$1" INTERVAL_SECONDS="$2"

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
  --workflow "$WORKFLOW" \
  --branch main \
  --limit 1 \
  --json createdAt \
  --jq '.[0].createdAt // empty')"

if [[ -n "$last_created" ]]; then
  age_seconds=$((now_epoch - $(to_epoch "$last_created")))
  if ((age_seconds < INTERVAL_SECONDS)); then
    echo "pace-workflow: $WORKFLOW last ran ${age_seconds}s ago, under ${INTERVAL_SECONDS}s; nothing to start"
    exit 0
  fi
fi

gh workflow run "$WORKFLOW" --repo "$REPO" --ref main
echo "pace-workflow: started $WORKFLOW; last run ${last_created:-is not on record}"
