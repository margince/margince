#!/usr/bin/env bash
# test-shardqueue.sh — unit test for scripts/lib-shardqueue.sh, the queue the CI
# integration shards share, and for queue_groups in scripts/lib-laneorder.sh.
#
# A queue that hands one group to two shards, or to none, still lets every
# shard pass: only the fan-in sees it, after the whole lane has run. These run
# the claim protocol with real concurrent shells against the `dir:` store, so a
# protocol bug fails here in seconds:
#   1. groups hold every part once, in order, and a heavy part stands alone;
#   2. shards that race for one queue claim every group exactly once;
#   3. a fast shard claims more than its share, and a shard that never starts
#      loses none of its groups;
#   4. a later attempt skips the groups green shards reported, and refuses an
#      artifact cut from other parts;
#   5. the dispatcher runs each claimed part once, never two at once on one slot;
#   6. a store that fails stops the claims and says so; the cache store reads
#      the service's answers as claim, lost claim, refusal or retry;
#   7. the action that hands the results service to `run:` steps writes it
#      and masks the token.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$root_dir/scripts/lib-laneorder.sh"
source "$root_dir/scripts/lib-shardqueue.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fails=0

fail() {
  echo "FAIL: $*" >&2
  fails=$((fails + 1))
}

# 1. Groups.
cat > "$work/parts" <<'PARTS'
backend|./big|^(TestA)$
backend|./mid|^(TestB|TestC)$
backend|./small|^(TestD)$
backend|./small|^(TestE)$
backend|./tiny|^(TestF)$
PARTS
cat > "$work/timing" <<'TIMES'
backend|./big|TestA|300.0
backend|./mid|TestB|50.0
backend|./mid|TestC|50.0
backend|./small|TestD|60.0
backend|./small|TestE|40.0
backend|./tiny|TestF|1.0
TIMES
got="$(queue_groups "$work/parts" "$work/timing" 200 | tr '\n' ';')"
[[ "$got" == "1;2 3 4;5;" ]] || fail "groups of 200 s: want 1;2 3 4;5; got $got"

# 2 and 3. Shards race for one queue. A shard claims, then sleeps its own pace;
# the claims it made go to claimed-<k>, one group per line.
# Every shard waits for $work/go, so one that a loaded machine starts late is
# not mistaken for a slow one.
shard() { # store shard total groups-count pace done-file
  local store="$1" k="$2" total="$3" count="$4" pace="$5" done_file="${6:-}"
  until [[ -e "$work/go" ]]; do sleep 0.01; done
  queue_open "$store" "$k" "$total" "$work/groups-$count" "$done_file"
  : > "$work/claimed-$k"
  while queue_next && [[ -n "$QUEUE_GROUP" ]]; do
    echo "$QUEUE_GROUP" >> "$work/claimed-$k"
    sleep "$pace"
  done
}

for count in 20 40 61; do seq 1 "$count" | sed 's/$/ x/' > "$work/groups-$count"; done
start_race() { sleep 0.2; touch "$work/go"; wait; rm -f "$work/go"; }

expect_whole() { # label count [done-file]
  local label="$1" count="$2" done_file="${3:-/dev/null}" want got
  want="$(seq 1 "$count" | grep -vxF -f "$done_file" || true)"
  got="$(cat "$work"/claimed-* | LC_ALL=C sort -n)"
  if [[ "$want" != "$(printf '%s\n' "$got" | LC_ALL=C sort -nu)" ]]; then
    fail "$label: the claims do not cover every group: $(comm -3 <(printf '%s\n' "$want") <(printf '%s\n' "$got" | LC_ALL=C sort -nu) | tr '\n' ' ')"
  fi
  if [[ -n "$(printf '%s\n' "$got" | uniq -d)" ]]; then
    fail "$label: groups claimed twice: $(printf '%s\n' "$got" | uniq -d | tr '\n' ' ')"
  fi
}

