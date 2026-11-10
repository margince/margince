#!/usr/bin/env bash
# The shared work queue of the CI integration shards. Source this; don't execute it.
#
# The shards run on runners whose speed differs by up to about 1.9x, so a split
# fixed before the run leaves the fast ones idle while the slow ones finish.
# Here every shard takes the next group of parts when a slot frees, so a fast
# runner takes more groups and a slow one fewer.
#
# A claim is a reservation that only one caller can make. INTEGRATION_QUEUE
# names the store:
#   cache:PREFIX  the GitHub Actions cache service. Creating a cache entry for a
#                 key that already has one fails, and that holds for every job
#                 of the run. Needs ACTIONS_RUNTIME_TOKEN and ACTIONS_RESULTS_URL.
#   dir:PATH      a directory on this machine: `mkdir` either makes the claim
#                 directory or finds it there. For tests and for shards started
#                 side by side on one machine.
#
# Each group has a home shard (group g belongs to shard ((g - 1) mod N) + 1).
# A shard claims its own groups from the front, heaviest first. With none left,
# it takes the groups of other shards from the back, lightest first. So the
# groups a shard owns that are claimed are always a run from the front plus a
# run from the back, and the one gap between them closes from both ends:
#   - the owner stops at its first lost claim, because past it every group is
#     already taken;
#   - a thief stops taking from a shard at its first lost claim. The thief that
#     won that group goes on down, and so does the owner from the front, so
#     the gap is never left without a claimer. A shard that never starts loses
#     none of its groups.
# The fan-in still fails the run when two shards report one test.

# queue_open SPEC SHARD TOTAL GROUPS DONE — set up this shard's view of the
# queue. GROUPS is queue_groups output; DONE lists group numbers an earlier
# attempt of this run already finished, which nobody claims again.
queue_open() {
  local spec="$1" done_file="$5" g
  QUEUE_SHARD="$2" QUEUE_TOTAL="$3"
  QUEUE_GROUP_COUNT=$(wc -l < "$4" | tr -d ' ')
  case "$spec" in
    cache:?*) QUEUE_BACKEND=cache QUEUE_PREFIX="${spec#cache:}" ;;
    dir:?*) QUEUE_BACKEND=dir QUEUE_PREFIX="${spec#dir:}"; mkdir -p "$QUEUE_PREFIX" || return 1 ;;
    *) echo "FAIL: INTEGRATION_QUEUE must be cache:PREFIX or dir:PATH, got '$spec'" >&2; return 1 ;;
  esac
  QUEUE_DONE=()
  if [[ -n "$done_file" ]]; then
    while read -r g; do QUEUE_DONE[g]=1; done < "$done_file"
  fi
  # The next own group to claim, and for each other shard the next group to
  # take from the back, 0 once there is none.
  QUEUE_OWN="$QUEUE_SHARD"
  QUEUE_BACK=()
  for ((g = 1; g <= QUEUE_TOTAL; g++)); do
    QUEUE_BACK[g]=0
    (( g > QUEUE_GROUP_COUNT )) || QUEUE_BACK[g]=$(( g + (QUEUE_GROUP_COUNT - g) / QUEUE_TOTAL * QUEUE_TOTAL ))
  done
}

# queue_done_groups DIR SHARD DIGEST — print the groups that the shard
# artifacts under DIR, from an earlier attempt of this run, report as passed.
# Only a green shard writes meta.txt. The shard's own earlier artifact is left
# out, because a shard runs again only when it failed or someone asked it to.
queue_done_groups() {
  local dir="$1" shard="$2" digest="$3" meta
  for meta in "$dir"/integration-shard-*/meta.txt; do
    [[ -e "$meta" ]] || continue
    [[ "$(sed -n 's/^shard=//p' "$meta")" != "$shard" ]] || continue
    if ! grep -qx "queue=$digest" "$meta"; then
      echo "FAIL: $meta numbers other parts than this attempt (no queue=$digest), so its groups cannot be skipped" >&2
      return 1
    fi
    cut -d' ' -f1 "$(dirname "$meta")/claimed.txt"
  done | LC_ALL=C sort -n -u
}

# queue_next — set QUEUE_GROUP to the next group this shard claimed, or to
# nothing when no group is left. Returns non-zero when the store cannot be
# reached; groups this shard did not claim may then be left over.
queue_next() {
  local g victim step st
  QUEUE_GROUP=""
  while (( QUEUE_OWN <= QUEUE_GROUP_COUNT )); do
    g=$QUEUE_OWN
    QUEUE_OWN=$(( QUEUE_OWN + QUEUE_TOTAL ))
    [[ -z "${QUEUE_DONE[g]:-}" ]] || continue
    queue_take "$g" && { QUEUE_GROUP=$g; return 0; }
    st=$?
    (( st == 1 )) || return "$st"
    QUEUE_OWN=$(( QUEUE_GROUP_COUNT + 1 ))
  done
  for ((step = 1; step < QUEUE_TOTAL; step++)); do
    victim=$(( (QUEUE_SHARD - 1 + step) % QUEUE_TOTAL + 1 ))
    while (( QUEUE_BACK[victim] >= 1 )); do
      g=${QUEUE_BACK[victim]}
      QUEUE_BACK[victim]=$(( g - QUEUE_TOTAL ))
      [[ -z "${QUEUE_DONE[g]:-}" ]] || continue
      queue_take "$g" && { QUEUE_GROUP=$g; return 0; }
      st=$?
      (( st == 1 )) || return "$st"
      QUEUE_BACK[victim]=0
    done
  done
}

