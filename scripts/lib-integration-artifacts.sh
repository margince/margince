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
  selector="$(cat "$1")"
  [[ -n "$selector" ]] || return 0
  selector="${selector#^(}"
  selector="${selector%)\$}"
  tr '|' '\n' <<< "$selector"
}

# part_names WORK REGEX_DIR — print `slot|name` for every part. A package run
# whole keeps its flattened path, the name its log always had. A split package
# names each part by its rank among them, ordered by its first test, so the
# name does not move when the lane dispatches the parts in another order.
part_names() {
  local work="$1" regex_dir="$2" slot=0 line
  while IFS= read -r line; do
    slot=$((slot + 1))
    printf '%s|%s|%s\n' "$line" "$(part_tests "$regex_dir/$slot" | LC_ALL=C sort | head -n1)" "$slot"
  done < "$work" \
    | LC_ALL=C sort -t'|' -k1,1 -k2,2 -k3,3 \
    | awk -F'|' '
        { key[NR] = $1 "|" $2; pkg[NR] = $2; slot[NR] = $4; parts[$1 "|" $2]++ }
        END {
          for (i = 1; i <= NR; i++) {
            flat = pkg[i]; gsub(/[\/.]/, "_", flat)
            rank[key[i]]++
            name = parts[key[i]] > 1 ? flat "--part" rank[key[i]] "of" parts[key[i]] : flat
            print slot[i] "|" name
          }
        }' \
    | LC_ALL=C sort -t'|' -k1,1n
}

# keep_shard_evidence OUT WORK REGEX_DIR LOG_DIR FAILED — write each part's
# lists under OUT/parts, copy each failed part's log to OUT/failed-logs under
# its part name, and gather OUT/rerun.txt: every test a continuation owes.
# FAILED holds `dir|package|slot.log` for each part that did not exit 0.
keep_shard_evidence() {
  local out="$1" work="$2" regex_dir="$3" log_dir="$4" failed="$5"
  local slot name line dir pkg log prefix
  mkdir -p "$out/parts"
  : > "$out/rerun.txt"
  while IFS='|' read -r slot name; do
    line="$(sed -n "${slot}p" "$work")"
    dir="${line%%|*}" pkg="${line#*|}" prefix="$out/parts/$name"
    part_tests "$regex_dir/$slot" | awk -v p="$dir|$pkg|" '{ print p $0 }' | LC_ALL=C sort > "$prefix.assigned.txt"
    log="$log_dir/$slot.log"
    : > "$prefix.ran.txt"
    : > "$prefix.passed.txt"
    if [[ -f "$log" ]]; then
      awk -v p="$dir|$pkg|" '/^--- (PASS|FAIL|SKIP): / { print p $3 }' "$log" | LC_ALL=C sort > "$prefix.ran.txt"
      awk -v p="$dir|$pkg|" '/^--- PASS: / { print p $3 }' "$log" | LC_ALL=C sort > "$prefix.passed.txt"
    fi
    # Owed: given to this part and not passed, so a failure and a test the
    # part never reached (killed, timed out, crashed) are both run again.
    LC_ALL=C comm -23 "$prefix.assigned.txt" "$prefix.passed.txt" > "$prefix.rerun.txt"
    rm -f "$prefix.passed.txt"
    cat "$prefix.rerun.txt" >> "$out/rerun.txt"
    if grep -qx "$dir|$pkg|$slot.log" "$failed"; then
      mkdir -p "$out/failed-logs"
      cp "$log" "$out/failed-logs/$name.log"
    fi
  done < <(part_names "$work" "$regex_dir")
  LC_ALL=C sort -o "$out/rerun.txt" "$out/rerun.txt"
}

# reconcile_parts SHARD_DIR — fail unless the shard's parts together hold its
# assigned tests exactly once each, every part with all of its lists.
reconcile_parts() {
  local shard="$1" assigned union
  compgen -G "$shard/parts/*.assigned.txt" > /dev/null || {
    echo "FAIL: $shard has no part lists"
    return 1
  }
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
