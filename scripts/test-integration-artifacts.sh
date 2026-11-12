#!/usr/bin/env bash
# test-integration-artifacts.sh — unit test for scripts/lib-integration-artifacts.sh,
# what a shard of the parallel integration lane keeps for the fan-in and for a
# continuation.
#
# A shard that lost a failing part's log still reports red, so nothing else
# would notice. Five properties:
#   1. two failing parts of one package keep two logs, each under a name that
#      does not move with the dispatch order, and no two parts share a name;
#   2. every part keeps the tests it was given and the tests that reported;
#   3. rerun.txt holds exactly the tests not passed, a test a killed part never
#      reached included, and none that passed;
#   4. the fan-in check refuses a missing part, a test in two parts and a part
#      without its lists, and accepts a shard that was dealt no tests or, from
#      the queue, claimed none;
#   5. a step that cannot be written makes the function fail, not the shell exit;
#   6. a part with more tests than a pipe buffer holds keeps its evidence.
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
[[ "$logs" == "backend@internal%2Fcompose@part1of2.log backend@internal%2Fcompose@part2of2.log backend@internal%2Fnotes.log " ]] \
  || fail "failed-logs holds [$logs], want both compose parts and the whole notes package"
grep -q 'TestA' "$forward/out/failed-logs/backend@internal%2Fcompose@part1of2.log" \
  || fail "part 1 of compose is not the part holding TestA"
grep -q 'TestC' "$forward/out/failed-logs/backend@internal%2Fcompose@part2of2.log" \
  || fail "part 2 of compose is not the part holding TestC"
reverse="$work/reverse"
shard_fixture "$reverse" reverse
for name in backend@internal%2Fcompose@part1of2 backend@internal%2Fcompose@part2of2; do
  cmp -s "$forward/out/parts/$name.assigned.txt" "$reverse/out/parts/$name.assigned.txt" \
    || fail "$name names other tests when the lane dispatches its parts in another order"
done

# 2. Every part keeps what it was given and what reported.
[[ "$(cat "$forward/out/parts/backend@internal%2Fcompose@part1of2.assigned.txt")" == $'backend|./internal/compose|TestA\nbackend|./internal/compose|TestB' ]] \
  || fail "part 1 of compose lists the wrong assigned tests"
[[ "$(cat "$forward/out/parts/backend@internal%2Fcompose@part1of2.ran.txt")" == 'backend|./internal/compose|TestA' ]] \
  || fail "part 1 of compose lists the wrong tests as reported"
[[ -f "$forward/out/parts/backend@internal%2Fdeals.ran.txt" ]] || fail "a passing package kept no lists"

# 3. Owed: the failure and the test the killed part never reached, nothing passed.
want=$'backend|./internal/compose|TestB\nbackend|./internal/compose|TestC\nbackend|./internal/notes|TestN'
[[ "$(cat "$forward/out/rerun.txt")" == "$want" ]] \
  || fail "rerun.txt holds [$(tr '\n' ' ' < "$forward/out/rerun.txt")], want TestB, TestC and TestN only"

# 4. The fan-in check.
reconcile_parts "$forward/out" > /dev/null || fail "a complete shard did not reconcile"
broken="$work/broken"
cp -R "$forward/out" "$broken"
rm "$broken/parts/backend@internal%2Fnotes.assigned.txt"
reconcile_parts "$broken" > /dev/null && fail "a shard missing a part's tests reconciled"
rm -rf "$broken" && cp -R "$forward/out" "$broken"
for list in assigned ran rerun; do
  cp "$broken/parts/backend@internal%2Fdeals.$list.txt" "$broken/parts/again.$list.txt"
done
reconcile_parts "$broken" > /dev/null && fail "a test held by two parts reconciled"
rm -rf "$broken" && cp -R "$forward/out" "$broken"
rm "$broken/parts/backend@internal%2Fdeals.rerun.txt"
reconcile_parts "$broken" > /dev/null && fail "a part without its rerun list reconciled"

# Names that used to collide: a split ./foo beside a package literally named
# ./foo--part1of2, ./a.b beside ./a/b, and one package path in two modules.
names="$work/names"
mkdir -p "$names/logs" "$names/regex" "$names/out"
printf 'backend|./foo\nbackend|./foo\nbackend|./foo--part1of2\nbackend|./a.b\nbackend|./a/b\nbackend|./x\nextensions/y|./x\n' > "$names/work"
: > "$names/failed"
slot=0
for test in TestA TestB TestF TestDot TestSlash TestBackend TestExtension; do
  slot=$((slot + 1))
  printf '^(%s)$' "$test" > "$names/regex/$slot"
  printf -- '--- FAIL: %s (0.01s)\nEXIT 1\n' "$test" > "$names/logs/$slot.log"
  sed -n "${slot}p" "$names/work" | awk -v s="$slot" '{ print $0 "|" s ".log" }' >> "$names/failed"
