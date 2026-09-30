#!/usr/bin/env bash
# The machine applier: move this host's wall clock, prove it moved, put it back.
#
# WHY THE HOST'S AND NOT A FAKE ONE. Go reads the wall clock through the vDSO,
# so no LD_PRELOAD shim reaches time.Now(); and Linux has no CLOCK_REALTIME
# namespace — a container cannot hold a wall clock of its own, only its own
# monotonic and boottime. The host's clock is therefore the only one whose move
# reaches the Go test process and the compose Postgres as ONE clock. That
# matters more than the mechanism: 905 of this module's files compare against
# Postgres' now(), so a shift reaching only Go would fail them in their
# hundreds, and every one of those failures would belong to this lane rather
# than to the tree. A lane whose reds are its own fault teaches a reader to
# ignore it, which is worse than having no lane.
#
# NOT FOR A DEVELOPER MACHINE. Moving a laptop's clock 200 days expires its
# credentials and its certificates. This is for a disposable CI runner.
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
	echo "usage: $0 move <days> | assert <days> | restore <days>" >&2
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
	sudo timedatectl set-ntp false
	sudo date -s "+${days} days" >/dev/null
	# Written only once the jump has taken, so the file's EXISTENCE is the
	# evidence that there is something to undo.
	printf '%s\n' "$real" >"$BASELINE"
	assert "$days"
}

# assert proves the clock is still the moved one.
#
# Called after the suites as well as before them, because the failure worth
# catching is a time service that came back and re-synced partway through.
#
# ONE-SIDED, and that is the whole correctness of it. The baseline is the real
# instant frozen at move time; it does not tick. The moved clock does, so after
# an hour of suites the drift is the offset PLUS an hour, and a two-sided
# tolerance would call every run of any length a failure. What this has to
# separate is "the clock is still 200 days out" from "something put it back",
# and only a SHORTFALL says the second.
assert() {
	local days short
	days=$(days "$1")
	if [ ! -r "$BASELINE" ]; then
		echo "clock-drift-host: no baseline at $BASELINE — nothing recorded the real instant, so the" >&2
		echo "                  shift cannot be proven and a green run would prove nothing." >&2
		exit 1
	fi
	short=$(shortfall "$days")
	if [ "$short" -gt "$SHORTFALL_TOLERANCE_SECONDS" ]; then
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
# It SUBTRACTS the same offset rather than rewinding to the parked instant. The
# moved clock ticked normally while the suite ran, so subtracting is exact,
# where restoring the baseline would leave the machine however long the suite
# took in the past.
#
# It re-proves the shift first. A baseline can outlive a jump that did not hold
# — `move` writes it and then asserts, so a time service winning the race
# between the two leaves the file behind — and the workflow reaches this step
# under always(). Subtracting from a clock nobody moved is the poisoned runner
# this step exists to prevent, arrived at from the other side.
restore() {
	local days short
	days=$(days "$1")
	if [ ! -r "$BASELINE" ]; then
		echo "clock-drift-host: no baseline, so the clock was never moved — leaving it where it is."
		sudo timedatectl set-ntp true
		return 0
	fi
	short=$(shortfall "$days")
	rm -f "$BASELINE"
	if [ "$short" -gt "$SHORTFALL_TOLERANCE_SECONDS" ]; then
		echo "clock-drift-host: the clock is already back (${short}s short of the offset) — leaving it alone."
		sudo timedatectl set-ntp true
		return 0
	fi
	sudo date -s "-${days} days" >/dev/null
	sudo timedatectl set-ntp true
	echo "clock-drift-host: clock restored, time sync back on."
}

case "${1:-}" in
move) [ $# -eq 2 ] || usage; move "$2" ;;
assert) [ $# -eq 2 ] || usage; assert "$2" ;;
restore) [ $# -eq 2 ] || usage; restore "$2" ;;
*) usage ;;
esac