for round in 1 2 3 4 5 6; do
  rm -rf "$work"/claimed-* "$work/q"
  for k in 1 2 3 4 5 6 7 8; do
    shard "dir:$work/q" "$k" 8 61 "0.0$(( (k * round) % 7 ))" &
  done
  start_race
  expect_whole "race round $round" 61
done

rm -rf "$work"/claimed-* "$work/q"
shard "dir:$work/q" 1 4 40 0 &
for k in 2 3 4; do shard "dir:$work/q" "$k" 4 40 0.1 & done
start_race
expect_whole "one fast shard" 40
fast=$(wc -l < "$work/claimed-1")
(( fast > 10 )) || fail "the fast shard claimed $fast of 40 groups; at its home share of 10 it would wait for the slow ones"

rm -rf "$work"/claimed-* "$work/q"
for k in 1 2 3; do shard "dir:$work/q" "$k" 4 40 0.01 & done
start_race
expect_whole "a shard that never started" 40

# 4. A later attempt. Groups 1 to 5 passed on another shard and are skipped.
seq 1 5 > "$work/done"
rm -rf "$work"/claimed-* "$work/q"
for k in 1 2; do shard "dir:$work/q" "$k" 4 20 0 "$work/done" & done
start_race
expect_whole "a later attempt" 20 "$work/done"

mkdir -p "$work/earlier/integration-shard-1" "$work/earlier/integration-shard-2" "$work/earlier/integration-shard-3"
printf 'shard=1\ntotal=3\ntiming=t\nqueue=D\n' > "$work/earlier/integration-shard-1/meta.txt"
printf '4 7 8\n1 1\n' > "$work/earlier/integration-shard-1/claimed.txt"
printf 'shard=2\ntotal=3\ntiming=t\nqueue=D\n' > "$work/earlier/integration-shard-2/meta.txt"
printf '2 2\n' > "$work/earlier/integration-shard-2/claimed.txt"
printf '3 3\n' > "$work/earlier/integration-shard-3/claimed.txt"
printf 'shard=3\ntotal=3\ntiming=t\nqueue=D\n' > "$work/earlier/integration-shard-3/meta.txt"
echo 'shard 3/3 failed' > "$work/earlier/integration-shard-3/failure-summary.txt"
got="$(queue_done_groups "$work/earlier" 3 D | tr '\n' ' ')"
[[ "$got" == "1 2 4 " ]] || fail "shard 3 skips what shards 1 and 2 passed and nothing a red shard took: want 1 2 4, got $got"
got="$(queue_done_groups "$work/earlier" 2 D | tr '\n' ' ')"
[[ "$got" == "1 4 " ]] || fail "a shard runs its own earlier groups again: want 1 4, got $got"
if queue_done_groups "$work/earlier" 3 OTHER > /dev/null 2>&1; then
  fail "an artifact cut from other parts must not be read as done"
fi

# 5. The dispatcher. Each part takes its slot as a directory, so two parts on
# one slot at once fail to take it.
printf 'backend|./p%s\n' 1 2 3 4 5 6 7 > "$work/work"
printf '1\n2 3\n4 5 6\n7\n' > "$work/dgroups"
run_one() { # line part outdir slot
  if ! mkdir "$3/slot-$4" 2>/dev/null; then echo "slot $4 shared by part $2" >> "$3/errors"; fi
  echo "$2 $1" >> "$3/ran"
  sleep 0.0$(( $2 % 3 ))
  rmdir "$3/slot-$4" 2>/dev/null || :
}
mkdir -p "$work/out"
: > "$work/claimed"
queue_open "dir:$work/dq" 1 1 "$work/dgroups" ""
run_queue_packages "$work/work" "$work/dgroups" 3 "$work/out" "$work/claimed" > /dev/null \
  || fail "the dispatcher failed on a store that works"
