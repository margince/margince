#!/usr/bin/env bash
# Parallel integration-test runner.
#
# The serial lane runs every integration package with `go test -p 1` against ONE
# shared database, because parallel packages racing on the same DB collide on the
# schema each test rebuilds. That serialization is the only reason for -p 1 — the
# suites are I/O-bound (mostly idle on Postgres round-trips and TLS handshakes in
# the HTTP e2e), so the CPU sits unused while one package runs.
#
# This runner removes the shared-state constraint instead of the serialization.
# Each package gets private throwaway state, so concurrent packages share nothing:
#   Postgres — its own empty clone db (the Go harness migrates it once, then
#              resets the data between tests — see internal/platform/testdb).
#   MinIO    — a private bucket (MARGINCE_TEST_BLOBSTORE_BUCKET=<base>-p<idx>),
#              auto-created by the blobstore store.
#   Redis    — the one shared instance, but each package gets its own logical
#              db (MARGINCE_TEST_REDIS_DB, mapped 1..REDIS_DBS by slot), so no
#              two packages ever share a stream. Db 0 is reserved for `make dev`,
#              so a running dev stack and this lane never collide either. More
#              than one package touches Redis, and at least two FLUSHDB between
#              tests, so this is an isolation boundary rather than a convenience
#              — batches wait before reusing the reserved test block.
# Within a process nothing changes — still -p 1, the same sequential model that is
# green today — so no test file needs editing. A package too heavy for one
# process's timeout runs as several parts (split_parts in scripts/lib-laneorder.sh),
# and each part is a process with its own private state like any other.
#
# Same teeth as the serial lane: zero-skip guard (a SKIP fails the run) and any
# package failure fails the whole run. MARGINCE_ENV=dev is exported so the HTTP
# e2e suites boot under the same non-production postures the serial lane uses.
#
# Shard mode (the CI matrix): INTEGRATION_SHARD="k/N" slices the lane across N
# independent runners BY TEST, not by package — package-level fan-out bottoms
# out at the heaviest package (compose/integration is minutes of serial tests),
# so each shard runs a deterministic, duration-balanced slice of every package's
# top-level Test functions via -run. Discovery is static (`func Test…` in the
# package's *_test.go files) so it costs no compile; files under the known
# opt-in lane tags (e2e_llm, livesmoke, voicelive) and the by-hand benchmark lane
# (`integration && bench`) are skipped exactly as the compiler
# skips them, and any other constraint — an expression, or a lone tag the
# allowlist does not know (a satisfied built-in like linux or cgo would
# compile but never be sliced) — fails discovery loudly instead of being
# silently mis-sliced. Shard teeth on top of
# the lane's own: the set of
# tests a shard actually ran must equal its assigned slice, and
# scripts/test-integration-reconcile.sh re-checks the union across shards
# (complete + disjoint) before merging coverage — a slicing bug can only read
# as red, never as a quietly thinner lane.
#
# Env:
#   INTEGRATION_JOBS        max concurrent packages (default: min(nproc, 8))
#   INTEGRATION_SHARD       "k/N" → run the k-th of N test slices (CI matrix)
#   INTEGRATION_SHARD_OUT   shard mode only: directory receiving the manifests
#                           (discovery/assigned/ran/meta) and per-package binary
#                           covdata pods for the CI fan-in to reconcile + merge;
#                           coverage instrumentation is on iff this is set
#   INTEGRATION_BUILD_OUT   compile every lane package's covered test binary into
#                           this directory, then stop: no database, no tests. CI
#                           does this once per run, and its shards read the result
#   INTEGRATION_BIN_DIR     shard mode only: run each package from the binary
#                           INTEGRATION_BUILD_OUT left here instead of compiling it
#   INTEGRATION_TEST_TIMING the test durations to split by (default: the committed
#                           scripts/integration-test-timing.txt). Every shard of
#                           one run must read the same file
#   INTEGRATION_TIMEOUT     per-package go-test timeout, as <seconds>s (default
#                           900s; the budget column parses this). Resolved in
#                           scripts/lib-testdb.sh, shared with the one-package lane
#   MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN   owner + app DSNs (Makefile defaults)
#
# This lane also EXPORTS one variable to each package process, per slot rather
# than for the run: MARGINCE_TEST_CLONE_DB names the throwaway clone that slot
# just created. It is what lets testdb.EnsureSchema skip re-migrating a database
# this script copied from an already-migrated template — see testdb's CloneDBEnv
# for the four things it proves before taking the copy at its word, and why the
# variable carries the NAME rather than a flag. The serial lane deliberately
# does not set it: it runs on the template itself, where the rebuild is what
# keeps one package's residue out of every later clone.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/lib-testdb.sh
source "$ROOT/scripts/lib-testdb.sh"
source "$ROOT/scripts/lib-laneorder.sh"
source "$ROOT/scripts/lib-lanebinaries.sh"
source "$ROOT/scripts/lib-integration-artifacts.sh"
parse_test_dsn

SHARD_IDX=0 SHARD_TOTAL=0
if [[ -n "${INTEGRATION_SHARD:-}" ]]; then
  if [[ ! "$INTEGRATION_SHARD" =~ ^([0-9]+)/([0-9]+)$ ]]; then
    echo "FAIL: INTEGRATION_SHARD must be k/N (e.g. 3/8), got '${INTEGRATION_SHARD}'"
    exit 1
  fi
  SHARD_IDX="${BASH_REMATCH[1]}" SHARD_TOTAL="${BASH_REMATCH[2]}"
  if (( SHARD_IDX < 1 || SHARD_IDX > SHARD_TOTAL )); then
    echo "FAIL: INTEGRATION_SHARD index out of range: ${INTEGRATION_SHARD}"
    exit 1
  fi
fi

