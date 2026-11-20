#!/usr/bin/env bash
# Machine-wide slots for the full gate runs, so that several worktrees do not
# each take every core and gigabytes of memory at once.
#
# Usage:
#   gate-slot.sh acquire <target> [holder-pid]
#       Wait for a free slot and record holder-pid (default: the caller's parent,
#       which is the make that runs the recipe) as its holder. Nothing releases it:
#       the slot is free as soon as that process is gone, and the next waiter
#       reclaims it.
#   gate-slot.sh run <target> -- <command...>
#       Hold a slot for the command and release it on exit, failure or interrupt.
#
# A run takes no slot when CI is set, when GATE_SLOTS is 0 or "off", or when it
# runs under a process that already holds one (a nested make, or GATE_SLOT_HELD).
#
# Env:
#   GATE_SLOTS             slots on this machine (default 2; 0 or "off" disables)
#   GATE_SLOT_DIR          lock directory (default $XDG_CACHE_HOME/margince/gate-slots)
#   GATE_SLOT_POLL         seconds between attempts (default 2)
#   GATE_SLOT_REPORT_EVERY seconds between "waiting" reports (default 30)
set -euo pipefail

SLOTS="${GATE_SLOTS:-2}"
DIR="${GATE_SLOT_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/margince/gate-slots}"
POLL="${GATE_SLOT_POLL:-2}"
REPORT_EVERY="${GATE_SLOT_REPORT_EVERY:-30}"

start_stamp() { ps -o lstart= -p "$1" 2>/dev/null | sed 's/^ *//' || true; }

# A holder is alive when its pid exists and still started when it was recorded,
# so a recycled pid does not keep a slot.
holder_alive() {
	local file="$1" pid stamp
	pid="$(sed -n 1p "$file" 2>/dev/null || true)"
	stamp="$(sed -n 2p "$file" 2>/dev/null || true)"
	[[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null && [[ "$(start_stamp "$pid")" == "$stamp" ]]
}

file_age() {
	local m
	m="$(stat -f %m "$1" 2>/dev/null || stat -c %Y "$1" 2>/dev/null || echo 0)"
	echo $(($(date +%s) - m))
}

describe_holders() {
	local d f
	for d in "$DIR"/slot-*; do
		[[ -d "$d" ]] || continue
		f="$d/holder"
		if [[ -f "$f" ]] && holder_alive "$f"; then
			echo "  $(basename "$d"): pid $(sed -n 1p "$f"), $(sed -n 3p "$f"), $(sed -n 4p "$f")"
		fi
	done
}

# Run by an ancestor that holds a slot: its sub-makes must not queue behind it.
ancestor_holds() {
	local pid="$PPID" d
	[[ -n "${GATE_SLOT_HELD:-}" ]] && return 0
	while [[ -n "$pid" && "$pid" -gt 1 ]]; do
		for d in "$DIR"/slot-*; do
			[[ -f "$d/holder" && "$(sed -n 1p "$d/holder" 2>/dev/null)" == "$pid" ]] && return 0
		done
		pid="$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ' || true)"
	done
	return 1
}

# Moving the directory away first keeps two waiters from both deleting a slot
# another waiter has just taken.
reclaim_if_dead() {
	local d="$1" gone="$1.gone.$$"
	if [[ -f "$d/holder" ]]; then
		holder_alive "$d/holder" && return 1
	else
		[[ "$(file_age "$d")" -gt 5 ]] || return 1
	fi
	mv "$d" "$gone" 2>/dev/null || return 1
	if [[ -f "$gone/holder" ]] && holder_alive "$gone/holder"; then
		mv "$gone" "$d" 2>/dev/null || true
		return 1
	fi
	rm -rf "$gone"
}

try_take() {
	local pid="$1" target="$2" i d
	for ((i = 1; i <= SLOTS; i++)); do
		d="$DIR/slot-$i"
		[[ -d "$d" ]] && reclaim_if_dead "$d" || true
		if mkdir "$d" 2>/dev/null; then
			printf '%s\n%s\n%s\n%s\n' "$pid" "$(start_stamp "$pid")" "worktree $PWD" "target $target" >"$d/holder"
			TAKEN="$d"
			return 0
		fi
	done
	return 1
}

acquire() {
	local target="$1" pid="$2" waited=0 since_report=0
	TAKEN=""
	mkdir -p "$DIR"
	while ! try_take "$pid" "$target"; do
		if [[ "$since_report" -le 0 ]]; then
			echo "gate-slot: $target waits, all $SLOTS gate slots are taken (GATE_SLOTS=0 skips the wait):" >&2
			describe_holders >&2
			since_report="$REPORT_EVERY"
		fi
		sleep "$POLL"
		waited=$((waited + POLL))
		since_report=$((since_report - POLL))
	done
	[[ "$waited" -eq 0 ]] || echo "gate-slot: $target got $(basename "$TAKEN") after ${waited}s" >&2
}

skip() {
	[[ -n "${CI:-}" ]] && return 0
	[[ "$SLOTS" == "0" || "$SLOTS" == "off" ]] && return 0
	ancestor_holds
}

mode="${1:-}"
case "$mode" in
acquire)
	target="${2:?target}"
	skip && exit 0
	acquire "$target" "${3:-$PPID}"
	;;
run)
	target="${2:?target}"
	shift 2
	[[ "${1:-}" == "--" ]] && shift
	if skip; then exec "$@"; fi
	acquire "$target" "$$"
	trap 'rm -rf "$TAKEN"' EXIT
	trap 'exit 130' INT
	trap 'exit 143' TERM
	GATE_SLOT_HELD=1 "$@"
	;;
*)
	echo "usage: gate-slot.sh acquire <target> [pid] | run <target> -- <command...>" >&2
	exit 2
	;;
esac
