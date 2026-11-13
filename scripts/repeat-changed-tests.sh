#!/usr/bin/env bash
#
# repeat-changed-tests.sh — run each test a change adds or changes many times,
# so a test that fails at random fails the pull request that brings it, and not
# the next unrelated one.
#
#   scripts/repeat-changed-tests.sh scope BASE_REF       whether the diff touches a test file
#   scripts/repeat-changed-tests.sh scope - < paths      the same, over the changed paths
#   scripts/repeat-changed-tests.sh plan BASE_REF FILE   write the plan to FILE
#   scripts/repeat-changed-tests.sh run FILE             run the plan, under `make repeat-changed-tests`
#   scripts/repeat-changed-tests.sh attempts COUNT LOG   name the attempt of each failure in a `go test -v` log
#
# backend/tools/changed-tests picks the tests and the repeat count of each. The
# run starts the Go unit, Go integration and vitest lanes at the same time, so
# each lane runs on a loaded machine. Go repeats in one process (`-count`);
# vitest has no report that names a failed repeat, so it runs the files once per
# attempt. The integration lane runs through `make test-it`, and needs
# `make db-up` and `make test-db-up` first. Run composes with the flags the
# make profile (scripts/composition-profile.mk) gives `make test-db-up`, so the
# test manifest names every extension ledger the template holds.
#
# With GITHUB_OUTPUT set, scope writes run=true|false, and plan writes which
# lanes the plan holds (go, integration, vitest).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# is_test_file is the one list of what the plan can select from: Go test files
# of the backend module, and vitest files of the frontend.
is_test_file() {
  case "$1" in
    backend/*_test.go | frontend/*.test.* | frontend/*.spec.*) return 0 ;;
  esac
  return 1
}

output() {
  echo "repeat-changed-tests: $1=$2"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "$1=$2" >>"$GITHUB_OUTPUT"
  fi
}

changed_test_files() {
  local base="$1" path
  while IFS= read -r path; do
    if is_test_file "$path"; then
      echo "$path"
    fi
  done < <(if [[ "$base" == "-" ]]; then cat; else git -C "$ROOT" diff --name-only --diff-filter=d "$base"...HEAD; fi)
}

scope() {
  local files
  files="$(changed_test_files "$1")"
  if [[ -z "$files" ]]; then
    output run false
  else
    output run true
  fi
}

plan() {
  local base="$1" out="$2" files=()
  mapfile -t files < <(changed_test_files "$base")
  : >"$out"
  if ((${#files[@]} > 0)); then
    git -C "$ROOT" diff -U0 --no-color --no-ext-diff "$base"...HEAD -- "${files[@]}" |
      (cd "$ROOT/backend" && GOWORK="$ROOT/go.work" go run ./tools/changed-tests \
        -root "$ROOT" -timing "$ROOT/scripts/integration-test-timing.txt") >"$out"
  fi
  cat "$out"
  output go "$(grep -q '^go-unit' "$out" && echo true || echo false)"
  output integration "$(grep -q '^go-integration' "$out" && echo true || echo false)"
  output vitest "$(grep -q '^vitest' "$out" && echo true || echo false)"
}

# attempts prints one line per failed top-level test, with the attempt it
# failed on. With -count, go test runs every test once before the next round,
# so the attempt is the number of times the test had started.
attempts() {
  local count="$1" log="$2"
  awk -v count="$count" '
    /^=== RUN   / && $3 !~ /\// { started[$3]++ }
    /^--- FAIL: / && $3 !~ /\// { print "FAIL: " $3 " on attempt " started[$3] " of " count }
  ' "$log"
}

# report names what failed in one package's log, then prints the log.
report() {
  local lane="$1" count="$2" target="$3" log="$4" named
  named="$(attempts "$count" "$log")"
  echo "::group::$lane $target failed — its log"
  cat "$log"
  echo "::endgroup::"
  echo "$lane $target: ${named:-FAIL: no test failed on its own; the package did not build, panicked or timed out (log above)}"
}

# counted holds a green package to the runs it was asked for: a pattern that
# matches no test passes `go test` without running anything.
counted() {
  local lane="$1" count="$2" dir="$3" pattern="$4" log="$5" names passed
  names="$(awk -F'|' '{ print NF }' <<<"$pattern")"
  passed="$(grep -cE '^--- (PASS|SKIP): ' "$log" || true)"
  if ((passed != names * count)); then
    echo "$lane: FAIL: $dir passed $passed runs, want $names tests times $count"
    return 1
  fi
  echo "$lane: $dir passed all $count attempts of $names tests"
}

go_lane() {
  local which="$1" plan="$2" logs="$3" failed=0 lane count dir pattern log status
  while IFS=$'\t' read -r lane count dir pattern; do
    [[ "$lane" == "$which" ]] || continue
    log="$logs/${dir//\//_}.$lane.log"
    echo "$lane: $dir, $count times: $pattern"
    status=0
    if [[ "$lane" == go-unit ]]; then
      local pkg="./${dir#backend/}"
      [[ "$dir" != backend ]] || pkg=.
      # The composed workspace the unit lane tests in, as backend/Makefile's GOWORK_COMPOSED.
      (cd "$ROOT/backend" && GOWORK="$ROOT/build/composition/go.work" \
        go test -v -count="$count" -timeout=30m -run "$pattern" "$pkg") >"$log" 2>&1 || status=$?
    else
      # A flag after the package wins over the -count=1 the script names before it.
      IT_ARGS="-count=$count" INTEGRATION_TIMEOUT="${INTEGRATION_TIMEOUT:-1800s}" CHECK_WRITERS_RAN=1 \
        make -C "$ROOT/backend" --no-print-directory test-it DIR="$dir" RUN="$pattern" >"$log" 2>&1 || status=$?
    fi
    if ((status != 0)); then
      report "$lane" "$count" "$dir" "$log"
      failed=1
    elif ! counted "$lane" "$count" "$dir" "$pattern" "$log"; then
      failed=1
    fi
  done <"$plan"
  ((failed == 0))
}

vitest_lane() {
  local plan="$1" logs="$2" count files=() failed=0 attempt log
  count="$(awk -F'\t' '$1 == "vitest" { print $2; exit }' "$plan")"
  mapfile -t files < <(awk -F'\t' '$1 == "vitest" { print $3 }' "$plan")
  ((${#files[@]} > 0)) || return 0
  echo "vitest: ${#files[@]} files, $count attempts: ${files[*]}"
  for attempt in $(seq 1 "$count"); do
    log="$logs/vitest.$attempt.log"
    # No colour: CI forces it on, and its codes hide the ` FAIL ` the grep below reads.
    if (cd "$ROOT/frontend" && NO_COLOR=1 FORCE_COLOR=0 pnpm exec vitest run "${files[@]}") >"$log" 2>&1; then
      continue
    fi
    failed=$((failed + 1))
    echo "::group::vitest attempt $attempt of $count failed — its log"
    cat "$log"
    echo "::endgroup::"
    grep -E '^ FAIL ' "$log" | sed "s/^ FAIL /vitest: FAIL on attempt $attempt of $count: /" || true
  done
  if ((failed > 0)); then
    echo "vitest: $failed of $count attempts failed"
    return 1
  fi
  echo "vitest: passed all $count attempts"
}

run() {
  local plan="$1" logs verdict=0 pids=()
  if [[ ! -s "$plan" ]]; then
    echo "repeat-changed-tests: the plan is empty, so no test to repeat"
    return 0
  fi
  if grep -q '^go-' "$plan" && [[ -z "${COMPOSITION_FLAGS+set}" ]]; then
    echo "repeat-changed-tests: run the plan through \`make repeat-changed-tests\`; without its composition profile the lanes compose without the examples the template holds" >&2
    return 2
  fi
  logs="$(mktemp -d)"
  # The lanes below only read build/composition/; one write first keeps a
  # lane from reading it while another rewrites it.
  if grep -q '^go-' "$plan"; then
    make -C "$ROOT/backend" --no-print-directory composition
  fi
  go_lane go-unit "$plan" "$logs" &
  pids+=($!)
  go_lane go-integration "$plan" "$logs" &
  pids+=($!)
  vitest_lane "$plan" "$logs" &
  pids+=($!)
  for pid in "${pids[@]}"; do
    wait "$pid" || verdict=1
  done
  rm -rf "$logs"
  if [[ "$verdict" -ne 0 ]]; then
    echo "repeat-changed-tests: a changed test failed at least once; the lines starting FAIL name it and its attempt"
  fi
  return "$verdict"
}

case "${1:-}" in
  scope) scope "${2:?usage: $0 scope BASE_REF|-}" ;;
  plan) plan "${2:?usage: $0 plan BASE_REF FILE}" "${3:?usage: $0 plan BASE_REF FILE}" ;;
  run) run "${2:?usage: $0 run FILE}" ;;
  attempts) attempts "${2:?usage: $0 attempts COUNT LOG}" "${3:?usage: $0 attempts COUNT LOG}" ;;
  *)
    echo "usage: $0 scope|plan|run|attempts ..." >&2
    exit 2
    ;;
esac
