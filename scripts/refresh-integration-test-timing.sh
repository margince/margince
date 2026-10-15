#!/usr/bin/env bash
# Rewrite scripts/integration-test-timing.txt from the integration shards of
# one CI run. Each shard uploads the durations of the tests it ran as
# test-timing.txt in its integration-shard-<k> artifact.
#
# Only tests of SLOW_TEST seconds or more are listed one by one; they hold half
# the lane's time in a tenth of its tests. The rest of each package is one
# `dir|package|*|seconds` line, their mean.
#
# Usage: scripts/refresh-integration-test-timing.sh <run-id | test-timing-file>
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <run-id | test-timing-file>  (a green ci.yml run whose integration lane ran)" >&2
  exit 2
fi
root="$(cd "$(dirname "$0")/.." && pwd)"
target="$root/scripts/integration-test-timing.txt"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

if [[ -f "$1" ]]; then
  cp "$1" "$work/all.txt"
else
  gh run download "$1" --pattern 'integration-shard-*' --dir "$work/shards"
  find "$work/shards" -name test-timing.txt -exec cat {} + > "$work/all.txt"
fi
if [[ ! -s "$work/all.txt" ]]; then
  echo "FAIL: no test durations in $1 — pick a run whose shards uploaded test-timing.txt" >&2
  exit 1
fi

{
  grep '^#' "$target"
  # A malformed line stops the refresh, so the old weights stay: awk reads a
  # non-numeric duration as 0, which would weigh that test as free.
  LC_ALL=C sort -u "$work/all.txt" | awk -F'|' -v slow="${SLOW_TEST:-1}" '
    NF != 4 || $4 !~ /^[0-9]+(\.[0-9]+)?$/ {
      printf "FAIL: not `dir|package|Test|seconds`: %s\n", $0 > "/dev/stderr"; bad = 1; exit 1
    }
    { valid++ }
    $4 + 0 >= slow { print; next }
    { key = $1 "|" $2; rest[key] += $4; n[key]++ }
    END {
      if (bad) exit 1
      if (!valid) { print "FAIL: no test durations to write" > "/dev/stderr"; exit 1 }
      for (key in n) printf "%s|*|%.3f\n", key, rest[key] / n[key]
    }
  ' | LC_ALL=C sort
} > "$work/timing.txt"
mv "$work/timing.txt" "$target"
echo "OK: $(grep -vc '^#' "$target") lines written to scripts/integration-test-timing.txt"
