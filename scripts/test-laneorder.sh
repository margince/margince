#!/usr/bin/env bash
# test-laneorder.sh — unit test for scripts/lib-laneorder.sh, the parallel
# integration lane's dispatch order.
#
# The order is a scheduling hint, so getting it wrong never turns a lane red —
# it just makes the run longer, or, in the case this exists to catch, shorter
# for the packages that do not matter and longer for the one that does. That
# silence is the whole reason for a test: a regression here is invisible on
# every dashboard and shows up only as a lane that drifted back to costing what
# it cost before the hint was added.
#
# Three properties, each of which has a failure mode nothing else would report:
#   1. a named package sorts by its recorded duration, longest first;
#   2. an UNNAMED package keeps the order it arrived in — a stale baseline must
#      degrade to no-hint behaviour, not to an order the sort invented;
#   3. the measured hint beats the committed baseline, and an empty or missing
#      file is not a hint at all;
#   4. the shard split holds every test once and spreads slow tests apart.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root_dir/scripts/lib-laneorder.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fails=0

fail() {
  echo "FAIL: $*" >&2
  fails=$((fails + 1))
}

expect_order() {
  local label="$1" want="$2" got="$3"
  if [[ "$want" != "$got" ]]; then
    fail "$label
  want: $(echo "$want" | tr '\n' ' ')
  got:  $(echo "$got" | tr '\n' ' ')"
  fi
}

# The regex field carries its own pipes, exactly as the dispatcher's does. If
# the helper ever split on them, these lines would come back mangled.
cat > "$work/grouped" <<'GROUPED'
backend|./internal/compose/integration|^(TestA|TestB)$
backend|./internal/modules/contacts|^(TestC)$
backend|./migrations|^(TestD)$
backend|./cmd/worker|^(TestE)$
GROUPED

cat > "$work/measured" <<'HINT'
./internal/modules/contacts|31.500
./migrations|104.135
./internal/compose/integration|221.093
./cmd/worker|4.498
HINT

# 1. Longest first, and the -run regex survives the trip intact.
expect_order "named packages must sort longest-first" \
  './internal/compose/integration
./migrations
./internal/modules/contacts
./cmd/worker' \
  "$(order_by_hint "$work/grouped" "$work/measured" | cut -d'|' -f2)"

if ! order_by_hint "$work/grouped" "$work/measured" | grep -qxF 'backend|./internal/compose/integration|^(TestA|TestB)$'; then
  fail "the -run regex did not survive ordering — its own pipes were treated as fields"
fi

# 2. A baseline that names only one package: the three it does not name sort
# ahead of it (unknown is treated as heaviest) AND keep their input order.
# Without a stable sort they come back reversed, which is the regression.
cat > "$work/partial" <<'HINT'
./internal/compose/integration|221.093
HINT
expect_order "unnamed packages must keep the order they arrived in" \
  './internal/modules/contacts
./migrations
./cmd/worker
./internal/compose/integration' \
  "$(order_by_hint "$work/grouped" "$work/partial" | cut -d'|' -f2)"

# A baseline naming nothing this tree still has must leave the order untouched.
cat > "$work/stale" <<'HINT'
./internal/modules/gone|900.000
./internal/modules/alsogone|800.000
HINT
expect_order "a wholly stale baseline must degrade to discovery order" \
  "$(cut -d'|' -f2 "$work/grouped")" \
  "$(order_by_hint "$work/grouped" "$work/stale" | cut -d'|' -f2)"

# A duration the lane could not record reaches the hint as an empty field, and
# awk reads that as zero — which would sort the package LAST. It has to read as
# "no time recorded" instead, or one unmeasurable package silently becomes the
# thing the whole run waits for.
cat > "$work/unmeasured" <<'HINT'
./internal/compose/integration|
./internal/modules/contacts|31.500
./migrations|not-a-number
./cmd/worker|0
HINT
expect_order "a duration that is not a positive number is no duration at all" \
  './internal/compose/integration
./migrations
./cmd/worker
./internal/modules/contacts' \
  "$(order_by_hint "$work/grouped" "$work/unmeasured" | cut -d'|' -f2)"

# The committed baseline carries a human-readable header. It must not disturb the
# order: an unrecognised key is one nothing looks up, and this pins that reading
# rather than leaving it to be rediscovered the next time somebody edits the file.
cat > "$work/commented" <<'HINT'
# a header a contact wrote

./migrations|104.135
HINT
expect_order "a header in a hint file leaves the order alone" \
  './internal/compose/integration
./internal/modules/contacts
./cmd/worker
./migrations' \
  "$(order_by_hint "$work/grouped" "$work/commented" | cut -d'|' -f2)"


# 3. Hint resolution.
: > "$work/empty"
cat > "$work/baseline" <<'HINT'
./cmd/worker|4.498
HINT
[[ "$(resolve_order_hint "$work/measured" "$work/baseline")" == "$work/measured" ]] \
  || fail "a measured hint must beat the committed baseline"
[[ "$(resolve_order_hint "$work/empty" "$work/baseline")" == "$work/baseline" ]] \
  || fail "an EMPTY measured hint must fall through to the committed baseline"
[[ "$(resolve_order_hint "$work/absent" "$work/baseline")" == "$work/baseline" ]] \
  || fail "an ABSENT measured hint must fall through to the committed baseline"
[[ -z "$(resolve_order_hint "$work/absent" "$work/alsoabsent")" ]] \
  || fail "with neither file present there is no hint, and the lane dispatches in discovery order"

# A hint whose only content is a header must order nothing rather than sort every
# package behind a phantom named '#'.
cat > "$work/commented" <<'HINT'
# a header a contact wrote

./migrations|104.135
HINT
expect_order "comment and blank lines in a hint are not packages" \
  './internal/compose/integration
