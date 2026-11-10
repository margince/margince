#!/usr/bin/env bash
# Shared helper for the parallel integration lane: find the tests the lane runs,
# decide which packages the dispatcher starts first, and which tests each CI
# shard runs. Source this; don't execute it.
#
# Wall clock is set by when the LONGEST package finishes, so it should start at
# t=0. Dispatched in discovery order it does not: measured, this lane's long
# pole waited 16s of a 252s run for a slot while shorter packages held all
# eight. That is the classic longest-processing-time-first result.
#
# The durations are read as an ORDER, never as a prediction. They are wall-clock
# seconds from one machine under one load, so the numbers themselves carry
# little across hosts; what survives the move is which package is the long pole.
#
# Lives in its own file so `scripts/test-laneorder.sh` can exercise the two
# properties that decide whether the ordering helps or quietly hurts — a named
# package sorts by its duration, and an unnamed one keeps the order it would
# have had with no hint at all — without standing up a database.

# resolve_order_hint MEASURED BASELINE — print the hint file to dispatch by, or
# nothing when neither is usable.
#
# MEASURED is what the last full run on THIS machine recorded and wins whenever
# it exists: a machine's own durations beat anybody else's. BASELINE is the
# committed fallback, so a fresh clone — which has no .tmp/ at all — still
# dispatches longest-first on its very first lane. The committed file is never
# written by a run; see the publish step in test-integration-parallel.sh for why
# a shard's slice measurements must not become anyone's baseline.
resolve_order_hint() {
  local measured="$1" baseline="$2"
  if [[ -s "$measured" ]]; then
    printf '%s' "$measured"
  elif [[ -s "$baseline" ]]; then
    printf '%s' "$baseline"
  fi
}

# order_by_hint GROUPED HINT — print GROUPED's lines longest-package-first.
#
# GROUPED lines are `moduledir|package|run-regex`; HINT lines are
# `package|seconds`. Only fields 1 and 2 of each are read, and the GROUPED line
# is reprinted whole, because its third field is a -run regex containing its own
# pipes.
#
# A HINT line naming something that is not a package in GROUPED is simply never
# looked up, which is what makes the committed file's human-readable header
# harmless: it becomes an entry keyed on its own text that nothing asks for.
#
# A package with no USABLE time sorts FIRST, not last. The unknown is usually a
# new small package, where the cost of guessing wrong is one slot occupied early;
# guessing wrong the other way puts a new heavy package last and pays its whole
# duration after everything else has finished.
#
# "Usable" and not merely "present", because awk reads a non-numeric field as
# ZERO — which sorts that package LAST, the exact schedule this ordering exists
# to avoid, and it is reachable: the lane's own report has a branch for a package
# that "recorded no test duration", and such a line reaches the hint as
# `package|` with an empty second field. A duration that is not a positive number
# is therefore treated as no duration at all.
#
# -s (stable) is what makes an unnamed package degrade to the order it would
# have had with no hint at all. Every unnamed package shares one sort key, and
# without -s sort breaks those ties on the whole line — under -r, in REVERSE
# input order. A stale baseline must hand the packages it does not name back to
# the order they arrived in, never to an order the sort invented from them.
order_by_hint() {
  local grouped="$1" hint="$2"
  awk -F'|' -v hint="$hint" '
    BEGIN {
      while ((getline l < hint) > 0) {
        split(l, f, "|")
        if (f[2] ~ /^[0-9]+(\.[0-9]+)?$/ && f[2] + 0 > 0) secs[f[1]] = f[2] + 0
      }
    }
    { printf "%012.3f|%s\n", ($2 in secs ? secs[$2] : 999999), $0 }
  ' "$grouped" | LC_ALL=C sort -s -t'|' -k1,1 -rn | cut -d'|' -f2-
}

# The one weighing rule, for the split and for the order alike. TIMING lines are
# `moduledir|package|Test|seconds` and name the slow tests one by one; a
# `dir|package|*|seconds` line gives the weight of every test of that package it
# does not name. A test with neither weighs 1 s, so a stale file costs balance
# and never a test. With no timing at all every weight is equal.
#
# No weight is under 0.01 s, go test's own resolution. A zero never adds to a
# bin's load, so every zero-weight test would be dealt to the same first bin.
LANEORDER_WEIGHT='
  function load_weights(timing,    l, f, n) {
    while (timing != "" && (getline l < timing) > 0) {
      n = split(l, f, "|")
      if (n == 4 && f[4] ~ /^[0-9]+(\.[0-9]+)?$/) secs[f[1] "|" f[2] "|" f[3]] = (f[4] + 0 < 0.01) ? 0.01 : f[4] + 0
    }
  }
  function weight(dir, pkg, test,    rest) {
    if ((dir "|" pkg "|" test) in secs) return secs[dir "|" pkg "|" test]
    rest = dir "|" pkg "|*"
    return (rest in secs) ? secs[rest] : 1
  }
  # The weight of the current `moduledir|package|^(A|B)$` line: its tests summed.
  function part_weight(    i, test, total) {
    for (i = 3; i <= NF; i++) {
      test = $i
      sub(/^\^\(/, "", test)
      sub(/\)\$$/, "", test)
      total += weight($1, $2, test)
    }
    return total
  }'

