#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixtures="$(mktemp -d)"
trap 'rm -rf "$fixtures"' EXIT
mkdir "$fixtures/bin"
export CLOCK_STATE="$fixtures/clock" CLOCK_ACTIONS="$fixtures/actions"
export CLOCK_NTP_STATE="$fixtures/ntp"
export CLOCK_DRIFT_BASELINE="$fixtures/baseline"
export SLICE_GAP=0
export PATH="$fixtures/bin:$PATH"

cat >"$fixtures/bin/date" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == '-u +%s' ]]; then cat "$CLOCK_STATE"; exit; fi
if [[ "$1" != '-s' ]]; then exit 90; fi
if [[ "${MOCK_RESTORE_FAIL:-0}" == 1 && "$2" == -* ]]; then exit 91; fi
if [[ "${MOCK_MOVE_FAIL:-0}" == 1 && "$2" == +* ]]; then exit 92; fi
amount="${2%% days}"
echo "$(( $(cat "$CLOCK_STATE") + amount * 86400 ))" >"$CLOCK_STATE"
printf 'date %s\n' "$2" >>"$CLOCK_ACTIONS"
STUB
cat >"$fixtures/bin/sudo" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == timedatectl ]]; then
	printf '%s\n' "$3" >"$CLOCK_NTP_STATE"
	printf 'ntp %s\n' "$3" >>"$CLOCK_ACTIONS"
	exit 0
fi
exec "$@"
STUB
cat >"$fixtures/bin/make" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == backend-clock-drift ]]
[[ "$BACKEND_CLOCK_SKEW" == machine:200 ]]
[[ "$(cat "$CLOCK_STATE")" == 19000000 ]]
echo suite >>"$CLOCK_ACTIONS"
echo "slice $BACKEND_CLOCK_SLICE" >>"$CLOCK_ACTIONS"
exit "${MOCK_SUITE_STATUS:-0}"
STUB
cat >"$fixtures/bin/timeout" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1 $2" == '--signal=TERM --kill-after=30s' ]]
[[ "$3" == *m ]]
minutes="${3%m}"
(( minutes > 0 && minutes < 60 ))
echo budget >>"$CLOCK_ACTIONS"
shift 3
if [[ "${MOCK_TERMINATE:-0}" == 1 ]]; then kill -TERM "$PPID"; exit 0; fi
if [[ "${MOCK_TIMEOUT:-0}" == 1 ]]; then exit 124; fi
exec "$@"
STUB
cat >"$fixtures/bin/sleep" <<'STUB'
#!/usr/bin/env bash
echo "sleep $1" >>"$CLOCK_ACTIONS"
STUB
chmod +x "$fixtures/bin/"*

run_case() {
	local name="$1" expected="$2" status=0
	shift 2
	echo 1720000 >"$CLOCK_STATE"
	echo true >"$CLOCK_NTP_STATE"
	: >"$CLOCK_ACTIONS"
	rm -f "$CLOCK_DRIFT_BASELINE"
	env "$@" bash "$root/scripts/clock-drift-host.sh" run 200 >"$fixtures/output" 2>&1 || status=$?
	if [[ "$status" != "$expected" ]]; then
		cat "$fixtures/output" >&2
		echo "$name: status $status, want $expected" >&2
		exit 1
	fi
	if [[ "$(cat "$CLOCK_STATE")" != 1720000 || -e "$CLOCK_DRIFT_BASELINE" ]]; then
		echo "$name: clock or recovery state was not restored" >&2
		exit 1
	fi
	[[ "$(cat "$CLOCK_NTP_STATE")" == true ]]
	echo "PASS: $name"
}
run_case success 0
run_case 'caller applier cannot double the shift' 0 BACKEND_CLOCK_SKEW=database:200
grep -q '^suite$' "$CLOCK_ACTIONS"
# What must run, taken from the Makefile and the shard numbering, not from the script under test.
want="$(sed -n 's/^test:[[:space:]]*\([^#]*\).*/\1/p' "$root/backend/Makefile" | tr ' ' '\n' | sed '/^$/d; s/^test-/unit-/')"
run_case 'every slice gets its own moved window' 0
ran="$(sed -n 's/^slice //p' "$CLOCK_ACTIONS")"
total="$(printf '%s\n' "$ran" | sed -n 's|^integration:[0-9]*/||p' | sort -n | tail -1)"
for ((k = 1; k <= total; k++)); do want="$want"$'\n'"integration:$k/$total"; done
grep -q '^unit-' <<<"$want"
(( total > 1 ))
[[ "$(printf '%s\n' "$ran" | sort)" == "$(printf '%s\n' "$want" | sort)" ]] || { echo "slices that ran differ from the Makefile's test legs and every shard:" >&2; diff <(printf '%s\n' "$want" | sort) <(printf '%s\n' "$ran" | sort) >&2 || true; exit 1; }
slices="$(printf '%s\n' "$ran" | wc -l | tr -d ' ')"
[[ "$(grep -c '^date +200 days$' "$CLOCK_ACTIONS")" == "$slices" ]]
[[ "$(grep -c '^date -200 days$' "$CLOCK_ACTIONS")" == "$slices" ]]
grep -q 'ran 0s at the moved clock' "$fixtures/output"
run_case 'a real-time gap separates the slices' 0 SLICE_GAP=7
[[ "$(grep -c '^sleep 7$' "$CLOCK_ACTIONS")" == "$((slices - 1))" ]]
grep -q '7s at real time before the next slice' "$fixtures/output"
run_case 'suite failure stays red' 23 MOCK_SUITE_STATUS=23
run_case 'timeout restores clock' 124 MOCK_TIMEOUT=1
run_case 'termination restores clock' 143 MOCK_TERMINATE=1
run_case 'failed move restores time sync' 92 MOCK_MOVE_FAIL=1

echo 1720000 >"$CLOCK_STATE"
: >"$CLOCK_ACTIONS"
status=0
MOCK_RESTORE_FAIL=1 bash "$root/scripts/clock-drift-host.sh" run 200 >"$fixtures/output" 2>&1 || status=$?
[[ "$status" != 0 && -e "$CLOCK_DRIFT_BASELINE" ]]
[[ "$(cat "$CLOCK_STATE")" == 19000000 ]]
grep -q 'restoration failed' "$fixtures/output"
bash "$root/scripts/clock-drift-host.sh" restore 200
[[ "$(cat "$CLOCK_STATE")" == 1720000 && ! -e "$CLOCK_DRIFT_BASELINE" ]]
echo 'PASS: restoration failure remains recoverable'

status=0
bash "$root/scripts/clock-drift-host.sh" run 08 >"$fixtures/output" 2>&1 || status=$?
[[ "$status" == 2 && ! -e "$CLOCK_DRIFT_BASELINE" ]]
echo 'PASS: invalid offset changes no clock'

awk '
 /^  backend-clock-drift:/ { inside=1; next }
 inside && /^  [^ #][^:]*:/ { exit }
 inside && /^[[:space:]]*#/ { next }
 inside { print }
' "$root/.github/workflows/scheduled.yml" >"$fixtures/job"
# Matched anywhere in a step, comments left out: the suites step wraps its call to read the verdict.
grep -Fq './scripts/clock-drift-host.sh run ' "$fixtures/job"
if grep -Fq './scripts/clock-drift-host.sh move ' "$fixtures/job"; then
 echo 'the runner must not manage a new step while its clock is shifted' >&2
 exit 1
fi
grep -Fq 'run: ./scripts/clock-drift-host.sh restore ' "$fixtures/job"
echo 'PASS: workflow keeps shift and recovery in one shell'
