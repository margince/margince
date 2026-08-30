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

# stub_docker <lines…> — a `docker ps` that answers with exactly these rows and
# RECORDS how it was asked, because what the query filters on is the whole
# check: a resolution missing either filter would find a postgres the DSN does
# not name, and answer confidently about it.
stub_docker() {
	printf '%s\n' "$@" >"$work/ps-output"
	: >"$work/ps-args"
	cat >"$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
# Only `docker ps` is answered; anything else is a case reaching past what it
# said it was testing.
if [[ "${1:-}" != "ps" ]]; then
	echo "the stub was asked for '$1', and these cases only resolve containers" >&2
	exit 64
fi
printf '%s\n' "$*" >"$PS_ARGS"
cat "$PS_OUTPUT"
STUB
	chmod +x "$work/bin/docker"
}

# stub_docker_failing — a docker that cannot be reached at all.
stub_docker_failing() {
	cat >"$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
echo "Cannot connect to the Docker daemon at unix:///var/run/docker.sock." >&2
exit 1
STUB
	chmod +x "$work/bin/docker"
}

# case_is <name> <expected-exit> <substring the output must carry>
case_is() {
	local name="$1" want="$2" carries="$3" out status=0
	out="$(PATH="$work/bin:$PATH" PS_OUTPUT="$work/ps-output" PS_ARGS="$work/ps-args" "$resolver" "${4:-15432}" 2>&1)" || status=$?
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
out="$(PATH="$work/bin:$PATH" PS_OUTPUT="$work/ps-output" PS_ARGS="$work/ps-args" "$resolver" 15432 2>&1)" || true
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

# The filters are the whole check, asserted on the query the resolver ACTUALLY
# MADE rather than on the text of the script: a filter written in a comment or
# built and then not passed reads the same to a grep. The port is deliberately
# not 15432, so a filter hard-coded to the developer's own stack fails here.
stub_docker "84cda66f97aa margince-postgres-1"
case_is "a non-default port resolves too" 0 "84cda66f97aa" 25432
asked="$(cat "$work/ps-args")"
for required in "--filter publish=25432" "--filter label=com.docker.compose.service=postgres"; do
	if [[ "$asked" != *"$required"* ]]; then
		echo "FAIL: docker ps was asked '$asked', which carries no '$required' — the resolver can answer about a container the DSN does not name"
		failures=$((failures + 1))
	else
		echo "ok: the query carries $required"
	fi
done

# A docker that cannot be reached answers with no rows, which downstream is
# indistinguishable from a stack that is not up. Telling a developer to run
# `make db-up` while the daemon is stopped sends them to the wrong problem.
stub_docker_failing
out="$(PATH="$work/bin:$PATH" "$resolver" 15432 2>&1)" && status=0 || status=$?
if [[ $status -eq 0 ]]; then
	echo "FAIL: an unreachable docker resolved a container"
	failures=$((failures + 1))
elif [[ "$out" != *"docker could not be asked"* || "$out" != *"Cannot connect to the Docker daemon"* ]]; then
	echo "FAIL: an unreachable docker is reported as an absent stack, not as an unreachable docker"
	echo "$out" | sed 's/^/    /'
	failures=$((failures + 1))
else
	echo "ok: an unreachable docker says so, rather than 'the stack is not up'"
fi

# dsn_port is what picks the port this whole resolution hangs on, so its own
# reading of a DSN is checked here rather than left to the one shape a dev
# stack happens to write.
port_of() {
	# shellcheck source=/dev/null
	. /dev/stdin <<-EOF
		$(sed -n '/^dsn_port() {/,/^}$/p' "$root/scripts/dev.sh")
	EOF
	dsn_port "$1"
}
while read -r dsn want why; do
	got="$(port_of "$dsn")"
	if [[ "$got" != "$want" ]]; then
		echo "FAIL: dsn_port($dsn) = $got, want $want — $why"
		failures=$((failures + 1))
	else
		echo "ok: $why"
	fi
done <<'DSNS'
postgres://u:p@localhost:15432/margince 15432 an explicit port is the port
postgres://u:p@localhost/margince 5432 no port means the one postgres defaults to
postgres://u:p@[::1]:15432/margince 15432 a bracketed IPv6 host with a port
postgres://u:p@[::1]/margince 5432 a bracketed IPv6 host whose colons are the address, not a port
postgres://u:p@localhost:15432?sslmode=disable 15432 parameters with no database path do not become part of the port
postgres://u:p@localhost:15432/margince?sslmode=disable 15432 parameters after a database path
DSNS

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures dev-postgres-container case(s) did not hold" >&2
	exit 1
fi
echo "OK: dev-postgres-container resolves one publisher and refuses every other shape"
