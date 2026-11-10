#!/usr/bin/env bash
# test-integration-artifacts.sh — unit test for scripts/lib-integration-artifacts.sh,
# what a shard of the parallel integration lane keeps for the fan-in and for a
# continuation.
#
# A shard that lost a failing part's log still reports red, so nothing else
# would notice. Four properties:
#   1. two failing parts of one package keep two logs, each under a name that
#      does not move with the dispatch order; a whole package keeps its old name;
#   2. every part keeps the tests it was given and the tests that reported;
#   3. rerun.txt holds exactly the tests not passed, a test a killed part never
#      reached included, and none that passed;
#   4. the fan-in check refuses a missing part, a test in two parts and a part
#      without its lists.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root_dir/scripts/lib-integration-artifacts.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail() {
  echo "FAIL: $*"
  exit 1
}

# shard_fixture DIR ORDER — one shard's run, its parts dispatched in ORDER:
# "forward" puts the second compose part first, "reverse" the first.
shard_fixture() {
  local dir="$1" order="$2"
  mkdir -p "$dir/logs" "$dir/regex"
  local first=1 second=2
  [[ "$order" == forward ]] || { first=2; second=1; }
  printf 'backend|./internal/compose\nbackend|./internal/compose\nbackend|./internal/deals\nbackend|./internal/notes\n' > "$dir/work"
  # The compose part holding TestC and TestD fails TestC; the one holding
  # TestA and TestB is killed after TestA.
  printf '^(TestC|TestD)$' > "$dir/regex/$first"
  printf -- '--- FAIL: TestC (0.01s)\n--- PASS: TestD (0.01s)\nFAIL\nEXIT 1\n' > "$dir/logs/$first.log"
  printf '^(TestA|TestB)$' > "$dir/regex/$second"
  printf -- '--- PASS: TestA (0.01s)\n*** Test killed: ran too long\nEXIT 2\n' > "$dir/logs/$second.log"
  printf '^(TestX)$' > "$dir/regex/3"
  printf -- '--- PASS: TestX (0.01s)\nok\nEXIT 0\n' > "$dir/logs/3.log"
  printf '^(TestN)$' > "$dir/regex/4"
  printf -- '--- FAIL: TestN (0.01s)\nFAIL\nEXIT 1\n' > "$dir/logs/4.log"
  printf 'backend|./internal/compose|1.log\nbackend|./internal/compose|2.log\nbackend|./internal/notes|4.log\n' > "$dir/failed"
  mkdir -p "$dir/out"
  printf 'backend|./internal/compose|Test%s\n' A B C D > "$dir/out/assigned.txt"
  printf 'backend|./internal/deals|TestX\nbackend|./internal/notes|TestN\n' >> "$dir/out/assigned.txt"
  LC_ALL=C sort -o "$dir/out/assigned.txt" "$dir/out/assigned.txt"
  keep_shard_evidence "$dir/out" "$dir/work" "$dir/regex" "$dir/logs" "$dir/failed"
}

forward="$work/forward"
shard_fixture "$forward" forward

# 1. Two failing parts, two logs, named by rank and not by slot.
logs="$(cd "$forward/out/failed-logs" && ls | LC_ALL=C sort | tr '\n' ' ')"
[[ "$logs" == "__internal_compose--part1of2.log __internal_compose--part2of2.log __internal_notes.log " ]] \
  || fail "failed-logs holds [$logs], want both compose parts and the whole notes package"
grep -q 'TestA' "$forward/out/failed-logs/__internal_compose--part1of2.log" \
  || fail "part 1 of compose is not the part holding TestA"
grep -q 'TestC' "$forward/out/failed-logs/__internal_compose--part2of2.log" \
  || fail "part 2 of compose is not the part holding TestC"
reverse="$work/reverse"
shard_fixture "$reverse" reverse
for name in __internal_compose--part1of2 __internal_compose--part2of2; do
  cmp -s "$forward/out/parts/$name.assigned.txt" "$reverse/out/parts/$name.assigned.txt" \
    || fail "$name names other tests when the lane dispatches its parts in another order"
done

# 2. Every part keeps what it was given and what reported.
[[ "$(cat "$forward/out/parts/__internal_compose--part1of2.assigned.txt")" == $'backend|./internal/compose|TestA\nbackend|./internal/compose|TestB' ]] \
  || fail "part 1 of compose lists the wrong assigned tests"
[[ "$(cat "$forward/out/parts/__internal_compose--part1of2.ran.txt")" == 'backend|./internal/compose|TestA' ]] \
  || fail "part 1 of compose lists the wrong tests as reported"
[[ -f "$forward/out/parts/__internal_deals.ran.txt" ]] || fail "a passing package kept no lists"

# 3. Owed: the failure and the test the killed part never reached, nothing passed.
want=$'backend|./internal/compose|TestB\nbackend|./internal/compose|TestC\nbackend|./internal/notes|TestN'
[[ "$(cat "$forward/out/rerun.txt")" == "$want" ]] \
  || fail "rerun.txt holds [$(tr '\n' ' ' < "$forward/out/rerun.txt")], want TestB, TestC and TestN only"

# 4. The fan-in check.
reconcile_parts "$forward/out" > /dev/null || fail "a complete shard did not reconcile"
broken="$work/broken"
cp -R "$forward/out" "$broken"
rm "$broken/parts/__internal_notes.assigned.txt"
reconcile_parts "$broken" > /dev/null && fail "a shard missing a part's tests reconciled"
rm -rf "$broken" && cp -R "$forward/out" "$broken"
for list in assigned ran rerun; do
  cp "$broken/parts/__internal_deals.$list.txt" "$broken/parts/again.$list.txt"
done
reconcile_parts "$broken" > /dev/null && fail "a test held by two parts reconciled"
rm -rf "$broken" && cp -R "$forward/out" "$broken"
rm "$broken/parts/__internal_deals.rerun.txt"
reconcile_parts "$broken" > /dev/null && fail "a part without its rerun list reconciled"

echo "OK: each failing part keeps its own log and lists, and the rerun list holds what is owed"