SHARD_OUT="${INTEGRATION_SHARD_OUT:-}"
if [[ -n "$SHARD_OUT" ]]; then
  if (( SHARD_TOTAL == 0 )); then
    echo "FAIL: INTEGRATION_SHARD_OUT is set but INTEGRATION_SHARD is not — the manifests only mean something for a shard"
    exit 1
  fi
  case "$SHARD_OUT" in /*) ;; *) SHARD_OUT="$ROOT/$SHARD_OUT";; esac
  # Binary covdata pods, one dir per package slot; the fan-in merges every
  # shard's pods (plus the unit pass) into the one text profile SonarCloud
  # reads. Kept binary here because `go tool covdata` merges dirs, not text.
  COVERDIR="$SHARD_OUT/covdata"
  rm -rf "$COVERDIR"
  rm -f "$SHARD_OUT/discovery.txt" "$SHARD_OUT/assigned.txt" "$SHARD_OUT/ran.txt" "$SHARD_OUT/meta.txt" "$SHARD_OUT/rerun.txt"
  rm -rf "$SHARD_OUT/parts" "$SHARD_OUT/failed-logs"
  mkdir -p "$COVERDIR"
  export COVERDIR
fi

BUILD_OUT="${INTEGRATION_BUILD_OUT:-}"
BIN_DIR="${INTEGRATION_BIN_DIR:-}"
if [[ -n "$BUILD_OUT" ]] && { (( SHARD_TOTAL > 0 )) || [[ -n "$BIN_DIR" ]]; }; then
  echo "FAIL: INTEGRATION_BUILD_OUT compiles the whole lane and runs nothing — it takes neither INTEGRATION_SHARD nor INTEGRATION_BIN_DIR"
  exit 1
fi
# The binaries carry coverage instrumentation, and only a shard with a manifest
# directory has somewhere to put the pods they write.
if [[ -n "$BIN_DIR" && -z "$SHARD_OUT" ]]; then
  echo "FAIL: INTEGRATION_BIN_DIR runs covered binaries, so it needs INTEGRATION_SHARD_OUT for their pods"
  exit 1
fi
[[ -z "$BUILD_OUT" ]] || case "$BUILD_OUT" in /*) ;; *) BUILD_OUT="$ROOT/$BUILD_OUT";; esac
[[ -z "$BIN_DIR" ]] || case "$BIN_DIR" in /*) ;; *) BIN_DIR="$ROOT/$BIN_DIR";; esac
export BIN_DIR

# A named file that is missing is a broken hand-off, never a reason to fall
# back to the committed one: a shard that did would deal from other weights
# than its siblings.
SHARD_TIMING="${INTEGRATION_TEST_TIMING:-$ROOT/scripts/integration-test-timing.txt}"
case "$SHARD_TIMING" in /*) ;; *) SHARD_TIMING="$ROOT/$SHARD_TIMING";; esac
if [[ ! -s "$SHARD_TIMING" ]]; then
  echo "FAIL: the test durations to split by, $SHARD_TIMING, are missing or empty (INTEGRATION_TEST_TIMING)"
  exit 1
fi

# Per-package go-test timeout and its budget rule — shared with the one-package
# lane (scripts/lib-testdb.sh resolve_it_timeout), so both cost a package the same.
resolve_it_timeout
# The BUDGET is the policy number: what a package is allowed to cost before it
# has to be split. It is reported against, never enforced, and it does not move
# when the timeout below does.
IT_BUDGET="$IT_TIMEOUT"

# The TIMEOUT is a hang detector, and it has to fit the work a run actually
# does. Those were one number until this line, which is why CI was green while
# the same lane locally was killing a package with every test passing.
#
# A sharded run executes a SLICE — CI's matrix gives each shard a sixth of the
# tree — so the budget is generous headroom for it. An unsharded run executes
# whole packages, and compose/integration alone is 2284 tests: measured at 886s
# on an unmodified main, past the bound a slice needs. go test kills it at the ceiling,
# so NONE of its tests report, and a real regression in the same push would be
# invisible behind a timeout.
#
# So an unsharded run gets a multiple, and the budget column still prices every
# package against the budget policy — "split it before it crosses" keeps being
# said about a package that is genuinely too big. What stops is a hang detector
# calibrated for a sixth of the work killing a run doing all of it.
#
# An explicit INTEGRATION_TIMEOUT overrides both halves, because a caller naming
# a bound means it.
if [[ -z "${INTEGRATION_TIMEOUT:-}" ]] && (( SHARD_TOTAL <= 1 )); then
  # Twice the budget. The slowest package measures about 1x it today, so this
  # is headroom for a slower machine rather than a number chosen to fit — and it
  # is still far under the CI job limit this bound exists to pre-empt with a
  # legible message.
  IT_TIMEOUT=1800s
fi
# A single-shard coverage run adds instrumentation on top of whole packages.
if [[ -n "${COVERDIR:-}" && -z "${INTEGRATION_TIMEOUT:-}" ]] && (( SHARD_TOTAL == 1 )); then
  IT_TIMEOUT=2700s
fi
export IT_TIMEOUT IT_BUDGET

ncpu() { sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4; }
JOBS="${INTEGRATION_JOBS:-$(( $(ncpu) < 8 ? $(ncpu) : 8 ))}"

# The lane's connection budget, declared once in scripts/lib-testdb.sh and
# computed for THIS run's concurrency. Exports MARGINCE_TEST_POOL_MAX_CONNS and
# LANE_CONN_BUDGET for the harness.
declare_lane_budget "$JOBS"

# The coverage flags of a sharded run, one string because run_one reads it in
# the child shell xargs starts, and an array does not cross that boundary.
export LANE_COVER_FLAGS="-cover -coverpkg=./... -covermode=atomic"

# Compile the lane once before fanning out, with the flags every package's
# `go test` uses. Concurrent `go` processes do not share a compile still in
# flight, so packages starting together on a cold cache each built the same
# dependencies. It runs beside the template build, which mostly waits on
# Postgres; `wait` reports its status, so a failure cannot hide behind the `&`.
prebuild_flags=(-tags=integration)
if [[ -n "${COVERDIR:-}" || -n "$BUILD_OUT" ]]; then
  read -r -a cover_flags <<< "$LANE_COVER_FLAGS"
  prebuild_flags+=("${cover_flags[@]}")
fi
# A run from binaries has nothing to compile, and a run that builds them needs
# no database, so each skips the half that is not its own.
prebuild_pid=""
if [[ -z "$BUILD_OUT" && -z "$BIN_DIR" ]]; then
  prebuild_log="$(mktemp)"
  ( cd "$ROOT/backend" && go build "${prebuild_flags[@]}" ./... ) > "$prebuild_log" 2>&1 &
  prebuild_pid=$!
  # A failed template build exits here under set -e; stop and reap the compiler
  # rather than leave it running. The lane's own EXIT trap below replaces this.
  trap 'kill "$prebuild_pid" 2>/dev/null || true; wait "$prebuild_pid" 2>/dev/null || true; rm -f "$prebuild_log"' EXIT
fi

# Build the migrated template once, fresh, before fanning out. Every package
# clones from it (CREATE DATABASE ... TEMPLATE) instead of re-migrating.
if [[ -z "$BUILD_OUT" ]]; then
  echo "test-integration-parallel: building migrated template ${TEMPLATE_NAME}…"
  build_template
fi

if [[ -n "$prebuild_pid" ]]; then
  prebuild_started=$SECONDS
  if ! wait "$prebuild_pid"; then
    cat "$prebuild_log"
    rm -f "$prebuild_log"
    echo "FAIL: the integration lane does not compile with ${prebuild_flags[*]} — the compiler's error is above"
    exit 1
  fi
  rm -f "$prebuild_log"
  echo "test-integration-parallel: compiled the lane once ($(( SECONDS - prebuild_started ))s after the template was ready)"
fi

# Every integration test in this repo lives in the backend module.
GO_DIRS=(backend)

# Redis logical dbs available to the lane: every db the server serves except 0,
# which `make dev` owns, and the block above this one, which the per-worktree
# dev stacks take. So --databases in docker-compose.dev.yml is higher than this
# number, not equal to it, and the gate asserts the offset rather than equality.
# Packages in each batch own distinct databases until the whole batch exits.
# Later batches reuse that block without entering the worktree dev allocation.
REDIS_DBS=64

LANE_PKGS="$(mktemp)" WORK="$(mktemp)"
DISCOVERY="$(mktemp)" ASSIGNED="$(mktemp)" RAN="$(mktemp)" UNTAGGED="$(mktemp)"
TIMING="$(mktemp)" TEST_TIMING="$(mktemp)" WALLCLOCK="$(mktemp)" FAILED_PKGS="$(mktemp)"
# The packages go test KILLED for exceeding the budget. Kept apart from
# FAILED_PKGS because a timeout explains something no other failure does: every
# one of that package's assigned tests is missing from the run, and the
# reconciliation below would otherwise report all of them as a discovery
# divergence — the loudest thing in the output, naming the sharding mechanism
# rather than the clock.
TIMEDOUT="$(mktemp)"
# When the lane started, so each package can record WHEN it ran and not only how
# long its tests took. The per-package cost report deliberately prices tests
# alone; that leaves everything around them — clone provisioning, compiling a
# test binary, process start — invisible, and it is not small: measured at 42s of
# a 221s run, which is more than any package except the longest. A number nobody
# can see is a number nobody reduces.
#
# Whole seconds: this attributes tens of seconds across a run of hundreds, and a
# sub-second clock would mean a Perl or Python dependency the lane does not
# otherwise have.
LANE_T0="$(date +%s)"
export LANE_T0 WALLCLOCK
OUTDIR="$(mktemp -d)"
# Each package process appends the large statements it sends here; the lane
# checks them for a repeated content gate once every package passed.
export MARGINCE_SQL_CENSUS_DIR="$OUTDIR/sql-census"
# Set on the failing path only, and read by the trap below, which then leaves
# $OUTDIR behind. The per-package logs are the ONLY copy of a failure that is not
# interleaved with 35 other packages; deleting them on the way out made an
# intermittent failure permanently undiagnosable — the class of failure that most
# needs the evidence, because the re-run that would reproduce it usually passes.
# A green run still cleans up: nobody wants a temp dir per passing run.
KEEP_LOGS=""
trap 'rm -f "$LANE_PKGS" "$WORK" "$DISCOVERY" "$ASSIGNED" "$RAN" "$UNTAGGED" "$TIMING" "$TEST_TIMING" "$WALLCLOCK" "$FAILED_PKGS" ${GROUPED:+"$GROUPED"}; [[ -n "$KEEP_LOGS" ]] || rm -rf "$OUTDIR"; rm -rf ${REGEX_DIR:+"$REGEX_DIR"}' EXIT
prepare_test_extension_manifest "$OUTDIR"

# Static discovery of every top-level Test function of every integration-TAGGED
# file, as "module_dir|rel|TestName"; the rules are in scripts/lib-laneorder.sh.
# Lane membership comes from the tests found, never from grepping for the tag:
# a header comment may name `go:build integration`, and a package whose tagged
# files declare no Test has nothing for this lane to run.
#
# Untagged files are left out, and counted. `-tags=integration` is ADDITIVE, so
# `go test -tags=integration <pkg>` compiles a package's untagged tests too, and
# without the -run union the lane would re-run the unit suite against real
# infrastructure.
lane_discover "$UNTAGGED" "${GO_DIRS[@]}" > "$DISCOVERY"

NTESTS=$(wc -l < "$DISCOVERY" | tr -d ' ')

# The lane's packages are exactly those that produced a tagged test. Deriving
# the list rather than maintaining it is what stops a package with no tagged
# test being handed a clone, and it cannot drift from what discovery found.
cut -d'|' -f1,2 "$DISCOVERY" | LC_ALL=C sort -u > "$LANE_PKGS"
NPKGS_DISCOVERED=$(wc -l < "$LANE_PKGS" | tr -d ' ')

# Untagged tests this lane no longer schedules — counted only inside packages it
# actually visits, since a package with no tagged test was never its business.
NUNTAGGED=$(awk -F'|' '
  NR == FNR { lane[$1 "|" $2] = 1; next }
  ($1 "|" $2) in lane { n += $3 }
  END { print n + 0 }
' "$LANE_PKGS" "$UNTAGGED")

# What the lane's binaries are compiled from. The build records it, and a run
# from binaries refuses them when its own checkout gives another answer.
if [[ -n "$BUILD_OUT" || -n "$BIN_DIR" ]]; then
  lane_packages=()
  while IFS='|' read -r _ rel; do lane_packages+=("$rel"); done < "$LANE_PKGS"
  source_stamp="$(lane_source_stamp "$ROOT/backend" integration "${lane_packages[@]+"${lane_packages[@]}"}")" || exit 1
  [[ -z "$BIN_DIR" ]] || lane_bin_dir_check "$BIN_DIR" "$source_stamp" || exit 1
fi

# Build mode: one `go test` compiles and links every lane package, and the
# keeper it runs instead of each binary copies that binary out. Every
# discovered package must then have one, because a shard fails on a missing
# binary and the build is the place to name the cause.
if [[ -n "$BUILD_OUT" ]]; then
  if (( NPKGS_DISCOVERED == 0 )); then
    echo "FAIL: no integration packages discovered — nothing to compile"
    exit 1
  fi
  lane_build_dir_prepare "$BUILD_OUT" "$source_stamp" || exit 1
  build_started=$SECONDS
  build_packages=("${lane_packages[@]}")
  # -count=1: a cached result would skip the keeper, and with it the binary.
  if ! ( cd "$ROOT/backend" \
      && LANE_MODULE_DIR="$ROOT/backend" LANE_BUILD_OUT="$BUILD_OUT" \
         go test -count=1 -vet=off -exec="\"$ROOT/scripts/test-integration-keep-binary.sh\"" \
           "${prebuild_flags[@]}" "${build_packages[@]}" ); then
    echo "FAIL: the integration lane does not compile with ${prebuild_flags[*]} — the compiler's error is above"
    exit 1
  fi
  missing=0
  for rel in "${build_packages[@]}"; do
    [[ -x "$BUILD_OUT/$rel/integration.test" ]] && continue
    echo "FAIL: go test passed $rel but left no binary at $BUILD_OUT/$rel/integration.test"
    missing=1
  done
  (( missing == 0 )) || exit 1
  echo "OK: compiled ${#build_packages[@]} integration test binaries into $BUILD_OUT ($(( SECONDS - build_started ))s)"
  exit 0
fi

if (( SHARD_TOTAL > 0 )); then
  if (( NTESTS < SHARD_TOTAL )); then
    echo "FAIL: discovered only $NTESTS integration tests for $SHARD_TOTAL shards — the discovery is broken or the shard count is absurd"
    exit 1
  fi
  # Split by measured duration, so slow tests do not land on one shard; the
  # rules live in scripts/lib-laneorder.sh, which is unit-tested. Every shard
  # computes the same split from the same tree and weights, and the fan-in
  # verifies that the slices are complete and disjoint instead of trusting it.
  assign_shard "$DISCOVERY" "$SHARD_TIMING" "$SHARD_IDX" "$SHARD_TOTAL" > "$ASSIGNED"
else
  # Unsharded: the whole lane is this run's slice. Naming it explicitly is what
  # lets the -run union and the ran==assigned teeth below serve both modes.
  cp "$DISCOVERY" "$ASSIGNED"
fi

# Each package's anchored -run union of its assigned test names goes to a
# per-slot FILE, not onto the work line — hundreds of test names make a regex
# far past what xargs -I can carry. The union is also what confines the run to
# tagged tests, so it is built in BOTH modes, not only when sharding.
#
# A package whose slice weighs more than a quarter of the timeout runs as
# several parts, each its own process on its own clone. The weights are seconds
# from green CI runs, and one slice of ./internal/compose has taken 2.6 times
# as long on one runner as on another, so a part near the whole timeout would be
# killed with every test passing.
PART_SECONDS=$(( ${IT_TIMEOUT%s} / 4 ))
GROUPED="$(mktemp)"
split_parts "$ASSIGNED" "$SHARD_TIMING" "$PART_SECONDS" > "$GROUPED"
REGEX_DIR="$(mktemp -d)"
export REGEX_DIR
: > "$WORK"
# Longest first — the ordering rules and their rationale live in
# scripts/lib-laneorder.sh, which is unit-tested. A shard orders its parts by
# the weights it split by; a whole lane orders packages by what the last run on
# this machine measured.
#
# Ordered HERE, before slots are numbered, so each package's -run slice, clone,
# bucket and Redis db follow it rather than being handed to whoever inherits its
# slot number.
MEASURED_HINT="$ROOT/.tmp/integration-lane-timing.txt"
BASELINE_HINT="$ROOT/scripts/integration-lane-timing.txt"
ORDER_HINT="$(resolve_order_hint "$MEASURED_HINT" "$BASELINE_HINT")"
if (( SHARD_TOTAL > 0 )); then
  order_by_weight "$GROUPED" "$SHARD_TIMING" > "${GROUPED}.ordered"
  mv "${GROUPED}.ordered" "$GROUPED"
elif [[ -n "$ORDER_HINT" ]]; then
  order_by_hint "$GROUPED" "$ORDER_HINT" > "${GROUPED}.ordered"
  mv "${GROUPED}.ordered" "$GROUPED"
fi

slot=0
while IFS= read -r line; do
  slot=$((slot + 1))
  d="${line%%|*}" rest="${line#*|}"
  printf '%s|%s\n' "$d" "${rest%%|*}" >> "$WORK"
  printf '%s' "${rest#*|}" > "$REGEX_DIR/$slot"
done < "$GROUPED"
rm -f "$GROUPED"

NPARTS=$(wc -l < "$WORK" | tr -d ' ')
NPKGS=$(LC_ALL=C sort -u "$WORK" | wc -l | tr -d ' ')
# Say what was left to the unit lane. An unreported exclusion and a broken
# selector produce the same silence, and this lane's whole contract is that a
# thinner run must never read as a passing one.
echo "test-integration-parallel: $NTESTS tagged tests in $NPKGS_DISCOVERED packages; skipped $NUNTAGGED untagged (they run in \`make check\`)"
if (( SHARD_TOTAL > 0 )); then
  echo "test-integration-parallel: shard ${SHARD_IDX}/${SHARD_TOTAL} — $(wc -l < "$ASSIGNED" | tr -d ' ') of $NTESTS tests across $NPKGS packages in $NPARTS processes, up to $JOBS concurrent (template db=$TEMPLATE_NAME)"
else
  echo "test-integration-parallel: $NPKGS packages in $NPARTS processes, up to $JOBS concurrent (template db=$TEMPLATE_NAME)"
fi

# One job = clone an empty db + own a private MinIO bucket, run that package
# against them, drop the clone. In shard mode REGEX_DIR/<idx> holds the
# package's -run slice filter.
run_one() {
  local line="$1" idx="$2" outdir="$3"
  # Offsets from lane start, so the report can say when this package OCCUPIED a
  # slot as against how long its tests ran. The difference is the provisioning
  # around it, which the cost report prices at nothing.
  local began=$(( $(date +%s) - LANE_T0 ))
  local d="${line%%|*}" rel="${line#*|}" runre=""
  [[ -n "${REGEX_DIR:-}" ]] && runre="$(cat "$REGEX_DIR/$idx")"
  local db="margince_it_p${idx}_$$"
  local log="$outdir/$idx.log"
  local bucket; bucket="$(bucket_for "$idx")"
  # The batch barrier lets this wrap only after the previous holder exits.
  # Database 0 and the higher worktree dev block stay outside this mapping.
  local redis_db=$(( 1 + (idx - 1) % REDIS_DBS ))
  # Coverage args (empty unless COVERDIR is set — shard mode with a manifest
  # dir). -coverpkg=./... attributes cross-package exercise; binary output goes
  # to a per-package pod the CI fan-in merges.
  local cover_pre=() cover_post=() run_args=()
  if [[ -n "${COVERDIR:-}" ]]; then
    mkdir -p "$COVERDIR/$idx"
    read -r -a cover_pre <<< "$LANE_COVER_FLAGS"
    cover_post=(-args -test.gocoverdir="$COVERDIR/$idx")
  fi
  [[ -n "$runre" ]] && run_args=(-run "$runre")
  # -vet=off: `make lint` vets tagged code with a superset of these analyzers, and
  # gates/lintbuildtagreach_test.go holds that. A slot would only repeat it.
  local test_cmd=(go test -p 1 -vet=off -tags=integration -v -count=1 -timeout="$IT_TIMEOUT"
    "${cover_pre[@]+"${cover_pre[@]}"}" "${run_args[@]+"${run_args[@]}"}"
    "$rel" "${cover_post[@]+"${cover_post[@]}"}")
  [[ -z "${BIN_DIR:-}" ]] || test_cmd=(run_binary "$rel" "$COVERDIR/$idx" "${run_args[@]+"${run_args[@]}"}")
  {
    echo "=== integration $d $rel (db=$db bucket=$bucket redis-db=$redis_db${runre:+ sliced}) ==="
    make_clone "$db"
    local st=0
    ( cd "$d" \
        && MARGINCE_ENV=dev \
           MARGINCE_TEST_CLONE_DB="$db" \
           MARGINCE_TEST_DSN="$(owner_clone_dsn "$db")" \
           MARGINCE_TEST_APP_DSN="$(app_clone_dsn "$db")" \
           MARGINCE_TEST_BLOBSTORE_BUCKET="$bucket" \
           MARGINCE_TEST_REDIS_DB="$redis_db" \
        "${test_cmd[@]}" ) || st=$?
    if ! drop_clone "$db"; then
      echo "FAIL: clone db $db was not dropped — leaked on the test cluster"
      if [[ "$st" -eq 0 ]]; then st=1; fi
    fi
    echo "EXIT $st"
  } > "$log" 2>&1
  # Written after the log block closes, so a package that failed still reports
  # the slot time it consumed — a red run is exactly when someone asks where the
  # wall clock went.
  #
  # Best effort, and deliberately the one place in this file that discards a
  # status: this is the LAST command in run_one, the workers run under `xargs -P`,
  # and the script is `set -e`. A failed append would therefore fail the whole
  # lane over a measurement nobody asked to be load-bearing. It hides no test
  # result — a package's verdict is the EXIT line inside the log above, which the
  # reconciliation at the end reads.
  local ended=$(( $(date +%s) - LANE_T0 ))
  printf '%s\n' "${rel}|${began}|${ended}|${idx}" >> "$WALLCLOCK" || :
}
# run_binary REL COVERDIR [-run REGEX] — the `go test -v` of run_one, from the
# binary the build left in BIN_DIR. It runs in the package directory, as go test
# runs it, and then prints go test's closing "ok|FAIL <pkg> <secs>s" line, which
# the cost report reads.
run_binary() {
  local rel="$1" coverdir="$2" st=0 t0 t1
  shift 2
  local run_flags=()
  [[ "${1:-}" != "-run" ]] || run_flags=(-test.run="$2")
  t0="${EPOCHREALTIME:-$(date +%s)}"
  ( cd "$rel" && "$BIN_DIR/$rel/integration.test" -test.paniconexit0 -test.v=true -test.count=1 \
      -test.timeout="$IT_TIMEOUT" "${run_flags[@]+"${run_flags[@]}"}" -test.gocoverdir="$coverdir" ) || st=$?
  t1="${EPOCHREALTIME:-$(date +%s)}"
  local verdict="ok  "
  (( st == 0 )) || verdict=FAIL
  printf '%s\t%s\t%ss\n' "$verdict" "$rel" "$(lane_elapsed "$t0" "$t1")"
  return "$st"
}
export -f run_one run_binary lane_elapsed lane_micros owner_clone_dsn app_clone_dsn make_clone drop_clone db_admin bucket_for
# The workers run in re-exec'd shells, so a variable run_one reads must be
# exported and not merely set. REDIS_DBS is the only one: unexported it expands
# to empty, and `% ` is a division by zero rather than a wrong db.
export REDIS_DBS

# Compile this run's tests in one build before fanning out. `go build` above stops
# short of test code. Each slot would otherwise compile its own while the first
# packages run their tests on the same cores.
#
# Same flags as the `go test` in run_one, or that run recompiles. A package that
# does not compile fails again in its own slot, which reports it per package.
#
# A run from binaries compiles nothing. It checks instead that the build left a
# binary for every package of its slice, so a gap fails here by name and not as
# an exec error inside one slot's log.
if [[ -n "$BIN_DIR" ]]; then
  missing=0
  while IFS='|' read -r _ rel; do
    [[ -x "$BIN_DIR/$rel/integration.test" ]] && continue
    echo "FAIL: no test binary for $rel at $BIN_DIR/$rel/integration.test — the integration build did not produce it"
    missing=1
  done < "$WORK"
  (( missing == 0 )) || exit 1
  echo "test-integration-parallel: running $NPKGS packages from the binaries in $BIN_DIR"
else
  test_build_started=$SECONDS
  test_packages=()
  while IFS='|' read -r _ rel; do test_packages+=("$rel"); done < "$WORK"
  if ( cd "$ROOT/backend" && go test -vet=off -exec=true "${prebuild_flags[@]}" "${test_packages[@]+"${test_packages[@]}"}" ) > /dev/null 2>&1; then
    echo "test-integration-parallel: compiled the tests once ($(( SECONDS - test_build_started ))s)"
  else
    echo "test-integration-parallel: a test package does not compile; its slot prints the error"
  fi
fi

# When the first package could START, which separates two very different costs
# that both look like "before the tests ran": everything this script does before
# fanning out (the template, the constraint scan over every Go file in the module,
# the test enumeration) versus a package queueing for a busy slot. Only the first
# is this script's to reduce.
FANOUT_T0=$(( $(date +%s) - LANE_T0 ))

run_integration_packages "$WORK" "$JOBS" "$REDIS_DBS" "$OUTDIR"

# Aggregate: print every log in package (idx) order, then enforce the teeth.
fail=0
ran=0
for base in $(cd "$OUTDIR" && ls -1 -- *.log 2>/dev/null | sort -n); do
  log="$OUTDIR/$base"
  cat "$log"
  ran=$((ran + 1))
  # The package this log belongs to, resolved BEFORE the verdict below rather
  # than after it: $idx maps straight back through $WORK, so the failing
  # package's name is in hand at the exact moment the flag is set. It used to be
  # computed a few lines later and the verdict said only "36 packages", which
  # left the reader to find the failure by eye in the interleaved output of all
  # of them.
  idx="${base%.log}"
  line="$(sed -n "${idx}p" "$WORK")"
  d="${line%%|*}" rest="${line#*|}"
  rel="${rest%%|*}"
  if ! grep -q "^EXIT 0$" "$log"; then
    fail=1
    printf '%s\n' "$d|$rel|$base" >> "$FAILED_PKGS"
    # go test's own words for the two ways it kills a package over time: the
    # panic it raises itself, and the "*** Test killed" a -timeout kill prints.
    # Matched on the log rather than inferred from a missing test count, because
    # a package that died for any other reason is also missing its tests and
    # must NOT be excused from the reconciliation.
    if lane_timed_out "$log"; then
      printf '%s\n' "$d|$rel" >> "$TIMEDOUT"
    fi
  fi
  # Top-level results only (subtest lines are indented): "rel|TestName" per
  # the package this log belongs to, for the ran==assigned check below.
  #
  # SKIP counts as run. A skipped test still executed, and this lane forbids
  # skipping outright a few lines down — omitting it here would report it as
  # "assigned but not run" instead, which names the reconciliation rather than
  # the skip and sends the reader looking for a dead worker.
  grep -E '^--- (PASS|FAIL|SKIP): ' "$log" | awk -v p="$d|$rel|" '{print p $3}' >> "$RAN" || true
  # "dir|rel|TestName|seconds", the line shape scripts/integration-test-timing.txt
  # is refreshed from.
  grep -E '^--- (PASS|FAIL|SKIP): ' "$log" \
    | awk -v p="$d|$rel|" '{ secs = $4; gsub(/[()s]/, "", secs); print p $3 "|" secs }' >> "$TEST_TIMING" || true
  # Cost, taken from `go test`'s own trailing "ok <pkg> <n>s" — millisecond
  # precision already in the log, and it excludes the clone provisioning around
  # it, so what it prices is the tests themselves.
  # The duration field is matched, not the line end: a covered shard appends
  # "coverage: NN.N% of statements" after it, and anchoring on s$ would have
  # priced every CI package at nothing — shard mode always sets COVERDIR.
  # The slot ends the row: parts of one package share its name.
  awk -v rel="$rel" -v slot="$idx" '
    /^(ok|FAIL)[ \t]+[^ \t]+[ \t]+[0-9.]+s([ \t]|$)/ { secs = $3; sub(/s$/, "", secs) }
    /^--- (PASS|FAIL|SKIP): / { n++ }
    END { if (secs != "") printf "%s|%s|%d|%s\n", rel, secs, n + 0, slot }
  ' "$log" >> "$TIMING" || true
done

# Per-package cost. ADVISORY — printed, never enforced. A raw wall-time ceiling
# is the wrong shape: this package count grows, and a ceiling that healthy growth
# re-crosses gets bumped until nobody reads it. Amortized ms/test is
# scale-invariant, which is what makes it a signal — a per-test cost that is
# priced per table rather than per row trips it at any test count, while 300 clean
# new tests do not. Enforcing a number measured on one machine is the same
# unverified claim this lane keeps learning about, so the ceiling waits for a CI
# baseline.
if [[ -s "$TIMING" ]]; then
  # Ordered by ms/test, not by total, so an anomaly surfaces instead of merely
  # the biggest package: a small suite paying a per-test schema migration reads an
  # order of magnitude above the lane norm, and that is the shape worth seeing
  # first.
  # The budget column is what the ms/test column cannot say: a package can be
  # perfectly efficient per test and still be seconds from the timeout simply by
  # having grown. That margin closing is the failure this lane learned the hard
  # way, and it is only visible if the run prints it before it crosses.
  echo "test-integration-parallel: per-package cost (advisory, budget ${IT_BUDGET})"
  awk -F'|' '{ printf "%s|%s|%s|%.4f\n", $1, $2, $3, ($3 ? $2 * 1000 / $3 : 0) }' "$TIMING" \
    | LC_ALL=C sort -t'|' -k4 -rn \
    | awk -F'|' -v budget="${IT_BUDGET%s}" '
        { total += $2; tests += $3
          share = (budget ? $2 * 100 / budget : 0)
          # A package past half its budget is the signal that it needs splitting,
          # and it is the signal that would have made the class visible BEFORE a
          # package became unpassable rather than after. Still advisory — this is
          # the number already being printed, with a threshold on it, not the raw
          # wall-clock ceiling the comment above rules out.
          if (share >= 50) { over[++n] = sprintf("  %s is at %.1f%% of the %ss budget — split it before it crosses", $1, share, budget) }
          printf "  %-44s %8.2fs  %5d tests  %7.1f ms/test  %5.1f%% of budget\n", $1, $2, $3, $4, share }
        END { if (tests) printf "  %-44s %8.2fs  %5d tests  %7.1f ms/test\n", "TOTAL (sum of packages)", total, tests, total * 1000 / tests
              for (i = 1; i <= n; i++) print over[i] }
      '
fi

# Where the WALL clock went, which the cost report above cannot say: it prices
# tests, and the lane's wall clock is set by the package that finishes last plus
# everything that happened before it could start. Those two are the only numbers
# a split or a scheduling change moves, so they are worth printing every run
# rather than reconstructing from a log afterwards.
if [[ -s "$WALLCLOCK" ]] && [[ -s "$TIMING" ]]; then
  # Elapsed is computed here rather than in awk: systime() is a GNU extension and
  # this lane runs on BSD awk too.
  LANE_ELAPSED=$(( $(date +%s) - LANE_T0 ))
  critical_part "$WALLCLOCK" "$TIMING" | awk -F"|" -v elapsed="$LANE_ELAPSED" -v fanout="$FANOUT_T0" '
    {
      last = $1; began = $2; ended = $3; tests = $4
      # Tracked as FOUND rather than defaulted to zero: a package whose log
      # carries no go-test duration (a build failure prints "FAIL pkg [build
      # failed]", with no seconds) would otherwise report its entire slot as
      # provisioning — inventing a number instead of admitting it is missing.
      found = (tests != "")
      printf "test-integration-parallel: wall clock — %ds total\n", elapsed
      printf "  %ds  before any package could start (template, constraint scan, test enumeration)\n", fanout
      printf "  %ds  then %s waited for a slot\n", began - fanout, last
      if (found) {
        printf "  %ds  %s occupied a slot, of which %.1fs was its tests\n", ended - began, last, tests
        printf "  %.1fs  provisioning that slot (clone, compile, process start) — not priced above\n", (ended - began) - tests
      } else {
        printf "  %ds  %s occupied a slot; it recorded no test duration, so the tests/provisioning split is unavailable\n", ended - began, last
      }
    }'
fi

# Leave the durations behind for the next run to dispatch by. Full runs only: a
# shard measures its own slice, and saving that would order the next full run by
# a twelfth of each package.
#
# Published by rename, never written in place. The next run treats a non-empty
# hint as authoritative, so a HALF-written one is worse than none: truncate a
# heavy package's duration mid-line and it sorts LAST, which is precisely the
# schedule this ordering exists to avoid. The temporary file is created beside the
# destination so the rename stays within one filesystem and is atomic.
#
# Every step is non-fatal, and on any failure the previous hint survives
# untouched. A scheduling hint that could fail a green lane would be a worse
# trade than dispatching in discovery order forever.
if [[ -s "$TIMING" ]] && (( SHARD_TOTAL == 0 )); then
  hint_dir="$(dirname "$MEASURED_HINT")"
  if mkdir -p "$hint_dir" 2>/dev/null && hint_tmp="$(mktemp "$hint_dir/.integration-lane-timing.XXXXXX" 2>/dev/null)"; then
    # A package split into parts has one TIMING line per part; its hint is their sum.
    awk -F'|' '{ secs[$1] += $2 } END { for (p in secs) printf "%s|%.3f\n", p, secs[p] }' "$TIMING" > "$hint_tmp" 2>/dev/null \
      && mv -f "$hint_tmp" "$MEASURED_HINT" 2>/dev/null \
      || rm -f "$hint_tmp"
  fi
fi

# Reconcile against discovery: a green run must have executed every package we
# found. NPKGS=0 (a constraint-scan regression that admits no file) or a missing
# log (a worker that never wrote its EXIT line) must read as red — otherwise the
# "0 skips" sentinel below is a false green.
if [[ "$NPKGS_DISCOVERED" -eq 0 ]]; then
  echo "FAIL: no integration packages discovered — the build-constraint scan admitted no tagged file (regression?)"
  fail=1
elif [[ "$ran" -ne "$NPARTS" ]]; then
  # $NPARTS is THIS run's process count — the shard's slice, not the lane total —
  # so it is the number a missing worker has to be counted against.
  echo "FAIL: ran $ran package log(s) but this run started $NPARTS process(es) — a worker did not report; treating as red"
  fail=1
fi

# The teeth: the tests that actually ran are exactly the assigned slice. A
# discovery/compiler divergence (a test the grep saw but go test didn't run, or
# vice versa) reads as red here, not as a quietly thinner lane. These bind in
# BOTH modes — unsharded runs are confined by the same -run union, so a test
# discovery failed to see would otherwise be silently dropped rather than run.
# A timeout is reported BEFORE the reconciliation and in the clock's own words.
# Whoever reads this output first has to be told which thing broke, and hundreds
# of "assigned but not run" lines describe the sharding rather than the budget
# they crossed.
if [[ -s "$TIMEDOUT" ]]; then
  while IFS='|' read -r _ rel; do
    echo "FAIL: $rel exceeded its ${IT_TIMEOUT} budget — go test killed it, so none of its tests reported"
  done < "$TIMEDOUT"
fi

LC_ALL=C sort -o "$RAN" "$RAN"
if ! diff "$ASSIGNED" "$RAN" > /dev/null; then
  DIVERGENCE="$(mktemp)"
  diff "$ASSIGNED" "$RAN" | sed -n 's/^< /  assigned but not run: /p; s/^> /  ran but not assigned: /p' > "$DIVERGENCE" || true
  # A timed-out package's every missing test has ONE cause, already named above.
  # Only ITS lines are dropped: a genuine discovery divergence in another package
  # of the same run has a different cause and must not be buried by this one.
  lane_drop_timed_out "$DIVERGENCE" "$TIMEDOUT"
  # Suppressed to empty means the timeout accounted for all of it, and the
  # header alone would be a finding with no evidence under it.
  if [[ -s "$DIVERGENCE" ]]; then
    if (( SHARD_TOTAL > 0 )); then
      echo "FAIL: shard ${SHARD_IDX}/${SHARD_TOTAL} ran a different test set than assigned:"
    else
      echo "FAIL: the lane ran a different test set than discovery assigned:"
    fi
    cat "$DIVERGENCE"
  fi
  rm -f "$DIVERGENCE"
  fail=1
fi

if grep -rq -- '--- SKIP' "$OUTDIR"; then
  echo "FAIL: integration tests must not skip — provision the env/service, do not skip:"
  grep -rh -- '--- SKIP' "$OUTDIR"
  fail=1
fi

# The manifests the fan-in reconciles: full discovery (identical across
# shards), this shard's slice, and what actually ran. Written before the
# verdict, so a red shard keeps them too: it is the one somebody reads.
if [[ -n "$SHARD_OUT" ]]; then
  cp "$DISCOVERY" "$SHARD_OUT/discovery.txt"
  cp "$ASSIGNED" "$SHARD_OUT/assigned.txt"
  cp "$RAN" "$SHARD_OUT/ran.txt"
  # "rel|seconds|tests|slot" per process (a split package has one row per
  # part), so cost can be tracked across runs rather than re-derived by eye
  # from a scrolled log.
  cp "$TIMING" "$SHARD_OUT/timing.txt"
  cp "$TEST_TIMING" "$SHARD_OUT/test-timing.txt"
  # The digest of the weights, so the fan-in names a split from other weights
  # instead of reporting it as a slicing bug.
  timing_digest="$(git hash-object --no-filters "$SHARD_TIMING")"
  printf 'shard=%s\ntotal=%s\ntiming=%s\n' "$SHARD_IDX" "$SHARD_TOTAL" "$timing_digest" > "$SHARD_OUT/meta.txt"
  # Each part's lists and its failed log, under the part's own name, and the
  # tests a continuation owes (scripts/lib-integration-artifacts.sh).
  # Guarded, so the verdict below still prints: the shard is red and says why.
  if ! keep_shard_evidence "$SHARD_OUT" "$WORK" "$REGEX_DIR" "$OUTDIR" "$FAILED_PKGS"; then
    echo "FAIL: the shard could not keep its part evidence in $SHARD_OUT"
    fail=1
  fi
fi

if [[ "$fail" -ne 0 ]]; then
  # Name the packages, and keep their logs. "see package logs above" pointed at
  # the interleaved stdout of every package in the lane — thousands of lines —
  # for a name this script already held, and the logs that held the assertion
  # went with the process. Between them, an intermittent failure cost a full
  # re-run to even locate, and the re-run is precisely what does not reproduce
  # it.
  if [[ -s "$FAILED_PKGS" ]]; then
    echo "FAIL: integration tests failed (parallel, $NPKGS packages):"
    while IFS='|' read -r d rel base; do
      printf '  %s (%s)\n' "$rel" "$d"
    done < "$FAILED_PKGS"
  else
    # A red run with no failing EXIT line: one of the reconciliation teeth above
    # tripped (a missing worker, a discovery/run divergence, a skip) rather than
    # a test. Say so instead of printing an empty list, which would read as the
    # naming having broken.
    echo "FAIL: integration tests failed (parallel, $NPKGS packages) — no package reported a failing exit; the reconciliation above names the reason"
  fi

  # The logs survive the exit from here on. Printed as a path per failing
  # package rather than a directory to grep: the point is to answer "what did it
  # assert" without a re-run.
  KEEP_LOGS=1
  echo "  logs kept (this run only, not cleaned up):"
  if [[ -s "$FAILED_PKGS" ]]; then
    while IFS='|' read -r d rel base; do
      printf '    %-44s %s\n' "$rel" "$OUTDIR/$base"
    done < "$FAILED_PKGS"
  fi
  echo "    all $ran package log(s): $OUTDIR"

  # In CI the runner is destroyed at the end of the job, so a kept path there is
  # a path to nothing. Copy the failing logs into the shard's artifact dir, which
  # the workflow uploads — the same evidence, by the route that outlives the
  # runner. Best effort: a failed copy must not change the verdict, which is
  # already red and already printed.
  if [[ -n "$SHARD_OUT" ]] && mkdir -p "$SHARD_OUT/failed-logs" 2>/dev/null; then
    # Written unconditionally on this path, even when no package failed, for two
    # reasons: it is the one file that says WHY the shard was red when the reason
    # was a reconciliation tooth rather than a test, and the upload step asserts
    # its artifact is non-empty — an empty dir would turn one honest red into two.
    {
      printf 'shard %s/%s failed\n' "${SHARD_IDX:-0}" "${SHARD_TOTAL:-0}"
      if [[ -s "$FAILED_PKGS" ]]; then
        printf 'failing packages:\n'
        cut -d'|' -f2 "$FAILED_PKGS" | sed 's/^/  /'
      else
        printf 'no package reported a failing exit; the reconciliation in the job log names the reason\n'
      fi
    } > "$SHARD_OUT/failure-summary.txt" 2>/dev/null || :
    # The failing parts' logs are already in failed-logs, one per part, and
    # rerun.txt beside them lists every test still owed.
    echo "    copied to the shard artifact: $SHARD_OUT/"
  fi
  exit 1
fi

# A shard saw a slice of the statements, so it hands its seen senders to the
# fan-in, which judges staleness over every shard. A whole lane judges it here.
check_repeated_gates "$MARGINCE_SQL_CENSUS_DIR" "$OUTDIR/repeated-gates.txt"
if [[ -n "$SHARD_OUT" ]]; then
  cp "$OUTDIR/repeated-gates.txt" "$SHARD_OUT/repeated-gates.txt"
else
  stale_repeated_gates "$OUTDIR/repeated-gates.txt"
fi

# Keep the exact success sentinel the gates grep for; the count is informational.
if (( SHARD_TOTAL > 0 )); then
  echo "OK: integration passed with 0 skips (shard ${SHARD_IDX}/${SHARD_TOTAL}: $(wc -l < "$ASSIGNED" | tr -d ' ') tests, $NPKGS packages, parallel)"
else
  echo "OK: integration passed with 0 skips ($NPKGS packages, parallel)"
fi