# queue_take GROUP — 0 when this call claimed GROUP, 1 when another claim holds
# it, 2 when the store did not answer within QUEUE_RETRY_SECONDS (default 300),
# 3 when it refuses writes from this run.
queue_take() {
  if [[ "$QUEUE_BACKEND" == cache ]]; then
    queue_take_cache "${QUEUE_PREFIX}-g$1"
    return
  fi
  mkdir "$QUEUE_PREFIX/g$1" 2>/dev/null && return 0
  [[ -d "$QUEUE_PREFIX/g$1" ]] && return 1
  echo "FAIL: cannot claim group $1: $QUEUE_PREFIX is not writable" >&2
  return 2
}

# The version is part of a cache entry's identity, so every claim of a key has
# to send the same one: the SHA-256 of `margince-shard-queue-1`.
QUEUE_CACHE_VERSION=e5966bee5afc6b6c3437111d9c5af6cc342d6115969a7495e8a3b7a0c487f4f6

queue_take_cache() {
  local key="$1" body code wait_for spent=0 budget="${QUEUE_RETRY_SECONDS:-300}" reply
  body="$(printf '{"key":"%s","version":"%s"}' "$key" "$QUEUE_CACHE_VERSION")"
  reply="$(mktemp)"
  while :; do
    code=$(curl -sS -o "$reply" -w '%{http_code}' -X POST --max-time 30 \
      -H "Authorization: Bearer ${ACTIONS_RUNTIME_TOKEN:?}" -H 'Content-Type: application/json' \
      --data "$body" "${ACTIONS_RESULTS_URL%/}/twirp/github.actions.results.api.v1.CacheService/CreateCacheEntry" \
      2>/dev/null) || code=000
    case "$code" in
      200)
        if jq -e '.ok == true' "$reply" > /dev/null; then rm -f "$reply"; return 0; fi
        if jq -e '(.message // "") | startswith("cache write denied")' "$reply" > /dev/null; then
          rm -f "$reply"
          return 3
        fi
        rm -f "$reply"
        return 1
        ;;
      409) rm -f "$reply"; return 1 ;;
      403) rm -f "$reply"; return 3 ;;
    esac
    # 429, a 5xx or no answer: the service is busy or away, never a verdict.
    wait_for=$(( spent < 10 ? 2 : 15 ))
    if (( spent + wait_for > budget )); then
      echo "FAIL: the cache service answered $code to every claim of $key for ${spent}s: $(head -c 200 "$reply")" >&2
      rm -f "$reply"
      return 2
    fi
    sleep "$wait_for"
    spent=$(( spent + wait_for ))
  done
}

# run_queue_packages PARTS GROUPS JOBS OUTDIR CLAIMED — run the parts of every
# group this shard claims, up to JOBS at once, and append `group part...` to
# CLAIMED for each claim. Part n is line n of PARTS and runs as
# `run_one LINE n OUTDIR SLOT`. The slot is the part's Redis database, so no two
# parts that run at once share one.
#
# A claim is made only when a slot is free and every claimed part has started,
# so a shard never holds a group it cannot start. Returns non-zero when the
# queue store failed; the parts already claimed still run.
run_queue_packages() {
  local parts="$1" groups="$2" jobs="$3" outdir="$4" claimed="$5"
  local -a backlog=() free=() members=()
  local -A slot_of=()
  local slot part pid claiming=1 status=0
  for ((slot = 1; slot <= jobs; slot++)); do free+=("$slot"); done
  while :; do
    while (( ${#free[@]} > 0 )); do
      if (( ${#backlog[@]} == 0 && claiming )); then
        if ! queue_next; then
          status=1 claiming=0
        elif [[ -z "$QUEUE_GROUP" ]]; then
          claiming=0
        else
          read -r -a members <<< "$(sed -n "${QUEUE_GROUP}p" "$groups")"
          backlog+=("${members[@]}")
          printf '%s %s\n' "$QUEUE_GROUP" "${members[*]}" >> "$claimed"
          echo "test-integration-parallel: claimed group $QUEUE_GROUP (parts ${members[*]})"
        fi
      fi
      (( ${#backlog[@]} > 0 )) || break
      part=${backlog[0]} slot=${free[0]}
      backlog=("${backlog[@]:1}") free=("${free[@]:1}")
      # Without errexit, as xargs runs it for the fixed split.
      ( set +e; run_one "$(sed -n "${part}p" "$parts")" "$part" "$outdir" "$slot" ) &
      slot_of[$!]=$slot
    done
    (( ${#slot_of[@]} > 0 )) || break
    # The status is not the part's verdict; that is the EXIT line in its log.
    # Every part that has ended frees its slot, not only the one wait names:
    # bash may already have reaped one that ended while a claim was made.
    wait -n || :
    for pid in "${!slot_of[@]}"; do
      kill -0 "$pid" 2>/dev/null && continue
      free+=("${slot_of[$pid]}")
      unset "slot_of[$pid]"
    done
  done
  return "$status"
}