[[ ! -s "$work/out/errors" ]] || fail "$(cat "$work/out/errors")"
got="$(LC_ALL=C sort -n "$work/out/ran" | tr '\n' ';')"
[[ "$got" == "1 backend|./p1;2 backend|./p2;3 backend|./p3;4 backend|./p4;5 backend|./p5;6 backend|./p6;7 backend|./p7;" ]] \
  || fail "each part must run once with its own line: got $got"
[[ "$(cut -d' ' -f1 "$work/claimed" | tr '\n' ' ')" == "1 2 3 4 " ]] || fail "the claim record must list each group: $(tr '\n' ';' < "$work/claimed")"

# 6. A failing store: once open, the store is a file, so no claim can be made
# in it, by root either.
: > "$work/out/ran"
queue_open "dir:$work/ro" 1 1 "$work/dgroups" ""
rmdir "$work/ro"
: > "$work/ro"
if run_queue_packages "$work/work" "$work/dgroups" 3 "$work/out" "$work/claimed" > /dev/null 2>&1; then
  fail "a store that refuses every claim must fail the dispatcher"
fi
[[ ! -s "$work/out/ran" ]] || fail "a shard that claimed nothing ran $(tr '\n' ' ' < "$work/out/ran")"

# The cache store, against canned answers of the service. curl runs in a
# command substitution, so the answers left are kept in a file.
curl() { # writes the next canned body to the -o file and prints its status
  local out="" answer
  while (( $# )); do [[ "$1" == -o ]] && out="$2"; shift; done
  answer="$(head -n1 "$work/answers")"
  sed -i.bak 1d "$work/answers"
  printf '%s' "${answer#* }" > "$out"
  printf '%s' "${answer%% *}"
}
sleep() { :; }
QUEUE_BACKEND=cache QUEUE_PREFIX=p ACTIONS_RUNTIME_TOKEN=t ACTIONS_RESULTS_URL=https://results.invalid/
take() { # want answers...
  local want="$1" st=0
  shift
  printf '%s\n' "$@" > "$work/answers"
  queue_take 7 2> /dev/null || st=$?
  [[ "$st" == "$want" ]] || fail "answers $* must give $want, gave $st"
}
take 0 '200 {"ok":true,"signed_upload_url":"u"}'
take 1 '200 {"ok":false,"message":"already exists"}'
take 1 '409 {"code":"already_exists","msg":"exists"}'
take 3 '200 {"ok":false,"message":"cache write denied: read-only token"}'
take 3 '403 {"code":"permission_denied","msg":"no"}'
take 0 '429 {"msg":"slow down"}' '502 {}' '200 {"ok":true}'
QUEUE_RETRY_SECONDS=5 take 2 '503 {}' '503 {}' '503 {}' '503 {}' '503 {}' '503 {}'

# The build's choice of mode, from the same answers.
export -f curl sleep
export work
mode() { # answer
  printf '%s\n' "$1" > "$work/answers"
  ACTIONS_RUNTIME_TOKEN=t ACTIONS_RESULTS_URL=https://results.invalid/ "$root_dir/scripts/shard-queue-mode.sh" p 2> /dev/null
}
[[ "$(mode '200 {"ok":true}')" == queue ]] || fail "a claim the service takes must choose the queue"
[[ "$(mode '200 {"ok":false,"message":"cache write denied: read-only"}')" == fixed ]] \
  || fail "a service that refuses this run's writes must choose the fixed split"
unset -f curl sleep

# The action that hands the results service to the shard's `run:` steps.
node --test "$root_dir/.github/actions/results-service-env/index.test.cjs" > "$work/node-test.log" 2>&1 \
  || fail "results-service-env: $(cat "$work/node-test.log")"

if (( fails > 0 )); then
  echo "test-shardqueue: $fails failure(s)" >&2
  exit 1
fi
echo "OK: test-shardqueue — shards claim every group once, a fast one claims more, and a later attempt skips what passed"
