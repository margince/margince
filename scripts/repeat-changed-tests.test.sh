#!/usr/bin/env bash
# The repeat job's own test, for the parts a wrong answer hides: a scope that
# misses a test file skips the job green, a log reader that misses a failure
# names no attempt, and a run outside the make profile fails every integration
# test before its body. backend/tools/changed-tests tests the plan.
#
# Usage: bash scripts/repeat-changed-tests.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd)/repeat-changed-tests.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

scope_of() { printf '%s\n' "$@" | GITHUB_OUTPUT="" bash "$SCRIPT" scope -; }

for path in backend/internal/thing/thing_test.go frontend/src/a.test.tsx frontend/scripts/b.spec.ts; do
  [[ "$(scope_of README.md "$path")" == *"run=true"* ]] || fail "a diff with $path skipped the job"
done
for path in backend/internal/thing/thing.go frontend/src/a.tsx docs/x.md extensions/unit/thing_test.go; do
  [[ "$(scope_of "$path")" == *"run=false"* ]] || fail "a diff with only $path ran the job"
done

# Two rounds of a parallel test and a subtest; TestFlaky fails on round two.
cat >"$TMP/go.log" <<'EOF'
=== RUN   TestFlaky
=== PAUSE TestFlaky
=== RUN   TestSteady
=== RUN   TestSteady/case
--- PASS: TestSteady (0.00s)
    --- PASS: TestSteady/case (0.00s)
=== CONT  TestFlaky
--- PASS: TestFlaky (0.00s)
=== RUN   TestFlaky
=== PAUSE TestFlaky
=== RUN   TestSteady
=== RUN   TestSteady/case
    --- FAIL: TestSteady/case (0.00s)
--- FAIL: TestSteady (0.00s)
=== CONT  TestFlaky
--- FAIL: TestFlaky (0.00s)
FAIL
EOF
named="$(bash "$SCRIPT" attempts 20 "$TMP/go.log")"
want="FAIL: TestSteady on attempt 2 of 20
FAIL: TestFlaky on attempt 2 of 20"
[[ "$named" == "$want" ]] || fail "the log reader named '$named', want '$want'"

# The template holds the examples' ledgers; a composition without them makes
# every clone refuse its test, so the run stops before it composes.
printf 'go-unit\t3\tbackend/internal/thing\t^(TestThing)$\n' >"$TMP/plan.tsv"
status=0
outside="$(env -u COMPOSITION_FLAGS bash "$SCRIPT" run "$TMP/plan.tsv" origin/main 2>&1)" || status=$?
[[ "$status" -eq 2 && "$outside" == *"make repeat-changed-tests"* ]] ||
  fail "a run outside the make profile went on (status $status): $outside"

mkdir "$TMP/bin"
printf '#!/bin/sh\nexit 0\n' >"$TMP/bin/pnpm"
chmod +x "$TMP/bin/pnpm"
printf 'vitest\t1\tsrc/example.test.ts\t\n' >"$TMP/vitest.tsv"
for base in origin/main origin/develop; do
  replay="$(PATH="$TMP/bin:$PATH" TEST_SHUFFLE_SEED=22413 \
    bash "$SCRIPT" run "$TMP/vitest.tsv" "$base")"
  [[ "$replay" == *"replay: make repeat-changed-tests REPEAT_BASE=$base TEST_SHUFFLE_SEED=22413"* ]] ||
    fail "the replay dropped the base $base or the seed: $replay"
done

if ((FAILURES > 0)); then
  echo "repeat-changed-tests.test: $FAILURES failure(s)" >&2
  exit 1
fi
echo "OK: repeat-changed-tests scope and attempt names"
