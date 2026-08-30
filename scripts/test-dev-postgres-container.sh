#!/usr/bin/env bash
# test-dev-postgres-container.sh — prove the dev tooling names ONE database.
#
# The resolver's whole job is to refuse rather than to guess, and a resolver
# that guesses reports the same success as one that is right: the seed runs, the
# statements commit, and the rows land in a database nobody named. So each case
# asserts the REFUSAL SENTENCE, not merely a non-zero exit.
#
# `docker` is stubbed on PATH rather than real containers being started: what is
# under test is the resolution, and a case that needed two published-port
# collisions on one host could not be written honestly against real Docker
# without taking the port this developer's own stack is using.
set -euo pipefail

# Resolve $0 through any symlinks BEFORE deriving the directory, and clear
# CDPATH: with CDPATH set, `cd` can land in a directory of the same name
# elsewhere and every case would invoke a script that does not exist.
self="${BASH_SOURCE[0]}"
while [[ -L "$self" ]]; do
	link="$(readlink "$self")"
	[[ "$link" == /* ]] && self="$link" || self="$(dirname "$self")/$link"
done
root="$(CDPATH= cd -P "$(dirname "$self")/.." && pwd)"
resolver="$root/scripts/dev-postgres-container.sh"
failures=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

# stub_docker <lines…> — a `docker ps` that answers with exactly these rows.
stub_docker() {
	printf '%s\n' "$@" >"$work/ps-output"
	cat >"$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
# Only `docker ps` is answered; anything else is a case reaching past what it
# said it was testing.
if [[ "${1:-}" != "ps" ]]; then
	echo "the stub was asked for '$1', and these cases only resolve containers" >&2
	exit 64
fi
cat "$PS_OUTPUT"
STUB
	chmod +x "$work/bin/docker"
}

# case_is <name> <expected-exit> <substring the output must carry>
case_is() {
	local name="$1" want="$2" carries="$3" out status=0
	out="$(PATH="$work/bin:$PATH" PS_OUTPUT="$work/ps-output" "$resolver" 15432 2>&1)" || status=$?
	if [[ $status -ne $want ]]; then
		echo "FAIL: $name — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$carries" && "$out" != *"$carries"* ]]; then
		echo "FAIL: $name — output does not carry '$carries'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

stub_docker "84cda66f97aa margince-postgres-1"
case_is "one publisher resolves to its id" 0 "84cda66f97aa"

# The shape that shipped: a second checkout's stack owns the compose project
# and publishes nothing, so `compose exec` lands there while the DSN's port
# belongs to somebody else. Nothing published on the port at all is the honest
# reading of that, and it must not fall back to a project lookup.
stub_docker ""
case_is "no publisher refuses rather than falling back" 1 "no postgres container publishes :15432"

# Two stacks on one port cannot both be the one the DSN means, and picking
# either is the guess this exists to prevent. The refusal NAMES them, because a
# developer has to know which to stop.
stub_docker "84cda66f97aa margince-postgres-1" "f61b3a7f4aff other-postgres-1"
case_is "two publishers refuse and name both" 1 "2 postgres containers publish :15432"
out="$(PATH="$work/bin:$PATH" PS_OUTPUT="$work/ps-output" "$resolver" 15432 2>&1)" || true
for named in "margince-postgres-1" "other-postgres-1"; do
	if [[ "$out" != *"$named"* ]]; then
		echo "FAIL: the refusal does not name $named, so a developer cannot tell which stack to stop"
		failures=$((failures + 1))
	fi
done

# A resolver invoked wrong is an operator error, not an empty result.
if PATH="$work/bin:$PATH" "$resolver" >/dev/null 2>&1; then
	echo "FAIL: the resolver ran with no port"
	failures=$((failures + 1))
else
	echo "ok: no port at all refuses"
fi

# The filters are the whole check: a query missing either one would resolve a
# postgres that is not this stack's, or this stack's postgres on another port.
for required in "publish=" "label=com.docker.compose.service=postgres"; do
	if ! grep -q -- "$required" "$resolver"; then
		echo "FAIL: the resolver does not filter on '$required', so it can resolve a container the DSN does not name"
		failures=$((failures + 1))
	else
		echo "ok: the query filters on $required"
	fi
done

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures dev-postgres-container case(s) did not hold" >&2
	exit 1
fi
echo "OK: dev-postgres-container resolves one publisher and refuses every other shape"
