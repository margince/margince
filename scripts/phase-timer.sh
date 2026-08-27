#!/usr/bin/env bash
# Where `make check` spent its time, recorded phase by phase.
#
# The numbers in a comment go stale the first time somebody changes a gate, and
# nobody re-measures a lane that is merely slow. So the gate measures itself and
# says so on every green run: the next person to optimize it starts from a
# reading of THEIR machine rather than from a sentence written on someone else's.
#
# start <label> stamps the clock; stop closes the open phase; report prints the
# table and clears it. A phase that FAILS is never closed — make stops at the
# failing line — so the table describes a run that finished, which is the run
# you can compare against another one.
#
# The ledger is per worktree (.tmp is gitignored and local to the checkout), so
# two worktrees checking at once do not write over each other.
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
ledger="$root/.tmp/check-timings.tsv"
open="$root/.tmp/check-timings.open"

case "${1:-}" in
reset)
	# Called once, by the target that owns the whole run. A phase recorded by an
	# earlier partial run — `make check-go`, or a run that failed halfway — is
	# still on disk, and printing it inside this run's table would attribute
	# someone else's seconds to this one.
	mkdir -p "$(dirname "$ledger")"
	rm -f "$ledger" "$open"
	;;
start)
	label="${2:?start needs a label}"
	mkdir -p "$(dirname "$ledger")"
	# Phases are a flat sequence, never a nest. A phase opened inside another
	# would overwrite it here and be double-counted in the table — the outer
	# phase's seconds are already the sum of the inner ones. Refusing at the
	# point of nesting names the recipe that did it; without this the run dies
	# much later, at the outer `stop`, pointing at the wrong line.
	if [ -f "$open" ]; then
		echo "phase-timer: \"$label\" starts inside \"$(cut -f2- "$open")\", which is still open —" >&2
		echo "             phases do not nest; a sub-make's own phases replace its caller's, not sit inside it" >&2
		exit 1
	fi
	printf '%s\t%s\n' "$(date +%s)" "$label" > "$open"
	;;
stop)
	# No open phase means `start` never ran — a recipe edited into a shape this
	# script no longer brackets. Say so rather than recording a phase of zero,
	# which would read as a phase that cost nothing.
	if [ ! -f "$open" ]; then
		echo "phase-timer: stop with no open phase — a start line is missing from the recipe" >&2
		exit 1
	fi
	started=$(cut -f1 "$open")
	label=$(cut -f2- "$open")
	printf '%s\t%s\n' "$(( $(date +%s) - started ))" "$label" >> "$ledger"
	rm -f "$open"
	;;
report)
	[ -f "$ledger" ] || exit 0
	total=$(awk -F'\t' '{s += $1} END {print s+0}' "$ledger")
	echo ""
	echo "  where the time went ($(printf '%dm%02ds' $((total / 60)) $((total % 60))) total)"
	awk -F'\t' -v total="$total" '
		{ pct = total > 0 ? ($1 * 100 / total) : 0
		  printf "  %5ds  %5.1f%%  %s\n", $1, pct, $2 }
	' "$ledger"
	echo ""
	rm -f "$ledger"
	;;
*)
	echo "usage: phase-timer.sh reset | start <label> | stop | report" >&2
	exit 2
	;;
esac
