#!/usr/bin/env bash
# Where the gates lane spent its time, gate by gate.
#
# The lane takes about a minute on a developer machine and reached the
# ten-minute CI default, and those two numbers on their own invite a guess.
# This answers what the gap actually asks: the gates already run in parallel,
# so a run costs the SUM of their work divided by the cores it is given. A
# reading of one machine says nothing about the other until both are in front
# of you, which is why this prints them together.
#
# Reads `go test -v` output, so it measures the run a lane really did rather
# than a second one arranged to be measured.
#
#   make gate-timings            run the lane and report
#   gate-timings.sh run.log      report a run already captured
set -eu

log="${1:--}"
cores="${GATE_TIMING_CORES:-1}"

# Top-level results only. A subtest is indented and its seconds are already
# inside its parent, so counting both reports the tree twice.
tops() { grep -E "^--- (PASS|FAIL): " "$log"; }

tops | awk -v cores="$cores" -v log="$log" '
{
	secs = $4
	gsub(/[()s]/, "", secs)
	total += secs
	n += 1
}
END {
	if (n == 0) { print "gate-timings: no test timings in that output" > "/dev/stderr"; exit 1 }
	printf "gates: %d, costing %.1fs of work\n", n, total
	printf "cores: %s\n", cores
	printf "two cores owe about %.0fs of this, before any build\n", total / 2
}'

wall=$(grep -oE "^(ok|FAIL)[[:space:]]+[^[:space:]]+[[:space:]]+[0-9.]+s$" "$log" | tail -1 | awk '{print $NF}' | tr -d 's' || true)
if [ -n "${wall:-}" ]; then
	tops | awk -v wall="$wall" '{s = $4; gsub(/[()s]/, "", s); t += s} END {
		printf "wall: %ss, so %.1fx of the work ran at once\n", wall, t / wall
	}'
fi

echo
echo "the costliest, which is where a minute saved comes from:"
tops | sed -E 's/^--- (PASS|FAIL): ([A-Za-z0-9_]+) \(([0-9.]+)s\)/\3 \2/' |
	sort -rn | head -15 | awk '{printf "%9.2fs  %s\n", $1, $2}'