# weigh_tests DISCOVERY TIMING — print `moduledir|package|weight|<line>` for each
# DISCOVERY line (`moduledir|package|Test`), each package's tests slowest first.
weigh_tests() {
  local discovery="$1" timing="$2"
  awk -F'|' -v timing="$timing" "$LANEORDER_WEIGHT"'
    BEGIN { load_weights(timing) }
    { printf "%s|%s|%012.3f|%s\n", $1, $2, weight($1, $2, $3), $0 }
  ' "$discovery" | LC_ALL=C sort -t'|' -k1,2 -k3,3r -k4
}

# order_by_weight GROUPED TIMING — print GROUPED's lines heaviest part first,
# each part weighing the sum of its tests. Ties keep their input order.
#
# A shard orders by the file it split by rather than by the package hint above:
# the parts of one package share that hint's one number, so a heavy part of a
# package the hint ranks low waited for a slot behind lighter ones.
order_by_weight() {
  local grouped="$1" timing="$2"
  awk -F'|' -v timing="$timing" "$LANEORDER_WEIGHT"'
    BEGIN { load_weights(timing) }
    { printf "%012.3f|%s\n", part_weight(), $0 }
  ' "$grouped" | LC_ALL=C sort -s -t'|' -k1,1 -rn | cut -d'|' -f2-
}

# queue_groups PARTS TIMING SECONDS — print the groups a shard claims from the
# shared queue, one line of part numbers each. PARTS is order_by_weight output,
# and part n is its line n.
#
# A group is consecutive parts until it weighs SECONDS, so a heavy part is a
# group by itself and the light tail packs several. One claim takes one group,
# so SECONDS bounds how often the shards ask the queue.
queue_groups() {
  local parts="$1" timing="$2" seconds="$3"
  awk -F'|' -v timing="$timing" -v target="$seconds" "$LANEORDER_WEIGHT"'
    BEGIN { load_weights(timing) }
    {
      load += part_weight()
      group = (group == "") ? NR : group " " NR
      if (load >= target) { print group; group = ""; load = 0 }
    }
    END { if (group != "") print group }
  ' "$parts"
}

# The one dealing rule, for shards and for parts alike: a test goes to the bin
# holding the least of its package's time so far, ties to the bin with the least
# time overall, then to the lowest index. Fed slowest first, that is
# longest-processing-time-first.
LANEORDER_DEAL='
  function deal(p, weight, bins,    s, best) {
    best = 1
    for (s = 2; s <= bins; s++) {
      if (load[p, s] < load[p, best] || (load[p, s] == load[p, best] && sum[s] < sum[best])) best = s
    }
    load[p, best] += weight
    sum[best] += weight
    return best
  }'

# assign_shard DISCOVERY TIMING K N — print the DISCOVERY lines shard K of N runs.
#
# Tests of one package run one after another, so a shard's wall clock is set by
# its largest package slice. Each package is therefore split on its own.
#
# Every shard computes the same split from the same two files, and each line
# goes to exactly one shard; the lane's fan-in checks that the union is whole.
assign_shard() {
  local discovery="$1" timing="$2" shard="$3" total="$4"
  weigh_tests "$discovery" "$timing" \
    | awk -F'|' -v shard="$shard" -v total="$total" "$LANEORDER_DEAL"'
        { if (deal($1 "|" $2, $3 + 0, total) == shard) print $4 "|" $5 "|" $6 }
      ' \
    | LC_ALL=C sort
}

