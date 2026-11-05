#!/usr/bin/env bash
# Where `make check` spent its time, recorded phase by phase.
#
# The numbers in a comment go stale the first time somebody changes a gate, and
# nobody re-measures a lane that is merely slow. So the gate measures itself and
# says so on every green run: whoever optimizes it next starts from a
# reading of THEIR machine rather than from a sentence written on someone else's.
#
# reset stamps the start of the run; start <label> opens a phase; stop closes
# the open phase; report prints the table and clears it. A phase that FAILS is
# never closed (make stops at the failing line), so the table describes a run
# that finished, which is the run you can compare against another one.
#
# Phases in one lane are a flat sequence. Lanes run side by side: PHASE_LANE
# names the lane a phase belongs to, and each lane has its own open phase. The
# report gives the wall time of the run next to the sum of its phases, because
# with lanes that overlap the sum is larger than the time the run took.
#
# The ledger is per worktree (.tmp is gitignored and local to the checkout), so
# two worktrees checking at once do not write over each other.
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
ledger_dir="${PHASE_TIMER_DIR:-$root/.tmp}"
ledger="$ledger_dir/check-timings.tsv"
started_at="$ledger_dir/check-timings.start"
lane="${PHASE_LANE:-main}"
case "$lane" in
*[!A-Za-z0-9_-]* | "")
	echo "phase-timer: PHASE_LANE \"$lane\" is not a plain name (letters, digits, - and _)" >&2
	exit 2
	;;
esac
open="$ledger_dir/check-timings.open.$lane"

duration() {
	printf '%dm%02ds' $(($1 / 60)) $(($1 % 60))
}

case "${1:-}" in
reset)
	# Called once, by the target that owns the whole run. A phase recorded by an
	# earlier partial run — `make check-go`, or a run that failed halfway — is
	# still on disk, and printing it inside this run's table would attribute
	# someone else's seconds to this one.
	mkdir -p "$ledger_dir"
	rm -f "$ledger" "$ledger_dir"/check-timings.open.*
	date +%s >"$started_at"
	;;
start)
	label="${2:?start needs a label}"
	mkdir -p "$ledger_dir"
	# Phases in one lane do not nest. A phase opened inside another would
	# overwrite it here and be counted twice in the table, since the outer
	# phase's seconds are already the sum of the inner ones. Refusing at the
	# point of nesting names the recipe that did it; without this the run dies
	# much later, at the outer `stop`, pointing at the wrong line.
	if [[ -f "$open" ]]; then
		echo "phase-timer: \"$label\" starts inside \"$(cut -f2- "$open")\", which is still open in lane \"$lane\" —" >&2
		echo "             phases in one lane do not nest; a sub-make's own phases replace its caller's, not sit inside it" >&2
		exit 1
	fi
	printf '%s\t%s\n' "$(date +%s)" "$label" >"$open"
	;;
stop)
	# No open phase means `start` never ran — a recipe edited into a shape this
	# script no longer brackets. Say so rather than recording a phase of zero,
	# which would read as a phase that cost nothing.
	if [[ ! -f "$open" ]]; then
		echo "phase-timer: stop with no open phase in lane \"$lane\" — a start line is missing from the recipe" >&2
		exit 1
	fi
	started=$(cut -f1 "$open")
	label=$(cut -f2- "$open")
	# One short line per append, so lanes that stop at the same moment do not
	# mix their rows.
	printf '%s\t%s\t%s\n' "$(($(date +%s) - started))" "$lane" "$label" >>"$ledger"
	rm -f "$open"
	;;
report)
	[[ -f "$ledger" ]] || exit 0
	total=$(awk -F'\t' '{s += $1} END {print s+0}' "$ledger")
	echo ""
	if [[ -f "$started_at" ]]; then
		wall=$(($(date +%s) - $(cat "$started_at")))
		echo "  where the time went ($(duration "$wall") wall, $(duration "$total") summed over the phases)"
	else
		echo "  where the time went ($(duration "$total") total)"
	fi
	awk -F'\t' -v total="$total" '
		{ pct = total > 0 ? ($1 * 100 / total) : 0
		  printf "  %5ds  %5.1f%%  %-8s  %s\n", $1, pct, $2, $3 }
	' "$ledger"
	echo ""
	rm -f "$ledger" "$started_at"
	;;
*)
	echo "usage: [PHASE_LANE=<lane>] phase-timer.sh reset | start <label> | stop | report" >&2
	exit 2
	;;
esac
