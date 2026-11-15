#!/usr/bin/env bash
# The machine applier: move this host's wall clock, prove it moved, put it back.
#
# Why the host's clock and not a fake one: Go reads the wall clock through the
# vDSO, so no LD_PRELOAD shim reaches time.Now(), and Linux has no
# CLOCK_REALTIME namespace, so a container cannot hold a wall clock of its own.
# Moving the host's is the only shift that reaches the Go test process and the
# compose Postgres as one clock. Several hundred test files compare against
# Postgres' now(); a shift reaching only Go would fail them all, and those reds
# would be the lane's fault rather than the tree's.
#
# Not for a developer machine: a 200-day jump expires its credentials and
# certificates. This is for a disposable CI runner.
set -euo pipefail
cd "$(dirname "$0")/.."

# Where the real instant is parked while the clock is moved. Under RUNNER_TEMP
# when the job has one: that directory is per-job, where a fixed path under /tmp
# is guessable by anything else sharing a self-hosted runner, and a planted
# baseline would make `assert` pass over a clock that never moved.
BASELINE="${CLOCK_DRIFT_BASELINE:-${RUNNER_TEMP:-${TMPDIR:-/tmp}}/clock-drift-baseline}"

# How far short of the full offset the clock may read before this calls it
# unshifted. Generous, because it only ever has to separate "moved 200 days"
# from "did not move at all".
SHORTFALL_TOLERANCE_SECONDS=600

usage() {
	echo "usage: $0 move <days> | assert <days> | restore <days> | run <days>" >&2
	exit 2
}

# days validates the offset before it reaches arithmetic.
#
# Bash evaluates a variable's VALUE recursively inside $(( )), so a non-numeric
# argument is both a confusing failure and a way to smuggle an expression in.
#
# A LEADING ZERO is rejected rather than normalised, and it is the case with
# teeth: `08` is octal to the shell and aborts the arithmetic, but `move` has
# already jumped the clock by then — so the run ends with the machine shifted,
# time sync off, and a `restore 08` that dies the same way. Bare `0` goes with
# it: it makes the wanted offset nothing, which the shortfall check then reads
# as satisfied by a clock that never moved.
days() {
	case "$1" in
	'' | 0 | *[!0-9]* | 0[0-9]*)
		echo "clock-drift-host: $1 is not a positive number of days without a leading zero" >&2
		exit 2
		;;
	esac
	echo "$1"
}

# shortfall is how far short of the full offset the clock currently reads.
#
# Negative means it reads FURTHER out than the offset, which is the ordinary
# case after the suites have run: the moved clock keeps ticking, so an hour of
# testing puts it an hour past where the jump left it.
shortfall() {
	local want drift
	want=$(($1 * 86400))
	drift=$(($(date -u +%s) - $(cat "$BASELINE")))
	echo $((want - drift))
}

# move parks the real instant and jumps the clock forward.
#
# Time sync goes off FIRST. A runner whose NTP client is still running snaps the
# clock back mid-suite, and the half of the run after the snap is an ordinary
# run reporting under the drift lane's name — a green that means nothing.
move() {
	local days real
	days=$(days "$1")
	real=$(date -u +%s)
	# Record recovery before mutation: an interrupted move must still restore.
	printf '%s\n' "$real" >"$BASELINE"
	sudo timedatectl set-ntp false
	sudo date -s "+${days} days" >/dev/null
	assert "$days"
}

# assert proves the clock is still the moved one, before the suites and after.
#
# One-sided on purpose: the baseline is frozen at move time while the moved
# clock keeps ticking, so after an hour of suites the drift is the offset plus
# an hour. Only a shortfall says something put the clock back.
assert() {
	local days short
	days=$(days "$1")
	if [[ ! -r "$BASELINE" ]]; then
		echo "clock-drift-host: no baseline at $BASELINE — nothing recorded the real instant, so the" >&2
		echo "                  shift cannot be proven and a green run would prove nothing." >&2
		exit 1
	fi
	short=$(shortfall "$days")
	if [[ "$short" -gt "$SHORTFALL_TOLERANCE_SECONDS" ]]; then
		echo "clock-drift-host: the clock reads ${short}s short of the ${days}-day offset." >&2
		echo "                  Either the jump did not take or a time service re-synced during the" >&2
		echo "                  run; in both cases the suite ran at a clock nobody chose, so this run" >&2
		echo "                  is void rather than a finding about the tree." >&2
		exit 1
	fi
	echo "clock-drift-host: +${days} days still in force."
}

# restore puts the clock back and hands timekeeping to the machine again.
#
# It subtracts the offset rather than rewinding to the baseline, which would
# leave the machine however long the suite took in the past. It re-proves the
# shift first: a baseline can outlive a jump a time service undid, and
# subtracting from a clock nobody moved poisons the runner from the other side.
#
# Each failure returns its own status: run from an exit trap, a bare `return`
# reports the status from before the trap in bash 5.
restore() {
	local days short
	days=$(days "$1")
	if [[ ! -r "$BASELINE" ]]; then
		echo "clock-drift-host: no baseline, so the clock was never moved — leaving it where it is."
		sudo timedatectl set-ntp true || return $?
		return 0
	fi
	short=$(shortfall "$days") || return $?
	if [[ "$short" -gt "$SHORTFALL_TOLERANCE_SECONDS" ]]; then
		echo "clock-drift-host: the clock is already back (${short}s short of the offset) — leaving it alone."
		sudo timedatectl set-ntp true || return $?
		rm -f "$BASELINE" || return $?
		return 0
	fi
	sudo date -s "-${days} days" >/dev/null || return $?
	sudo timedatectl set-ntp true || return $?
	rm -f "$BASELINE" || return $?
	echo "clock-drift-host: clock restored, time sync back on."
}

# The runner's TLS credentials also expire at the shifted date. Restore in the
# same shell, before returning control to its agent, even on failure or a hang.
finish_run() {
	local status="$1" offset="$2"
	trap - EXIT INT TERM
	if ! restore "$offset"; then
		echo "clock-drift-host: restoration failed; recovery baseline retained." >&2
		[[ "$status" -ne 0 ]] || status=1
	fi
	exit "$status"
}

run_shifted() {
	local offset status=0
	offset=$(days "$1")
	export BACKEND_CLOCK_SKEW="machine:$offset"
	command -v timeout >/dev/null || { echo "clock-drift-host: GNU timeout is required." >&2; exit 1; }
	# Expanded now: offset is local and already validated as digits, and a
	# newer bash drops a function's locals before it runs an exit trap.
	trap "finish_run \"\$?\" $offset" EXIT
	trap 'exit 130' INT
	trap 'exit 143' TERM
	move "$offset"
	# A relative timer survives wall-clock changes. Leave recovery time before
	# the runner loses its control-plane lease during a long network outage.
	timeout --signal=TERM --kill-after=30s 45m make backend-clock-drift || status=$?
	if [[ "$status" -eq 124 ]]; then
		echo "clock-drift-host: suite exceeded the shifted-clock budget; no complete verdict." >&2
	fi
	exit "$status"
}

case "${1:-}" in
move) [[ $# -eq 2 ]] || usage; move "$2" ;;
assert) [[ $# -eq 2 ]] || usage; assert "$2" ;;
restore) [[ $# -eq 2 ]] || usage; restore "$2" ;;
run) [[ $# -eq 2 ]] || usage; run_shifted "$2" ;;
*) usage ;;
esac
