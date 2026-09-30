#!/usr/bin/env bash
# The backend's counterpart to fe-clock-drift: the same suites, run at a moved
# clock, required to reach the same verdict.
#
# A fixture only becomes a bomb when something COMPARES its instant to now to
# decide a state, and no static rule separates those from the instants a test
# merely stores — which is why this is a second RUN rather than a pattern, and
# why gates/wallclockfixtures_test.go counts the population instead of judging
# it.
#
# WHICH CLOCK MOVED is BACKEND_CLOCK_SKEW's business, and the appliers are not
# interchangeable — internal/shared/clockskew carries what each one reaches.
set -euo pipefail
cd "$(dirname "$0")/.."

SKEW="${BACKEND_CLOCK_SKEW:-}"
if [ -z "$SKEW" ]; then
	echo "backend-clock-drift: BACKEND_CLOCK_SKEW is unset, so this would be the ordinary suite" >&2
	echo "                     reporting its verdict under the drift lane's name — which reads" >&2
	echo "                     exactly like a suite with no date-fragile fixtures left." >&2
	echo "                     machine:200  moves the host clock. CI only: it expires a laptop's" >&2
	echo "                                  certificates, and it is the only applier that moves" >&2
	echo "                                  the Go process and Postgres together." >&2
	echo "                     database:200 shadows now() in the test database. A REPRODUCTION" >&2
	echo "                                  AID: Go's clock stays put and the migrations' column" >&2
	echo "                                  DEFAULTs keep the real date, so rows land at one date" >&2
	echo "                                  and queries compare against another. Expect failures" >&2
	echo "                                  that belong to the applier; use it to reproduce a" >&2
	echo "                                  named test, never to judge the tree." >&2
	echo "                     fixture:200  moves only what clocktest.Now(t) hands a fixture." >&2
	exit 1
fi
export BACKEND_CLOCK_SKEW

# The applier WORD, to decide what to say before starting. The vocabulary and
# the amount are validated in Go, which is the only parser of this value.
case "${SKEW%%:*}" in
machine)
	# The host clock moved, so Go and the compose Postgres read the same shifted
	# instant. Prove it is still in force before spending an hour on the suites.
	./scripts/clock-drift-host.sh assert "${SKEW##*:}"
	;;
database)
	echo "backend-clock-drift: the database applier is a REPRODUCTION AID, not a verdict."
	echo "                     Go's clock is untouched and the migrations' column DEFAULTs keep"
	echo "                     the real date, so a row written now lands 200 days before the"
	echo "                     now() a query compares it against. Failures under it are expected"
	echo "                     and are the applier's, not the tree's."
	;;
esac

# UNCACHED, both halves. Go's test cache keys on the binary, its arguments and
# the environment a test READS — not on the wall clock, which is the whole
# subject here. Without this, a package that passed at the real date an hour ago
# replays as `ok (cached)` and the lane reports a green over a suite that never
# ran at the moved clock.
export GOFLAGS="${GOFLAGS:-} -count=1"

echo "backend-clock-drift: running the backend suites at $SKEW"

# BOTH halves run, and their verdicts are combined. Short-circuiting on the unit
# suite would mean a single unit red — including one inherited from main — kept
# the integration half from running at all, every night until somebody fixed it.
# That half is the lane's actual subject: the 905 files comparing against
# Postgres' now() are exercised nowhere else.
suites=0
make -C backend test || suites=$?
make -C backend test-integration || suites=$?

# The shift is proven AGAIN, and on the failing path too. A time service that
# re-synced partway through leaves the rest of the run an ordinary one, so a
# green after that proves nothing and a red after it belongs to the lane rather
# than to the tree — which is the first thing anyone reading a failure has to
# rule out.
if [ "${SKEW%%:*}" = machine ]; then
	./scripts/clock-drift-host.sh assert "${SKEW##*:}"
fi
exit "$suites"
