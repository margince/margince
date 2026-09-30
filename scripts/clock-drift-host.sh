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
# credentials and its certificates. This is for a disposable CI runner; the
# local reproduction is the database applier, which `make backend-clock-drift`
# selects by default.
set -euo pipefail
cd "$(dirname "$0")/.."

# Where the real instant is parked while the clock is moved. Restoring computes
# from it rather than trusting a re-sync to happen, so a runner whose time
# service never comes back still leaves with a plausible clock.
BASELINE="${CLOCK_DRIFT_BASELINE:-${TMPDIR:-/tmp}/clock-drift-baseline}"

# The tolerance on "did it move". Generous on purpose: what this proves is that
# a 200-DAY shift took, and the seconds either side of it are the suite starting
# up. A tight window here would fail the lane over scheduling noise, which is
# the lane blaming the tree for itself.
TOLERANCE_SECONDS=600

usage() {
	echo "usage: $0 move <days> | assert <days> | restore <days>" >&2
	exit 2
}

# move parks the real instant and jumps the clock forward.
#
# Time sync goes off FIRST. A runner whose NTP client is still running snaps the
# clock back mid-suite, and the half of the run after the snap is an ordinary
# run reporting under the drift lane's name — a green that means nothing, which
# is the failure this whole lane exists to remove.
move() {
	local days="$1" real
	real=$(date -u +%s)
	sudo timedatectl set-ntp false
	sudo date -s "+${days} days" >/dev/null
	# Written only once the jump has taken, so the file's EXISTENCE is the
	# evidence that there is something to undo. Written before, a failed jump
	# would leave a baseline behind and the always() restore would subtract two
	# hundred days from a clock that never moved — the poisoned runner that step
	# exists to prevent, arrived at from the other direction.
	printf '%s\n' "$real" >"$BASELINE"
	assert "$days"
}

# assert proves the clock is still where move put it.
#
# Called after the suites as well as before them, because the failure worth
# catching is a time service that came back and re-synced partway through.
assert() {
	local days="$1"
	if [ ! -r "$BASELINE" ]; then
		echo "clock-drift-host: no baseline at $BASELINE — nothing recorded the real instant, so the" >&2
		echo "                  shift cannot be proven and a green run would prove nothing." >&2
		exit 1
	fi
	local want moved drift skew
	want=$((days * 86400))
	moved=$(date -u +%s)
	drift=$((moved - $(cat "$BASELINE")))
	skew=$((drift - want))
	if [ "${skew#-}" -gt "$TOLERANCE_SECONDS" ]; then
		echo "clock-drift-host: the clock is ${drift}s from the real instant, want ${want}s (±${TOLERANCE_SECONDS}s)." >&2
		echo "                  Either the jump did not take or a time service re-synced during the run;" >&2
		echo "                  in both cases the suite ran at a clock nobody chose." >&2
		exit 1
	fi
	echo "clock-drift-host: +${days} days in force (${drift}s from the real instant)."
}

# restore puts the clock back and hands timekeeping to the machine again.
#
# It SUBTRACTS the same offset rather than rewinding to the parked instant. The
# moved clock ticked normally while the suite ran, so subtracting is exact,
# where restoring the baseline would leave the machine however long the suite
# took in the past — and a runner whose clock goes backwards fails its own
# cleanup steps in ways nothing here would explain.
restore() {
	local days="$1"
	# No baseline means move never completed — the compose stack failed, or the
	# build did, or the jump itself. The workflow still reaches this step under
	# always(), and subtracting the offset from a clock nobody moved is how a
	# runner goes into the past.
	if [ ! -r "$BASELINE" ]; then
		echo "clock-drift-host: no baseline, so the clock was never moved — leaving it where it is."
		sudo timedatectl set-ntp true
		return 0
	fi
	sudo date -s "-${days} days" >/dev/null
	rm -f "$BASELINE"
	sudo timedatectl set-ntp true
	echo "clock-drift-host: clock restored, time sync back on."
}

case "${1:-}" in
move) [ $# -eq 2 ] || usage; move "$2" ;;
assert) [ $# -eq 2 ] || usage; assert "$2" ;;
restore) [ $# -eq 2 ] || usage; restore "$2" ;;
*) usage ;;
esac
