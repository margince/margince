#!/usr/bin/env bash
# The shard union gate's own test. Every case is a SYNTHETIC pair — the real
# merged report is written only by the CI lane and is supposed to pass, and a
# gate proven by "today's report is fine" keeps passing after it stops working.
#
# The shapes below are the ways a sharded lane gets smaller without going red:
# a leg that never reported, an N that stopped matching the matrix, a blob from
# another tree, two slices that overlap.
#
# Usage: bash frontend/scripts/check-shard-union.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-shard-union.sh"
FRONTEND_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0

# A merged report holding one entry per argument, in the shape vitest's json
# reporter writes: an absolute path under the frontend root.
write_report() {
  local name="$1"
  shift
  {
    printf '{"testResults":['
    local first=1 path
    for path in "$@"; do
      [[ "$first" -eq 1 ]] || printf ','
      first=0
      case "$path" in
        /*) printf '{"name":"%s"}' "$path" ;;
        *) printf '{"name":"%s/%s"}' "$FRONTEND_ROOT" "$path" ;;
      esac
    done
    printf ']}'
  } >"$TMP/$name"
  echo "$TMP/$name"
}

write_list() {
  local name="$1"
  shift
  printf '%s\n' "$@" >"$TMP/$name"
  echo "$TMP/$name"
}

expect_refusal() {
  local name="$1" report="$2" list="$3" want="$4"
  local out status=0
  out="$(bash "$GATE" "$report" "$list" 2>&1)" || status=$?
  if [[ "$status" -eq 0 ]]; then
    echo "FAIL: the gate accepted $name" >&2
    FAILURES=$((FAILURES + 1))
  elif ! grep -q -- "$want" <<<"$out"; then
    echo "FAIL: $name refused, but the message does not mention '$want':" >&2
    echo "$out" >&2
    FAILURES=$((FAILURES + 1))
  fi
}

expect_accepted() {
  local name="$1" report="$2" list="$3"
  local out status=0
  out="$(bash "$GATE" "$report" "$list" 2>&1)" || status=$?
  if [[ "$status" -ne 0 ]]; then
    echo "FAIL: the gate refused $name:" >&2
    echo "$out" >&2
    FAILURES=$((FAILURES + 1))
  fi
}

THREE="$(write_list discovered.txt "src/a.test.tsx" "src/b.test.tsx" "src/c.test.tsx")"

# The shape a healthy lane produces: the slices add up to the suite exactly.
expect_accepted "a report covering every discovered file once" \
  "$(write_report complete.json "src/a.test.tsx" "src/b.test.tsx" "src/c.test.tsx")" "$THREE"

# THE defect this gate exists for: one matrix leg never reported, so a third of
# the suite is missing from a report that says every test in it passed.
expect_refusal "a report missing a shard's files" \
  "$(write_report short.json "src/a.test.tsx" "src/b.test.tsx")" "$THREE" \
  "ran in no shard"

# Two slices that overlap — an N that no longer matches the matrix runs some
# files twice and, between them, still misses none.
expect_refusal "a report running one file in two shards" \
  "$(write_report overlapping.json "src/a.test.tsx" "src/b.test.tsx" "src/b.test.tsx" "src/c.test.tsx")" \
  "$THREE" "ran in more than one shard"

# A blob written against a different checkout: its paths resolve nowhere under
# this frontend root, so nothing about it describes this tree.
expect_refusal "a report from another checkout" \
  "$(write_report foreign.json "/elsewhere/frontend/src/a.test.tsx" "src/b.test.tsx" "src/c.test.tsx")" \
  "$THREE" "discovery does not list"

# The merge ran over no blobs at all. It succeeds, writes a report, and every
# reader downstream sees a lane that passed.
expect_refusal "a report holding no test file" \
  "$(write_report empty.json)" "$THREE" "holds no test file at all"

# Discovery itself came back empty — the comparison would be vacuous, which is
# the one answer a census must never give quietly.
expect_refusal "an empty discovery list" \
  "$(write_report complete2.json "src/a.test.tsx")" "$(write_list empty.txt "")" \
  "discovery list is empty"

# Neither input written, the case where the merge step never ran.
expect_refusal "a merged report that was never written" \
  "$TMP/absent.json" "$THREE" "no file at"

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "check-shard-union.test.sh: $FAILURES case(s) failed" >&2
  exit 1
fi

echo "PASS — check-shard-union.sh holds on all 7 cases"
