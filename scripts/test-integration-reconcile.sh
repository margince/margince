#!/usr/bin/env bash
# Fan-in for the sharded CI integration lane.
#
# Each `integration shard (k/N)` matrix job runs a slice of the integration
# tests (scripts/test-integration-parallel.sh, INTEGRATION_SHARD): a fixed one,
# or the groups it claimed from the queue the shards share. It uploads its
# manifests + binary coverage pods as an artifact. A green shard only proves
# ITS slice ran — this script proves the lane as a whole: every shard reported,
# every shard sliced the same discovery, and the slices add up to exactly that
# discovery (complete + disjoint). Without this check a slicing bug — a lost
# shard artifact, a group two shards claimed, divergent discoveries — would
# quietly run fewer or doubled tests while every job stays green, which for
# this lane means RLS/erasure gates silently not running. A shard that reported
# nothing is named with the tests no other shard ran. The coverage pods are
# merged later, with the unit legs' (scripts/merge-go-coverage.sh).
#
# Layout under <artifacts-root> (the actions/download-artifact target dir), once
# the newest attempt of each artifact has its plain name:
#   integration-shard-<k>/     discovery.txt assigned.txt ran.txt meta.txt
#                              repeated-gates.txt covdata/<part>/ (binary pods)
#                              claimed.txt (queue mode, also when red)
#
# Usage: test-integration-reconcile.sh <artifacts-root>
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
ROOT="$ROOT_DIR"
source "$ROOT/scripts/lib-testdb.sh"
source "$ROOT/scripts/lib-integration-artifacts.sh"

ARTIFACTS="${1:?usage: test-integration-reconcile.sh <artifacts-root>}"
case "$ARTIFACTS" in /*) ;; *) ARTIFACTS="$ROOT_DIR/$ARTIFACTS";; esac

fail() { echo "FAIL: $*"; exit 1; }

newest_attempt_artifacts "$ARTIFACTS" || fail "the artifacts under $ARTIFACTS could not be cut to the newest attempt of each"

# The expected shard count comes from the shards' own meta (INTEGRATION_SHARD's
# N, i.e. the CI matrix size) — no second copy of the number to drift. Any
# present shard works as the source; 1..N must then all be present and agree.
first_meta="$(find "$ARTIFACTS" -mindepth 2 -maxdepth 2 -path '*/integration-shard-*/meta.txt' | LC_ALL=C sort | sed -n 1p)"
[[ -n "$first_meta" ]] || fail "no integration-shard-*/meta.txt under $ARTIFACTS — no shard artifact was downloaded"
TOTAL="$(sed -n 's/^total=//p' "$first_meta")"
[[ "$TOTAL" =~ ^[0-9]+$ && "$TOTAL" -ge 1 ]] || fail "unparsable total= in $first_meta"

REF_SHARD="$(sed -n 's/^shard=//p' "$first_meta")"
REF_DISCOVERY="$ARTIFACTS/integration-shard-$REF_SHARD/discovery.txt"
REF_TIMING="$(sed -n 's/^timing=//p' "$first_meta")"
[[ -n "$REF_TIMING" ]] || fail "shard $REF_SHARD names no timing= digest in its meta.txt — the weights it split by are unknown"
UNION="$(mktemp)"
trap 'rm -f "$UNION"' EXIT

# A shard without meta.txt never finished, and one with failure-summary.txt is
# red. The tests no green shard ran are named below, instead of stopping at the
# first such shard.
missing=()
for k in $(seq 1 "$TOTAL"); do
  d="$ARTIFACTS/integration-shard-$k"
  if [[ ! -f "$d/meta.txt" || -f "$d/failure-summary.txt" ]]; then
    missing+=("$k")
    continue
  fi
  for f in discovery.txt assigned.txt ran.txt repeated-gates.txt; do
    [[ -f "$d/$f" ]] || fail "shard $k/$TOTAL is missing $f — its job reported green without a complete artifact"
  done
  grep -qx "shard=$k" "$d/meta.txt" || fail "shard $k artifact carries $(grep '^shard=' "$d/meta.txt") — artifact/matrix mix-up"
  grep -qx "total=$TOTAL" "$d/meta.txt" || fail "shard $k ran with $(grep '^total=' "$d/meta.txt"), expected total=$TOTAL — the matrix and INTEGRATION_SHARD disagree"
  cmp -s "$REF_DISCOVERY" "$d/discovery.txt" || fail "shard $k discovered a different test set than shard $REF_SHARD — shards did not run the same code"
  # Another file of weights deals another split, so the slices would overlap and
  # leave gaps with no slicing bug to find.
  grep -qx "timing=$REF_TIMING" "$d/meta.txt" || fail "shard $k split by other test durations than shard $REF_SHARD ($(grep '^timing=' "$d/meta.txt" || echo 'no timing=')) — every shard of a run must read the one file the build job hands on"
  diff "$d/assigned.txt" "$d/ran.txt" > /dev/null || fail "shard $k ran a different test set than assigned (its own gate should have caught this)"
  reconcile_parts "$d" || fail "shard $k's parts do not add up to its slice"
  cat "$d/assigned.txt" >> "$UNION"
done
LC_ALL=C sort -o "$UNION" "$UNION"

if (( ${#missing[@]} > 0 )); then
  for k in "${missing[@]}"; do
    claimed="$ARTIFACTS/integration-shard-$k/claimed.txt"
    state="reported no result"
    [[ ! -f "$ARTIFACTS/integration-shard-$k/failure-summary.txt" ]] || state="is red (its failure-summary.txt and rerun.txt say what failed)"
    if [[ -s "$claimed" ]]; then
      echo "FAIL: shard $k/$TOTAL $state; it had claimed groups $(cut -d' ' -f1 "$claimed" | paste -sd' ' -)"
    else
      echo "FAIL: shard $k/$TOTAL $state"
    fi
  done
  echo "FAIL: these tests passed on no green shard, by package (count, first names):"
  LC_ALL=C comm -23 "$REF_DISCOVERY" "$UNION" | awk -F'|' '
    { key = $1 "|" $2; n[key]++; if (n[key] <= 3) names[key] = names[key] " " $3 }
    END { for (key in n) printf "  %-48s %5d %s%s\n", key, n[key], names[key], (n[key] > 3 ? " ..." : "") }
  ' | LC_ALL=C sort
  exit 1
fi

# Complete + disjoint in one comparison: the concatenated slices, sorted, must
# be exactly the discovery (a gap shrinks the union, an overlap grows it).
if ! diff "$REF_DISCOVERY" "$UNION" > /dev/null; then
  echo "FAIL: the $TOTAL shard slices do not add up to the discovered test set:"
  diff "$REF_DISCOVERY" "$UNION" | sed -n 's/^< /  discovered but in no slice: /p; s/^> /  extra slice entry (duplicate or not discovered): /p' || true
  exit 1
fi
NTESTS="$(wc -l < "$REF_DISCOVERY" | tr -d ' ')"

# Each shard saw a slice of the statements, so an allowlisted sender is stale
# only when no shard saw it repeat the content gate.
seen_files=()
for k in $(seq 1 "$TOTAL"); do seen_files+=("$ARTIFACTS/integration-shard-$k/repeated-gates.txt"); done
stale_repeated_gates "${seen_files[@]}" || fail "the repeated-gate allowlist names senders that no longer repeat — remove them"

echo "OK: integration reconciled — $TOTAL shards cover all $NTESTS tests, complete and disjoint"
