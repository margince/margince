#!/usr/bin/env bash
# The timings report's own test. It reads `go test -v` output, and the two ways
# a reader is misled by a report like this are both under-counting:
#
#   1. Counting a subtest as a gate. A subtest's seconds are already inside its
#      parent, so a lane of 1100 gates would read as one of 2300 and the work
#      total would roughly double.
#   2. Missing a FAILING gate. The slowest gate on a red run is exactly the one
#      somebody is looking for, and a report that only reads PASS hides it.
#
# Both are asserted, with a fixture small enough to check by eye.
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat > "$tmp/run.log" <<'LOG'
=== RUN   TestSlow
--- PASS: TestSlow (9.00s)
    --- PASS: TestSlow/one (4.00s)
    --- PASS: TestSlow/two (5.00s)
=== RUN   TestBroken
--- FAIL: TestBroken (3.00s)
=== RUN   TestQuick
--- PASS: TestQuick (1.00s)
ok  	github.com/margince/margince/backend/gates	10.000s
LOG

out="$(GATE_TIMING_CORES=4 bash "$here/gate-timings.sh" "$tmp/run.log")"
fail() { echo "FAIL: $1"; echo "--- report:"; echo "$out"; exit 1; }

# Three gates, not five: the two subtests belong to TestSlow.
echo "$out" | grep -q "gates: 3, costing 13.0s of work" ||
	fail "a subtest was counted as a gate, or its seconds twice"

# The failing gate is in the costliest list, which is where it is wanted.
echo "$out" | grep -q "3.00s  TestBroken" ||
	fail "a failing gate is missing from the report"

# The slowest comes first.
echo "$out" | grep -A 1 "the costliest" | grep -q "TestSlow" ||
	fail "the list is not ordered by cost"

# Work over wall, from the package line rather than from a guess.
echo "$out" | grep -q "wall: 10.000s, so 1.3x of the work ran at once" ||
	fail "the parallel factor is not work over wall"

echo "OK: gate-timings.test.sh — 4 properties"
