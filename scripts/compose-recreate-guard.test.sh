#!/usr/bin/env bash
# Prove `make db-up` refuses to recreate a running shared dev container.
#
# `docker` is stubbed on PATH: a real case needs a running stack and would
# recreate the one this developer's own lanes are using.
set -euo pipefail

root="$(CDPATH= cd -P "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guard="$root/scripts/compose-recreate-guard.sh"
failures=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

# The stub answers from three files: the hashes this checkout's compose file
# wants, the services with a running container, and each container's label.
cat >"$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
case "$*" in
*" config --hash *") [[ "$3" == "dir with space/x.yml" ]] || { echo "stub: compose file split into words: $*" >&2; exit 65; }; cat "$STUB_DIR/wanted" ;;
*" ps -q --status running "*) grep -qx "${*: -1}" "$STUB_DIR/running" && echo "id-${*: -1}" || true ;;
"inspect "*) grep "^${*: -1} " "$STUB_DIR/labels" | cut -d' ' -f2 ;;
*) echo "stub: unexpected docker $*" >&2; exit 64 ;;
esac
STUB
chmod +x "$work/bin/docker"

# stack WANTED RUNNING LABELS — set what compose wants and what is running.
stack() {
	printf '%s\n' "$1" >"$work/wanted"
	printf '%s\n' "$2" >"$work/running"
	printf '%s\n' "$3" >"$work/labels"
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

wanted=$'postgres hash-pg\nredis hash-redis'

stack "$wanted" $'postgres\nredis' $'id-postgres hash-pg\nid-redis hash-redis'
expect "matching running stack is left to compose" 0 ''

stack "$wanted" $'postgres\nredis' $'id-postgres hash-OTHER\nid-redis hash-redis'
expect "a drifted running postgres is refused, by name" 1 'recreate the running postgres container'
expect "the refusal names the way through" 1 'DB_RECREATE=1'
expect "DB_RECREATE=1 lets compose recreate it" 0 '' DB_RECREATE=1

stack "$wanted" 'redis' $'id-redis hash-redis'
expect "a stopped container is not checked" 0 ''

stack "$wanted" '' ''
expect "a machine with no stack yet is left to compose" 0 ''

if ((failures > 0)); then
	echo "compose-recreate-guard: $failures case(s) failed" >&2
	exit 1
fi
echo "compose-recreate-guard: all cases pass"
