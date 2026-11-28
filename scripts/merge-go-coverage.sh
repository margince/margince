#!/usr/bin/env bash
# Merge every binary Go coverage pod of a run into the one text profile
# sonar-project.properties names (backend/coverage.out).
#
# The pods come from two places. The integration shards leave them in their
# artifacts (integration-shard-<k>/covdata). The unit legs that run test-cached
# and test-solo with UNIT_COVERDIR upload theirs as unit-covdata-<leg>. Those
# legs are jobs of the calling workflow, and a job of the integration lane cannot
# wait for them, so the merge runs in the job that needs both: the Sonar scan.
#
# It fails when a producer sent nothing. The scanner scores a package no report
# covers at 0%, so a lost upload would otherwise publish a lower number as a
# measurement.
#
# Usage: merge-go-coverage.sh <artifacts-root> <cover-out>
#   artifacts-root: the integration-shard-* and unit-covdata-* artifacts of the
#   run, each as <name>-attempt-<n> (actions/download-artifact)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/lib-integration-artifacts.sh"

ARTIFACTS="${1:?usage: merge-go-coverage.sh <artifacts-root> <cover-out>}"
COVER_OUT="${2:?usage: merge-go-coverage.sh <artifacts-root> <cover-out>}"
case "$ARTIFACTS" in /*) ;; *) ARTIFACTS="$PWD/$ARTIFACTS";; esac
case "$COVER_OUT" in /*) ;; *) COVER_OUT="$PWD/$COVER_OUT";; esac

# The matrix entries with `unit-cover: "on"` in ci.yml's deterministic-gates and
# main-health.yml's gates: the legs that run test-cached and test-solo.
UNIT_COVER_LEGS=(unit solo)

fail() { echo "FAIL: $*"; exit 1; }

newest_attempt_artifacts "$ARTIFACTS" || fail "the artifacts under $ARTIFACTS could not be cut to the newest attempt of each"

# Every shard must be here; a shard that claimed no group has no pods, so the
# lane as a whole is held to having some.
first_meta="$(find "$ARTIFACTS" -mindepth 2 -maxdepth 2 -path '*/integration-shard-*/meta.txt' | LC_ALL=C sort | sed -n 1p)"
[[ -n "$first_meta" ]] || fail "no integration-shard-*/meta.txt under $ARTIFACTS — no shard artifact was downloaded"
total="$(sed -n 's/^total=//p' "$first_meta")"
[[ "$total" =~ ^[0-9]+$ && "$total" -ge 1 ]] || fail "unparsable total= in $first_meta"
for k in $(seq 1 "$total"); do
  [[ -f "$ARTIFACTS/integration-shard-$k/meta.txt" ]] || fail "shard $k/$total sent no artifact, so its coverage is missing"
done

pods="$(find "$ARTIFACTS" -name 'covmeta.*' -exec dirname {} \; | LC_ALL=C sort -u)"
shard_pods="$(grep -c "^$ARTIFACTS/integration-shard-" <<<"$pods" || true)"
(( shard_pods > 0 )) || fail "no shard sent a coverage pod — the integration lane's coverage is missing"
for leg in "${UNIT_COVER_LEGS[@]}"; do
  # A here-string, not a pipe: grep -q stops reading at its first match, and
  # under pipefail the writer it cut off would fail this check.
  grep -qxF "$ARTIFACTS/unit-covdata-$leg" <<<"$pods" \
    || fail "no coverage pods from the unit leg '$leg' (unit-covdata-$leg) — its packages would read 0%"
done

dirs="$(printf '%s\n' "$pods" | paste -sd, -)"
( cd "$ROOT_DIR/backend" && go tool covdata textfmt -i="$dirs" -o="$COVER_OUT" )
echo "coverage: merged $(grep -c . <<<"$pods") pod dirs ($shard_pods from $total shards, unit legs: ${UNIT_COVER_LEGS[*]}) → $COVER_OUT"
# The share Sonar's overall figure starts from, so a run's log can be held
# against main's. A block repeated across lines counts once, covered if any is.
awk 'NR > 1 { key = $1; stmts[key] = $2; if ($3 > 0) hit[key] = 1 }
  END { for (k in stmts) { total += stmts[k]; if (k in hit) covered += stmts[k] }
        printf "coverage: %d of %d statements covered (%.1f%%)\n", covered, total, total ? 100 * covered / total : 0 }' "$COVER_OUT"