./internal/modules/contacts
./cmd/worker
./migrations' \
  "$(order_by_hint "$work/grouped" "$work/commented" | cut -d'|' -f2)"

# The committed baseline itself has to be usable by the helper it feeds: every
# line `package|seconds`, and every package a directory this tree still has. A
# baseline of nothing but dead paths is indistinguishable from no baseline, and
# nothing else in the tree would say so.
baseline="$root_dir/scripts/integration-lane-timing.txt"
if [[ ! -s "$baseline" ]]; then
  fail "$baseline is missing or empty — the committed dispatch baseline is what a fresh clone orders by"
else
  live=0
  while IFS='|' read -r pkg secs; do
    [[ -z "$pkg" || "$pkg" == \#* ]] && continue
    if [[ ! "$secs" =~ ^[0-9]+(\.[0-9]+)?$ ]]; then
      fail "baseline line for '$pkg' has a non-numeric duration '$secs' — awk would read it as 0 and sort the package last"
      continue
    fi
    [[ -d "$root_dir/backend/${pkg#./}" ]] && live=$((live + 1))
  done < "$baseline"
  if (( live == 0 )); then
    fail "no package named in $baseline still exists — the baseline orders nothing"
  fi
fi

# 4. Shard assignment. A wrong split never turns the lane red by itself: a
# missing or doubled test is caught by the fan-in, and a lopsided one is only a
# slow PR. So both halves are checked here.
# TestNew is in no timing line, so it weighs ./small's `*` row; ./gone is in no
# discovery line. The union check below covers both.
cat > "$work/discovery" <<'TESTS'
backend|./big|TestA
backend|./big|TestB
backend|./big|TestC
backend|./big|TestD
backend|./big|TestE
backend|./big|TestF
backend|./small|TestG
backend|./small|TestH
backend|./small|TestNew
TESTS
cat > "$work/timing" <<'TIMES'
# a header
backend|./big|TestA|60.0
backend|./big|TestB|1.0
backend|./big|TestC|1.0
backend|./big|TestD|30.0
backend|./big|TestE|30.0
backend|./big|TestF|1.0
backend|./small|TestG|2.0
backend|./small|TestH|4.0
backend|./small|*|3.0
backend|./gone|TestOld|90.0
TIMES

union=""
for k in 1 2 3; do
  assign_shard "$work/discovery" "$work/timing" "$k" 3 > "$work/shard$k"
  union+="$(cat "$work/shard$k")"$'\n'
done
expect_order "the shards together must hold every test exactly once" \
  "$(LC_ALL=C sort "$work/discovery")" "$(printf '%s' "$union" | LC_ALL=C sort)"
expect_order "the same inputs must give the same split" \
  "$(cat "$work/shard2")" "$(assign_shard "$work/discovery" "$work/timing" 2 3)"
# Split by count, TestA and TestD share shard 1 (90 s against 31 s and 2 s).
# By time, the 60 s test runs with no other test of its package.
if ! grep -qx 'backend|./big|TestA' "$work/shard1" || grep -q 'TestD\|TestE' "$work/shard1"; then
  fail "the slowest test must not share a shard with the next slowest ones
  shard1: $(tr '\n' ' ' < "$work/shard1")"
fi
for k in 1 2 3; do
  load=$(awk -F'|' 'NR == FNR { if (NF == 4) t[$1 "|" $2 "|" $3] = $4; next } $2 == "./big" { s += t[$0] } END { print s + 0 }' "$work/timing" "$work/shard$k")
  if (( ${load%.*} > 61 )); then
    fail "shard $k holds ${load}s of ./big; balanced, no shard holds more than the 60 s test plus a 1 s one"
  fi
done

# The `*` row weighs the tests the file does not name. At 6 s each, one of the
# three unnamed tests joins the named 10 s one; at the 1 s default, none would.
printf 'backend|./mid|TestP\nbackend|./mid|TestQ\nbackend|./mid|TestR\nbackend|./mid|TestS\n' > "$work/middisc"
printf 'backend|./mid|TestP|10.0\nbackend|./mid|*|6.0\n' > "$work/midtime"
count=$(assign_shard "$work/middisc" "$work/midtime" 1 2 | wc -l | tr -d ' ')
[[ "$count" -eq 2 ]] || fail "shard 1 of 2 got $count of ./mid's tests; the * row weighs the unnamed ones at 6 s, so it must hold 2"

: > "$work/notiming"
for k in 1 2 3; do
  count=$(assign_shard "$work/discovery" "$work/notiming" "$k" 3 | grep -c '|./big|' || true)
  [[ "$count" -eq 2 ]] || fail "with no timing, shard $k got $count of ./big's 6 tests; it must split by count, 2 each"
done

# The committed shard timing has to be readable by assign_shard: four fields,
# a number last. A line it cannot read silently weighs its test at the mean.
shard_timing="$root_dir/scripts/integration-test-timing.txt"
bad=$(grep -v '^#' "$shard_timing" | awk -F'|' 'NF != 4 || $4 !~ /^[0-9]+(\.[0-9]+)?$/' | head -3)
[[ -z "$bad" ]] || fail "$shard_timing has lines assign_shard cannot read: $bad"
live=$(grep -v '^#' "$shard_timing" | cut -d'|' -f2 | LC_ALL=C sort -u | while read -r pkg; do
  [[ -d "$root_dir/backend/${pkg#./}" ]] && echo "$pkg"
done | wc -l | tr -d ' ')
(( live > 0 )) || fail "no package named in $shard_timing still exists — every shard splits by count"

if (( fails > 0 )); then
  echo "test-laneorder: $fails failure(s)" >&2
  exit 1
fi
echo "test-laneorder: OK"
