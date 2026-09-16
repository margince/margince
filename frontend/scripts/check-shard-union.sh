#!/usr/bin/env bash
# Shard union gate: the merged report describes every test file the suite has,
# each of them exactly once.
#
# A sharded lane reaches its verdict by addition, and addition is the one thing
# that can fail SHORT without anything going red. A matrix leg that never
# started, a `--shard=k/N` whose N stopped matching the matrix, a blob left over
# from an earlier commit — each of those merges cleanly into a report that says
# "passed" about a smaller suite than the one this repository has, and the lcov
# built from it hands SonarCloud a measurement of the part that ran as though it
# were the whole. Nothing downstream can tell the difference: fewer files is not
# an error to any of it.
#
# So the count is checked against the suite's own discovery rather than against
# a number written down here. vitest globs the test files; the merged report
# says which of them ran; this compares the two SETS, in both directions.
#
# Usage: frontend/scripts/check-shard-union.sh <merged json report> <discovered file list>
#        (wired into `make fe-unit-merge`, which produces both)

set -euo pipefail

REPORT="${1:-}"
DISCOVERED="${2:-}"
if [[ -z "$REPORT" || -z "$DISCOVERED" ]]; then
  echo "usage: $0 <merged json report> <discovered file list>" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FRONTEND_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

for input in "$REPORT" "$DISCOVERED"; do
  if [[ ! -f "$input" ]]; then
    echo "FAIL: no file at $input — the merge step did not produce what this gate reads" >&2
    exit 1
  fi
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# One line per test file the report holds, DUPLICATES KEPT: a file that two
# shards both ran is a split that is not a partition, and deduplicating here
# would hide exactly that.
node -e '
  const [report, root] = process.argv.slice(1);
  const parsed = JSON.parse(require("node:fs").readFileSync(report, "utf8"));
  const results = parsed.testResults ?? [];
  const prefix = root.endsWith("/") ? root : root + "/";
  for (const result of results) {
    const name = result.name ?? "";
    process.stdout.write((name.startsWith(prefix) ? name.slice(prefix.length) : name) + "\n");
  }
' "$REPORT" "$FRONTEND_ROOT" >"$TMP/ran.txt"

sed '/^$/d' "$DISCOVERED" | sort >"$TMP/discovered.sorted"
sed '/^$/d' "$TMP/ran.txt" | sort >"$TMP/ran.sorted"

RAN_TOTAL="$(wc -l <"$TMP/ran.sorted" | tr -d ' ')"
DISCOVERED_TOTAL="$(wc -l <"$TMP/discovered.sorted" | tr -d ' ')"

echo "==> shard union check ($RAN_TOTAL entries in $REPORT, $DISCOVERED_TOTAL files discovered)"

if [[ "$DISCOVERED_TOTAL" -eq 0 ]]; then
  echo "FAIL: the discovery list is empty — vitest found no test files, so this gate has nothing to compare against" >&2
  exit 1
fi

if [[ "$RAN_TOTAL" -eq 0 ]]; then
  echo "FAIL: the merged report holds no test file at all — the shards' blobs never reached the merge" >&2
  exit 1
fi

STATUS=0

MISSING="$(comm -23 "$TMP/discovered.sorted" <(sort -u "$TMP/ran.sorted"))"
if [[ -n "$MISSING" ]]; then
  echo "FAIL: $(wc -l <<<"$MISSING" | tr -d ' ') discovered test file(s) ran in no shard:" >&2
  head -5 <<<"$MISSING" | sed 's/^/      /' >&2
  echo "" >&2
  echo "A shard that does not run is a suite that gets smaller in silence. Check" >&2
  echo "that every matrix leg reported, and that the N in --shard=k/N is the" >&2
  echo "number of legs." >&2
  STATUS=1
fi

UNEXPECTED="$(comm -13 "$TMP/discovered.sorted" <(sort -u "$TMP/ran.sorted"))"
if [[ -n "$UNEXPECTED" ]]; then
  echo "FAIL: $(wc -l <<<"$UNEXPECTED" | tr -d ' ') file(s) in the report that discovery does not list:" >&2
  head -5 <<<"$UNEXPECTED" | sed 's/^/      /' >&2
  echo "" >&2
  echo "A blob written against a different tree is being merged into this one." >&2
  STATUS=1
fi

DUPLICATED="$(uniq -d "$TMP/ran.sorted")"
if [[ -n "$DUPLICATED" ]]; then
  echo "FAIL: $(wc -l <<<"$DUPLICATED" | tr -d ' ') file(s) ran in more than one shard:" >&2
  head -5 <<<"$DUPLICATED" | sed 's/^/      /' >&2
  echo "" >&2
  echo "The slices overlap, so the merged coverage counts that file's tests twice" >&2
  echo "and the lane pays for them twice." >&2
  STATUS=1
fi

if [[ "$STATUS" -ne 0 ]]; then
  exit 1
fi

echo "PASS — the shards partition the suite: $DISCOVERED_TOTAL files, each run exactly once"
