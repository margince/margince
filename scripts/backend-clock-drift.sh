#!/usr/bin/env bash
# The backend's counterpart to fe-clock-drift: the same suites, run at a moved
# clock, required to reach the same verdict. Which clock moved is
# BACKEND_CLOCK_SKEW's business; internal/shared/clockskew says what each
# applier reaches.
set -euo pipefail
cd "$(dirname "$0")/.."

# The suites, one name per line. scheduled.yml runs each slice as its own job,
# because a hosted runner whose clock is moved 200 days stops renewing its job
# lock and is dropped after about an hour. The names live here, beside what
# runs them.
INTEGRATION_SLICES=6
slices() {
	# The unit legs are the prerequisites of `test:` in backend/Makefile, read
	# from there so a leg added or dropped there changes the slices with it.
	local leg
	for leg in $(sed -n 's/^test:[[:space:]]*\([^#]*\).*/\1/p' backend/Makefile); do
		printf 'unit-%s\n' "${leg#test-}"
	done
	local k
	for ((k = 1; k <= INTEGRATION_SLICES; k++)); do
		printf 'integration:%s/%s\n' "$k" "$INTEGRATION_SLICES"
	done
}

# slice_key is a slice's name as a job artifact may spell it: no ':' or '/'.
slice_key() {
	printf '%s' "$1" | tr ':/' '--'
}

# matrix is the slice list as scheduled.yml's job matrix reads it.
matrix() {
	local slice sep=''
	printf '['
	for slice in $(slices); do
		printf '%s{"slice":"%s","key":"%s"}' "$sep" "$slice" "$(slice_key "$slice")"
		sep=','
	done
	printf ']\n'
}

# verdict combines the per-slice verdict files in $1, one per slice key holding
# `pass` or `calendar`. A slice with no file never reached a verdict.
# Exit 0: every slice passed. 1: a suite failed at the moved clock. 3: a slice
# gave no verdict. A calendar red wins over a missing slice, because it is
# the one finding about the tree.
verdict() {
	local dir="$1" slice file got calendar=0 missing=0
	for slice in $(slices); do
		file="$dir/$(slice_key "$slice")"
		got=$(cat "$file" 2>/dev/null || true)
		case "$got" in
		pass) echo "backend-clock-drift: $slice passed at the moved clock" ;;
		calendar)
			echo "backend-clock-drift: $slice failed at the moved clock"
			calendar=1
			;;
		*)
			echo "backend-clock-drift: $slice gave no verdict" >&2
			missing=1
			;;
		esac
	done
	[[ "$calendar" -eq 0 ]] || return 1
	[[ "$missing" -eq 0 ]] || return 3
}

case "${1:-}" in
slices)
	slices
	exit 0
	;;
matrix)
	matrix
	exit 0
	;;
verdict)
	[[ $# -eq 2 ]] || { echo "usage: $0 verdict <dir>" >&2; exit 2; }
	status=0
	verdict "$2" || status=$?
	exit "$status"
	;;
esac

# BACKEND_CLOCK_SLICE picks one slice; unset runs every suite in one go, which
# is what the database and fixture appliers want on a laptop.
SLICE="${BACKEND_CLOCK_SLICE:-}"
KNOWN_SLICES="$(slices)"
if [[ -n "$SLICE" ]] && ! grep -Fxq -- "$SLICE" <<<"$KNOWN_SLICES"; then
	echo "backend-clock-drift: BACKEND_CLOCK_SLICE=$SLICE is not one of: $(tr '\n' ' ' <<<"$KNOWN_SLICES")" >&2
	exit 2
fi

SKEW="${BACKEND_CLOCK_SKEW:-}"
if [[ -z "$SKEW" ]]; then
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
	# No TLS certificate verifies at the moved date, so a module still missing
	# here fails by name instead of as a certificate error.
	export GOPROXY=off
	;;
database)
	echo "backend-clock-drift: the database applier is a REPRODUCTION AID, not a verdict."
	echo "                     Go's clock is untouched and the migrations' column DEFAULTs keep"
	echo "                     the real date, so a row written now lands 200 days before the"
	echo "                     now() a query compares it against. Failures under it are expected"
	echo "                     and are the applier's, not the tree's."
	;;
esac

# Uncached: Go's test cache does not key on the wall clock, so a package that
# passed at the real date would replay as `ok (cached)` under the lane's name.
export GOFLAGS="${GOFLAGS:-} -count=1"

echo "backend-clock-drift: running the backend suites${SLICE:+ ($SLICE)} at $SKEW"

# Every suite runs and the verdicts combine, so one unit red cannot keep the
# integration half, where the files comparing against Postgres' now() live,
# from running at all.
suites=0
case "$SLICE" in
'')
	make -C backend test || suites=$?
	make -C backend test-integration || suites=$?
	;;
unit-*) make -C backend "test-${SLICE#unit-}" || suites=$? ;;
integration:*) INTEGRATION_SHARD="${SLICE#integration:}" make -C backend test-integration || suites=$? ;;
esac

# The shift is proven again, on the failing path too: a time service that
# re-synced mid-run makes the rest an ordinary run, and its verdict the lane's.
if [[ "${SKEW%%:*}" == "machine" ]]; then
	./scripts/clock-drift-host.sh assert "${SKEW##*:}"
fi
# The one line that makes a red a finding about the tree: scheduled.yml reads it
# to tell a failed suite from a lane that stopped before its verdict.
if [[ "$suites" -ne 0 ]]; then
	echo "backend-clock-drift: the suites failed at $SKEW"
fi
exit "$suites"
