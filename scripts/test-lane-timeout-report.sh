#!/usr/bin/env bash
# test-lane-timeout-report.sh — prove a package killed for exceeding its budget
# is reported as a TIMEOUT, and that it does not bury a real divergence.
#
# The failure this replaces: when a package hit `go test -timeout`, every one of
# its assigned tests was missing from the run, so the reconciliation printed
# hundreds of "assigned but not run" lines. The reconciliation was working
# exactly as designed — those tests genuinely did not run — but it was the
# loudest thing in the output and it described the sharding mechanism rather
# than the clock. Whoever hit it first debugged the wrong thing.
#
# The lane's own path cannot be exercised here: reproducing it for real means
# waiting out a 600s budget. So the two behaviours are pinned against the
# script's SOURCE, which is where a future edit would undo them.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
lane="$root/scripts/test-integration-parallel.sh"
failures=0

check() { # description; reads $? style via caller
    if [ "$1" = "yes" ]; then
        printf '  ok   %s\n' "$2"
    else
        printf '  FAIL %s\n' "$2" >&2
        failures=$((failures + 1))
    fi
}
# The two behaviours are exercised for REAL, against fabricated logs: the lane
# itself provisions databases and waits out a 600s budget, so it cannot be run
# here, but the decisions it makes are functions in lib-testdb.sh and those can.
# shellcheck source=scripts/lib-testdb.sh
source "$root/scripts/lib-testdb.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "a package over its budget is reported as a timeout, not as a discovery failure"

# go test's OWN two spellings, each in the shape it actually prints.
printf 'panic: test timed out after 10m0s\n\ngoroutine 1:\n' > "$tmp/panic.log"
check "$(lane_timed_out "$tmp/panic.log" && echo yes || echo no)" \
    "the timeout panic is read as a timeout"

printf 'ok  \tpkg\t1.0s\n*** Test killed with quit: ran too long (10m0s).\n' > "$tmp/killed.log"
check "$(lane_timed_out "$tmp/killed.log" && echo yes || echo no)" \
    "the -timeout kill line is read as a timeout"

# NOT inferred from an absent test count. A package that died for any other
# reason is also missing its tests, and it must still face the reconciliation
# rather than be excused by a guess.
printf 'FAIL\tpkg [build failed]\nEXIT 2\n' > "$tmp/build.log"
check "$(lane_timed_out "$tmp/build.log" && echo no || echo yes)" \
    "a package that died some other way is NOT excused as a timeout"

# And a TEST that prints the marker is not go test printing it. These very
# fixtures quote the string, so an unanchored match would classify this
# harness's own package as timed out — the same wrong diagnosis one level down.
printf '    lane_test.go:42: want the log to carry *** Test killed with quit\n--- FAIL: TestX\n' > "$tmp/quoted.log"
check "$(lane_timed_out "$tmp/quoted.log" && echo no || echo yes)" \
    "a test QUOTING the kill marker mid-line is not read as a timeout"

# The timeout is named in the clock's own words, before the reconciliation.
# Order is the whole point: the first thing printed is what the reader debugs.
check "$(grep -qF 'exceeded its ${IT_TIMEOUT} budget' "$lane" && echo yes || echo no)" \
    "the timeout names the budget it crossed"
timeout_line="$(grep -n 'exceeded its ${IT_TIMEOUT} budget' "$lane" | head -1 | cut -d: -f1)"
diverge_line="$(grep -n 'ran a different test set' "$lane" | head -1 | cut -d: -f1)"
check "$([ -n "$timeout_line" ] && [ -n "$diverge_line" ] && [ "$timeout_line" -lt "$diverge_line" ] && echo yes || echo no)" \
    "the timeout is reported before the reconciliation diff"

echo "and it does not bury a divergence it did not cause"

# One timed-out package, one genuinely divergent one, in the same run.
cat > "$tmp/divergence" <<'EOF'
  assigned but not run: backend|./internal/compose/integration|TestABlindCopiedSend
  assigned but not run: backend|./internal/compose/integration|TestAccountTimeline
  assigned but not run: backend|./internal/modules/people|TestSomethingElseEntirely
  ran but not assigned: backend|./internal/modules/people|TestAppearedFromNowhere
EOF
printf 'backend|./internal/compose/integration\n' > "$tmp/timedout"
lane_drop_timed_out "$tmp/divergence" "$tmp/timedout"

check "$(grep -qF 'compose/integration' "$tmp/divergence" && echo no || echo yes)" \
    "the timed-out package's lines are gone — they all had one cause, already named"
check "$(grep -qF 'TestSomethingElseEntirely' "$tmp/divergence" && echo yes || echo no)" \
    "another package's assigned-but-not-run survives"
check "$(grep -qF 'TestAppearedFromNowhere' "$tmp/divergence" && echo yes || echo no)" \
    "and so does a ran-but-not-assigned somewhere else"

# The DIRECTION matters as much as the package. A timeout explains tests that
# were assigned and did not run, and nothing else — a killed package may have
# run some before it died, and one of those turning up unassigned is a
# divergence the timeout does not account for.
cat > "$tmp/direction" <<'EOF'
  assigned but not run: backend|./internal/compose/integration|TestKilledWithTheRest
  ran but not assigned: backend|./internal/compose/integration|TestRanBeforeTheKill
EOF
lane_drop_timed_out "$tmp/direction" "$tmp/timedout"
check "$(grep -qF 'TestKilledWithTheRest' "$tmp/direction" && echo no || echo yes)" \
    "the timed-out package's assigned-but-not-run goes"
check "$(grep -qF 'TestRanBeforeTheKill' "$tmp/direction" && echo yes || echo no)" \
    "but its ran-but-not-assigned stays — the timeout does not explain that one"

# Suppressed to empty means the timeout accounted for all of it, and a header
# with no evidence under it is a finding that says nothing.
cat > "$tmp/only" <<'EOF'
  assigned but not run: backend|./internal/compose/integration|TestOne
EOF
lane_drop_timed_out "$tmp/only" "$tmp/timedout"
check "$([ -s "$tmp/only" ] && echo no || echo yes)" \
    "a diff explained entirely by the timeout is emptied"
check "$(grep -qF 'if [[ -s "$DIVERGENCE" ]]; then' "$lane" && echo yes || echo no)" \
    "and the lane prints the header only when something survives"

# Nothing timed out: the diff is untouched.
: > "$tmp/none"
cp "$tmp/divergence" "$tmp/untouched"
lane_drop_timed_out "$tmp/untouched" "$tmp/none"
check "$(diff -q "$tmp/divergence" "$tmp/untouched" >/dev/null && echo yes || echo no)" \
    "a run with no timeout has its diff left alone"

echo "a package approaching its budget says so while it is still passing"

check "$(grep -qF 'split it before it crosses' "$lane" && echo yes || echo no)" \
    "the advisory report warns before a package becomes unpassable"

if [ "$failures" -ne 0 ]; then
    echo "FAIL: $failures check(s) failed" >&2
    exit 1
fi
echo "OK: the lane names the clock when the clock is what broke"
