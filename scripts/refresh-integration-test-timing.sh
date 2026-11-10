#!/usr/bin/env bash
# Rewrite the integration lane's test durations from green CI runs. The fan-in
# of each run uploads every shard's `dir|package|Test|seconds` lines as
# test-timing.txt in its test-timing-integration artifact.
#
# A test's weight is its mean over the inputs. One run measures each test on
# one runner, and the same test has run 0.7 to 1.2 times its usual time from
# one runner to the next, so several runs weigh it better than one.
#
# Only tests of SLOW_TEST seconds or more are listed one by one; they hold half
# the lane's time in a tenth of its tests. The rest of each package is one
# `dir|package|*|seconds` line, their mean.
#
# Usage: scripts/refresh-integration-test-timing.sh [-o OUT] <run-id | test-timing-file>...
#   OUT defaults to scripts/integration-test-timing.txt, the committed weights
#   a run falls back to; cache-warm.yml writes the cached ones with -o.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
target="$root/scripts/integration-test-timing.txt"
header="$target"
if [[ "${1:-}" == "-o" ]]; then
  target="${2:?-o needs a file}"
  shift 2
fi
if [[ $# -eq 0 ]]; then
  echo "usage: $0 [-o OUT] <run-id | test-timing-file>...  (green ci.yml runs whose integration lane ran)" >&2
  exit 2
fi
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

n=0
for input in "$@"; do
  n=$((n + 1))
  if [[ -f "$input" ]]; then
    cp "$input" "$work/in-$n.txt"
  else
    gh run download "$input" --name test-timing-integration --dir "$work/run-$n"
    cp "$work/run-$n/test-timing.txt" "$work/in-$n.txt"
  fi
  if [[ ! -s "$work/in-$n.txt" ]]; then
    echo "FAIL: no test durations in $input — pick a run whose integration fan-in passed" >&2
    exit 1
  fi
done

{
  grep '^#' "$header"
  # A malformed line stops the refresh, so the old weights stay: awk reads a
  # non-numeric duration as 0, which would weigh that test as free.
  cat "$work"/in-*.txt | awk -F'|' -v slow="${SLOW_TEST:-1}" '
    NF != 4 || $4 !~ /^[0-9]+(\.[0-9]+)?$/ {
      printf "FAIL: not `dir|package|Test|seconds`: %s\n", $0 > "/dev/stderr"; bad = 1; exit 1
    }
    { test = $1 "|" $2 "|" $3; secs[test] += $4; runs[test]++; valid++ }
    END {
      if (bad) exit 1
      if (!valid) { print "FAIL: no test durations to write" > "/dev/stderr"; exit 1 }
      for (test in runs) {
        mean = secs[test] / runs[test]
        if (mean >= slow) { printf "%s|%.2f\n", test, mean; continue }
        split(test, f, "|")
        key = f[1] "|" f[2]
        rest[key] += mean; count[key]++
      }
      for (key in count) printf "%s|*|%.3f\n", key, rest[key] / count[key]
    }
  ' | LC_ALL=C sort
} > "$work/timing.txt"
mv "$work/timing.txt" "$target"
echo "OK: $(grep -vc '^#' "$target") lines from $n run(s) written to $target"