# split_parts ASSIGNED TIMING SECONDS — print one `moduledir|package|-run regex`
# line for each `go test` process the lane starts.
#
# go test's -timeout bounds one process, so a package slice weighing more than
# SECONDS is dealt into as many parts as it takes to bring each part under it,
# and each part runs as its own process on its own clone. A package at or under
# SECONDS stays one process. No part is ever empty, and every ASSIGNED line is
# in exactly one part.
split_parts() {
  local assigned="$1" timing="$2" seconds="$3"
  weigh_tests "$assigned" "$timing" \
    | awk -F'|' -v target="$seconds" "$LANEORDER_DEAL"'
        # Deals the package into bins parts; answers whether every part of
        # more than one test fits under target.
        function dealt(bins,    i, b, s) {
          for (s = 1; s <= bins; s++) { sum[s] -= load[pkg, s]; load[pkg, s] = 0; count[s] = 0 }
          for (b in part) delete part[b]
          for (i = 1; i <= n; i++) {
            b = deal(pkg, weight[i], bins)
            # Counted, never `b in part`: mawk creates part[b] for the
            # assignment before it tests membership.
            part[b] = count[b]++ ? part[b] "|" name[i] : name[i]
          }
          for (s = 1; s <= bins; s++) if (count[s] > 1 && load[pkg, s] > target) return 0
          return 1
        }
        # The least parts that bring each one under target. Dealing is not
        # packing: 40, 30 and 30 s deal into two parts as 40 and 60, so a part
        # over target asks for one more, up to a part per test.
        function flush(    bins, b) {
          if (n == 0) return
          bins = (target > 0) ? int(total / target) : 1
          if (target > 0 && bins * target < total) bins++
          if (bins < 1) bins = 1
          if (bins > n) bins = n
          while (!dealt(bins) && target > 0 && bins < n) bins++
          for (b = 1; b <= bins; b++) if (count[b]) print pkg "|^(" part[b] ")$"
          n = 0
          total = 0
        }
        ($1 "|" $2) != pkg { flush(); pkg = $1 "|" $2 }
        { n++; weight[n] = $3 + 0; name[n] = $6; total += weight[n] }
        END { flush() }
      ' \
    | LC_ALL=C sort
}

# critical_part WALLCLOCK TIMING — print `package|began|ended|test seconds` for
# the process that ended last, the seconds empty when its log recorded none.
#
# WALLCLOCK lines are `package|began|ended|slot`, TIMING lines
# `package|seconds|tests|slot`. They are joined on the slot, never on the
# package: the parts of a split package share its name, and another part's
# seconds would price this part's provisioning wrong, even below zero.
critical_part() {
  local wallclock="$1" timing="$2"
  LC_ALL=C sort -t'|' -k3,3 -rn "$wallclock" | awk -F'|' -v timing="$timing" '
    NR == 1 {
      while ((getline line < timing) > 0) {
        split(line, f, "|")
        if (f[4] == $4) tests = f[2]
      }
      print $1 "|" $2 "|" $3 "|" tests
    }
  '
}

