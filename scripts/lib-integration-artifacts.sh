#!/usr/bin/env bash
# What a shard of the parallel integration lane leaves for the fan-in and for a
# continuation, part by part. Source this; don't execute it.
#
# A heavy package runs as several parts, each its own process with its own log.
# Named by package alone, two failing parts of one package wrote one file, and
# the artifact kept whichever failure was copied last. So each part keeps its
# own log and its own lists: the tests it was given, the tests that reported,
# and the tests a continuation still owes (any that did not pass).
#
# scripts/test-integration-artifacts.sh tests this file.

# part_tests REGEX_FILE — print the test names of one part's -run selector,
# `^(TestA|TestB)$`, one per line. An empty selector names no part.
part_tests() {
  local selector
  selector="$(cat "$1")" || return 1
  [[ -n "$selector" ]] || return 0
  selector="${selector#^(}"
  selector="${selector%)\$}"
  tr '|' '\n' <<< "$selector"
}

# part_names WORK REGEX_DIR — print `slot|name` for every part:
# `<moduledir>@<package>`, and `@part<k>of<n>` after it for a split package.
# Each component escapes `%` and `/`, and a package drops its leading `./`. A Go
# import path holds neither `@` nor `%`, so no two parts can share a name. A
# split package ranks its parts by their first test, so a name does not move
# when the lane dispatches the parts in another order.
part_names() {
  local work="$1" regex_dir="$2" slot=0 line first
  while IFS= read -r line; do
    slot=$((slot + 1))
    first="$(part_tests "$regex_dir/$slot" | LC_ALL=C sort | head -n1)" || return 1
    printf '%s|%s|%s\n' "$line" "$first" "$slot"
  done < "$work" \
    | LC_ALL=C sort -t'|' -k1,1 -k2,2 -k3,3 \
    | awk -F'|' '
        function escaped(component) {
          sub(/^\.\//, "", component)
          if (component == ".") return "%2E"
          gsub(/%/, "%25", component)
          gsub(/\//, "%2F", component)
          return component
        }
        { key[NR] = $1 "|" $2; dir[NR] = $1; pkg[NR] = $2; slot[NR] = $4; parts[$1 "|" $2]++ }
        END {
          for (i = 1; i <= NR; i++) {
            rank[key[i]]++
            name = escaped(dir[i]) "@" escaped(pkg[i])
            if (parts[key[i]] > 1) name = name "@part" rank[key[i]] "of" parts[key[i]]
            print slot[i] "|" name
          }
        }' \
    | LC_ALL=C sort -t'|' -k1,1n
}

# keep_shard_evidence OUT WORK REGEX_DIR LOG_DIR FAILED [SLOT...] — write each
# part's lists under OUT/parts, copy each failed part's log to OUT/failed-logs
# under its part name, and gather OUT/rerun.txt: every test a continuation owes.
# FAILED holds `dir|package|slot.log` for each part that did not exit 0. With
# SLOTs, only those parts: a queue shard's WORK holds the whole lane, so the
# names stay those of the whole lane while the lists cover what it claimed.
#
# Every step returns its own failure. A caller guards this with `||` so the
# lane still prints its verdict, and bash turns errexit off inside a function
# called that way, so a failed step would otherwise run on into partial lists.
keep_shard_evidence() {
  local out="$1" work="$2" regex_dir="$3" log_dir="$4" failed="$5"
  local slot name line dir pkg log prefix names
  local -A only=()
  shift 5
  for slot in "$@"; do only[$slot]=1; done
  mkdir -p "$out/parts" || return 1
  : > "$out/rerun.txt" || return 1
  names="$(part_names "$work" "$regex_dir")" || return 1
  while IFS='|' read -r slot name; do
    [[ -n "$slot" ]] || continue
    (( $# == 0 )) || [[ -n "${only[$slot]:-}" ]] || continue
    line="$(sed -n "${slot}p" "$work")" || return 1
    dir="${line%%|*}" pkg="${line#*|}" prefix="$out/parts/$name"
    part_tests "$regex_dir/$slot" | awk -v p="$dir|$pkg|" '{ print p $0 }' | LC_ALL=C sort > "$prefix.assigned.txt" || return 1
    log="$log_dir/$slot.log"
    : > "$prefix.ran.txt" || return 1
    : > "$prefix.passed.txt" || return 1
    if [[ -f "$log" ]]; then
      awk -v p="$dir|$pkg|" '/^--- (PASS|FAIL|SKIP): / { print p $3 }' "$log" | LC_ALL=C sort > "$prefix.ran.txt" || return 1
      awk -v p="$dir|$pkg|" '/^--- PASS: / { print p $3 }' "$log" | LC_ALL=C sort > "$prefix.passed.txt" || return 1
    fi
    # Owed: given to this part and not passed, so a failure and a test the
    # part never reached (killed, timed out, crashed) are both run again.
    LC_ALL=C comm -23 "$prefix.assigned.txt" "$prefix.passed.txt" > "$prefix.rerun.txt" || return 1
    rm -f "$prefix.passed.txt"
    cat "$prefix.rerun.txt" >> "$out/rerun.txt" || return 1
    if grep -qx "$dir|$pkg|$slot.log" "$failed"; then
      mkdir -p "$out/failed-logs" || return 1
      cp "$log" "$out/failed-logs/$name.log" || return 1
    fi
  done <<< "$names"
  LC_ALL=C sort -o "$out/rerun.txt" "$out/rerun.txt"
}

# reconcile_parts SHARD_DIR — fail unless the shard's parts together hold its
# assigned tests exactly once each, every part with all of its lists. A shard
# dealt no tests has no parts, and that reconciles.
reconcile_parts() {
  local shard="$1" assigned union
  if ! compgen -G "$shard/parts/*.assigned.txt" > /dev/null; then
    [[ ! -s "$shard/assigned.txt" ]] && return 0
    echo "FAIL: $shard was assigned tests but has no part lists"
    return 1
  fi
  for assigned in "$shard"/parts/*.assigned.txt; do
    for list in ran rerun; do
      [[ -f "${assigned%.assigned.txt}.$list.txt" ]] || {
        echo "FAIL: ${assigned##*/} has no ${list}.txt beside it"
        return 1
      }
    done
  done
  union="$(mktemp)"
  LC_ALL=C sort "$shard"/parts/*.assigned.txt > "$union"
  if ! diff "$shard/assigned.txt" "$union" > /dev/null; then
    echo "FAIL: the parts of $shard do not hold its assigned tests exactly once:"
    diff "$shard/assigned.txt" "$union" | sed -n 's/^< /  in no part: /p; s/^> /  extra part entry: /p' || true
    rm -f "$union"
    return 1
  fi
  rm -f "$union"
}
