#!/usr/bin/env bash
# Prove `make db-up` refuses to stop or rebuild a running shared dev container.
#
# `docker` is stubbed on PATH: a real case needs a running stack and would
# disrupt the one this developer's own lanes are using. The plans below are
# what `docker compose up -d --dry-run` printed for each kind of drift.
set -euo pipefail

root="$(CDPATH= cd -P "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guard="$root/scripts/compose-recreate-guard.sh"
failures=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

# The stub answers the dry run from $STUB_DIR/plan and `inspect` from the
# container names listed in $STUB_DIR/running.
cat >"$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
case "$*" in
*" up -d --dry-run")
	[[ "$3" == "dir with space/x.yml" ]] || { echo "stub: compose file split into words: $*" >&2; exit 65; }
	[[ -f "$STUB_DIR/plan" ]] || { echo "dry run is not supported" >&2; exit 1; }
	cat "$STUB_DIR/plan" ;;
"inspect -f {{.State.Running}} "*) grep -qx "${*: -1}" "$STUB_DIR/running" && echo true || echo false ;;
*) echo "stub: unexpected docker $*" >&2; exit 64 ;;
esac
STUB
chmod +x "$work/bin/docker"

# stack PLAN RUNNING — what the dry run prints, and which containers run now.
stack() {
	rm -f "$work/plan"
	[[ "$1" == - ]] || printf '%s\n' "$1" >"$work/plan"
	printf '%s\n' "$2" >"$work/running"
}

# expect NAME STATUS PATTERN [ENV…] — run the guard and check its exit and
# stderr; an empty PATTERN requires an empty stderr.
expect() {
	local name="$1" want="$2" pattern="$3" got=0
	shift 3
	env PATH="$work/bin:$PATH" STUB_DIR="$work" DB_RECREATE= "$@" \
		bash "$guard" docker compose -f "dir with space/x.yml" 2>"$work/err" || got=$?
	if [[ "$got" != "$want" ]] ||
		{ [[ -z "$pattern" ]] && [[ -s "$work/err" ]]; } ||
		{ [[ -n "$pattern" ]] && ! grep -qE "$pattern" "$work/err"; }; then
		echo "FAIL: $name: exit $got (want $want), stderr: $(cat "$work/err")" >&2
		failures=$((failures + 1))
	fi
}

all_running=$'margince-postgres-1\nmargince-redis-1\nmargince-minio-1'

stack $' Container margince-postgres-1 Running \n Container margince-redis-1 Running ' "$all_running"
expect "a stack with the same settings is left to compose" 0 ''

stack $' Container margince-redis-1 Running \n Container margince-postgres-1 Recreate \n Container margince-postgres-1 Recreated \n Container margince-postgres-1 Started ' "$all_running"
expect "a rebuild of the running postgres is refused, by name" 1 'running container\(s\) margince-postgres-1:'
expect "the refusal names the way through" 1 'DB_RECREATE=1'
expect "DB_RECREATE=1 lets compose rebuild it" 0 '' DB_RECREATE=1

stack $' Container margince-postgres-1 Stopping \n Container margince-redis-1 Stopping \n Network margince_default Removing \n Network margince_default Creating \n Container margince-postgres-1 Started ' "$all_running"
expect "network drift, which stops running containers, is refused" 1 'margince-postgres-1 margince-redis-1:'

stack $' Container margince-postgres-1 Recreate \n Container margince-postgres-1 Started ' $'margince-redis-1'
expect "a rebuild of a stopped container is not counted" 0 ''

stack $' Network margince_default Creating \n Container margince-postgres-1 Creating ' ''
expect "a machine with no stack yet is left to compose" 0 ''

stack - "$all_running"
expect "a dry run that fails is a refusal, never a pass" 1 'could not ask compose'

if ((failures > 0)); then
	echo "compose-recreate-guard: $failures case(s) failed" >&2
	exit 1
fi
echo "compose-recreate-guard: all cases pass"