# lane_discover UNTAGGED MODULE_DIR... — print every top-level Test function the
# integration lane runs, as sorted `moduledir|package|Test` lines, and write
# `moduledir|package|count` to UNTAGGED for each untagged test file the lane
# leaves to `make check`.
#
# Discovery is static, so it costs no compile, and it must agree with the
# compiler under `-tags=integration`. A test it misses is in no slice, and the
# ran==assigned teeth compare two sets derived from it, so where it cannot
# decide it fails rather than guesses:
#   - Files go reads as it reads them: none under testdata, vendor or a
#     directory starting with `_` or `.`, and no file starting with either.
#   - A file's constraints are its `//go:build` lines above the `package`
#     clause, or its `// +build` lines when it has no `//go:build` line.
#   - A package is in the lane when a test file's constraint names
#     `integration`. Only those packages are held to the allowlist below; a
#     package the lane never runs may use any tag.
#   - `integration` tags a file. `!integration`, the opt-in lanes (e2e_llm,
#     livesmoke, voicelive) and the `integration && bench|rolesweep` lanes are
#     never in this build, so their files are left out as the compiler leaves
#     them out. Any other constraint fails, `(integration)` included: a
#     built-in tag such as linux compiles under `go test` yet would land in no
#     slice. A tagged file named for a GOOS or GOARCH fails for the same reason.
#   - A tagged file with a Fuzz or Example function fails, because the anchored
#     -run union would filter it out where no check sees it, and so does a Test
#     name outside ASCII, which this scan cannot class as go does.
#   - TestMain(*testing.M) is a fixture, and Test followed by a lowercase letter
#     is a plain function, as `go test` reads them.
#
# A declaration-shaped line inside a raw string or a block comment reads as a
# test. That fails loudly in the ran==assigned check, never quietly.
#
# One awk process reads every test file. A shell loop that forked a sed and a
# grep per file cost each build and each shard about 40 s.
lane_discover() { # untagged module-dir...
  local untagged="$1" dir
  shift
  for dir in "$@"; do
    [[ -d "$dir" ]] || continue
    find "$dir" -mindepth 1 \( -name testdata -o -name vendor -o -name '_*' -o -name '.*' \) -prune \
      -o -type f -name '*_test.go' -print | awk -v dir="$dir" '{ print dir "\t" $0 }'
  done | LC_ALL=C sort | awk -F'\t' -v untagged="$untagged" '
    function fail(msg) { print "FAIL: " msg > "/dev/stderr"; exit 1 }
    # go/build: the part after the first `_`, less `_test`, ends in _GOOS,
    # _GOARCH or _GOOS_GOARCH.
    function named_for_platform(file,    base, at, parts, count) {
      base = file
      sub(/^.*\//, "", base)
      sub(/\.go$/, "", base)
      at = index(base, "_")
      if (!at) return 0
      base = substr(base, at)
      sub(/_test$/, "", base)
      count = split(base, parts, "_")
      if (count >= 3 && (parts[count - 1] in goos) && (parts[count] in goarch)) return 1
      return (parts[count] in goos) || (parts[count] in goarch)
    }
    BEGIN {
      split("aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos", list, " ")
      for (i in list) goos[list[i]] = 1
      split("386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm", list, " ")
      for (i in list) goarch[list[i]] = 1
    }
    {
      n++
      path[n] = $2
      pkgdir = $2
      sub(/\/[^\/]*$/, "", pkgdir)
      key[n] = $1 "|" ((pkgdir == $1) ? "." : "./" substr(pkgdir, length($1) + 2))
      region = 1; modern = 0; legacy = 0; funcs = 0; names = ""; fuzz[n] = 0; odd[n] = ""
      while ((getline line < $2) > 0) {
        if (region && line ~ /^package /) region = 0
        else if (region && line ~ /^\/\/go:build /) expr[n, ++modern] = substr(line, 12)
        else if (region && line ~ /^\/\/ \+build /) old[++legacy] = substr(line, 11)
        if (line ~ /^func Test[A-Za-z0-9_]*\(/) funcs++
        if (line ~ /^func Test/) {
          name = substr(line, 6)
          sub(/\(.*/, "", name)
          if (name ~ /[^A-Za-z0-9_]/) odd[n] = name
          else if (name == "TestMain" && line !~ /^func TestMain\([^)]*\*testing\.T\)/) continue
          else if (substr(name, 5, 1) !~ /[a-z]/) names = names "|" name
        }
        if (line ~ /^func (Fuzz|Example)[A-Za-z0-9_]*\(/) fuzz[n] = 1
      }
      close($2)
      # go ignores `// +build` once a file has a `//go:build` line.
      if (!modern) for (i = 1; i <= legacy; i++) expr[n, ++modern] = old[i]
      nexpr[n] = modern; nfuncs[n] = funcs; tests[n] = names
      for (i = 1; i <= modern; i++) {
        if (expr[n, i] ~ /(^|[^A-Za-z0-9_.])integration([^A-Za-z0-9_.]|$)/) lane[key[n]] = 1
      }
    }
    END {
      for (i = 1; i <= n; i++) {
        if (!(key[i] in lane)) continue
        tagged = 0; skip = 0
        for (j = 1; j <= nexpr[i]; j++) {
          e = expr[i, j]
          if (e == "") continue
          if (e == "integration") tagged = 1
          else if (e == "!integration" || e == "e2e_llm" || e == "livesmoke" || e == "voicelive" \
            || e == "integration && bench" || e == "integration && rolesweep") skip = 1
          else fail(path[i] " carries build constraint \047" e "\047, not one the static shard discovery knows\n" \
            "  teach the allowlist in scripts/lib-laneorder.sh or simplify the constraint")
        }
        if (skip) continue
        if (!tagged) { print key[i] "|" nfuncs[i] > untagged; continue }
        if (named_for_platform(path[i])) fail(path[i] " is named for a GOOS or GOARCH, which the static shard discovery cannot decide\n" \
          "  rename it, or move what it holds into a file every platform builds")
        if (fuzz[i]) fail(path[i] " declares a Fuzz or Example function in the integration lane\n" \
          "  the anchored -run union would filter it out and the ran==assigned teeth cannot see it;\n" \
          "  teach discovery to enumerate it, or move it to the unit lane")
        if (odd[i] != "") fail(path[i] " declares " odd[i] ", a test name outside ASCII letters, digits and _\n" \
          "  the static shard discovery cannot tell whether go test runs it; rename it")
        count = split(tests[i], found, "|")
        for (j = 2; j <= count; j++) print key[i] "|" found[j]
      }
    }
  ' | LC_ALL=C sort
}