done
keep_shard_evidence "$names/out" "$names/work" "$names/regex" "$names/logs" "$names/failed"
[[ "$(ls "$names/out/failed-logs" | wc -l | tr -d ' ')" == 7 ]] \
  || fail "seven failing parts kept [$(ls "$names/out/failed-logs" | tr '\n' ' ')]; two of them share a name"

# A shard dealt no tests keeps empty lists and reconciles.
empty="$work/empty"
mkdir -p "$empty/regex" "$empty/logs" "$empty/out"
: > "$empty/work"
: > "$empty/failed"
: > "$empty/out/assigned.txt"
keep_shard_evidence "$empty/out" "$empty/work" "$empty/regex" "$empty/logs" "$empty/failed" \
  || fail "a shard dealt no tests could not keep its evidence"
[[ ! -s "$empty/out/rerun.txt" && ! -d "$empty/out/failed-logs" ]] || fail "a shard dealt no tests owes something"
reconcile_parts "$empty/out" > /dev/null || fail "a shard dealt no tests did not reconcile"
printf 'backend|./internal/deals|TestX\n' > "$empty/out/assigned.txt"
reconcile_parts "$empty/out" > /dev/null && fail "a shard assigned tests but holding no parts reconciled"

# A queue shard that claimed slots 2 and 3 keeps those parts alone, under the
# names they have in the whole lane.
queue="$work/queue"
cp -R "$forward" "$queue"
rm -rf "$queue/out/parts" "$queue/out/failed-logs"
printf 'backend|./internal/compose|TestA\nbackend|./internal/compose|TestB\nbackend|./internal/deals|TestX\n' > "$queue/out/assigned.txt"
keep_shard_evidence "$queue/out" "$queue/work" "$queue/regex" "$queue/logs" "$queue/failed" --only 2 3 \
  || fail "a queue shard could not keep the evidence of the parts it claimed"
kept="$(cd "$queue/out/parts" && ls ./*.assigned.txt | LC_ALL=C sort | tr '\n' ' ')"
[[ "$kept" == "./backend@internal%2Fcompose@part1of2.assigned.txt ./backend@internal%2Fdeals.assigned.txt " ]] \
  || fail "a queue shard that claimed slots 2 and 3 kept [$kept]"
reconcile_parts "$queue/out" > /dev/null || fail "a queue shard's claimed parts did not reconcile with its slice"
keep_shard_evidence "$queue/out" "$queue/work" "$queue/regex" "$queue/logs" "$queue/failed" 2 3 2> /dev/null \
  && fail "slots without --only were read as a claim"

# A queue shard that started after the others took every group claimed none:
# it keeps no part, owes nothing, and reconciles with its empty slice.
idle="$work/idle"
cp -R "$forward" "$idle"
rm -rf "$idle/out/parts" "$idle/out/failed-logs"
: > "$idle/out/assigned.txt"
keep_shard_evidence "$idle/out" "$idle/work" "$idle/regex" "$idle/logs" "$idle/failed" --only \
  || fail "a queue shard that claimed nothing could not keep its evidence"
[[ -z "$(ls "$idle/out/parts")" ]] || fail "a queue shard that claimed nothing kept [$(ls "$idle/out/parts" | tr '\n' ' ')]"
[[ ! -s "$idle/out/rerun.txt" && ! -d "$idle/out/failed-logs" ]] || fail "a queue shard that claimed nothing owes something"
reconcile_parts "$idle/out" > /dev/null || fail "a queue shard that claimed nothing did not reconcile"

# 5. A part whose selector is gone fails the function, and the shell runs on.
lost="$work/lost"
cp -R "$forward" "$lost"
rm "$lost/regex/3"
rm -rf "$lost/out/parts" "$lost/out/failed-logs"
if keep_shard_evidence "$lost/out" "$lost/work" "$lost/regex" "$lost/logs" "$lost/failed" 2> /dev/null; then
  fail "a part with no selector was kept as if complete"
fi

# 6. A part whose sorted tests outgrow a pipe buffer is still named and kept.
long="$work/long"
mkdir -p "$long/regex" "$long/logs" "$long/out"
printf 'backend|./internal/long\n' > "$long/work"
: > "$long/failed"
printf '^(%s)$' "$(seq 1 50000 | sed 's/^/TestLong/' | paste -sd'|' -)" > "$long/regex/1"
printf -- '--- PASS: TestLong1 (0.01s)\nok\nEXIT 0\n' > "$long/logs/1.log"
seq 1 50000 | sed 's/^/backend|.\/internal\/long|TestLong/' | LC_ALL=C sort > "$long/out/assigned.txt"
keep_shard_evidence "$long/out" "$long/work" "$long/regex" "$long/logs" "$long/failed" \
  || fail "a part holding 50000 tests could not keep its evidence"

echo "OK: each failing part keeps its own log and lists, and the rerun list holds what is owed"
