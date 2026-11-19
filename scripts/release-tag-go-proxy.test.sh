#!/usr/bin/env bash
# The Go proxy check's own test: a version Go has not recorded passes, a version
# recorded at the tagged commit passes, and a version recorded at another
# commit, one recorded with no commit, and a proxy that does not answer are
# refused, each with its own message. A local HTTP server plays the proxy.
#
# Usage: bash scripts/release-tag-go-proxy.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/release-tag-go-proxy.sh"

WORK="$(mktemp -d)"
server_pid=""
cleanup() {
	[[ -n "$server_pid" ]] && kill "$server_pid" 2>/dev/null
	rm -rf "$WORK"
}
trap cleanup EXIT
FAILURES=0

tagged=1111111111111111111111111111111111111111
other=2222222222222222222222222222222222222222
module=example.com/acme
printf 'module %s\n' "$module" >"$WORK/go.mod"
versions="$WORK/proxy/$module/@v"
mkdir -p "$versions"
info() { printf '{"Version":"%s","Origin":{"VCS":"git","Hash":"%s"}}' "$1" "$2" >"$versions/$1.info"; }
info v1.0.0 "$tagged"
info v1.0.1 "$other"
printf '{"Version":"v1.0.2"}' >"$versions/v1.0.2.info"

port="$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')"
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$WORK/proxy" >/dev/null 2>&1 &
server_pid=$!
for _ in $(seq 50); do
	curl --silent --output /dev/null "http://127.0.0.1:$port/" && break
	sleep 0.1
done

# verdict <want: pass|refuse> <tag> <proxy> <message part> <reason>, run in $WORK.
verdict() {
	local want="$1" tag="$2" proxy="$3" part="$4" reason="$5" out status=0
	out="$(cd "$WORK" && GO_PROXY_CACHED="$proxy" bash "$CHECK" "$tag" "$tagged" 2>&1)" || status=$?
	if [[ "$want" == pass && "$status" -ne 0 ]] || [[ "$want" == refuse && "$status" -eq 0 ]]; then
		printf 'FAIL: wanted %s, got exit %s, because %s\n  got: %s\n' "$want" "$status" "$reason" "$out"
		FAILURES=$((FAILURES + 1))
	elif [[ "$out" != *"$part"* ]]; then
		printf 'FAIL: the message lacks %q, because %s\n  got: %s\n' "$part" "$reason" "$out"
		FAILURES=$((FAILURES + 1))
	fi
}

proxy="http://127.0.0.1:$port"
verdict pass v9.9.9 "$proxy" 'has not recorded' 'a version nobody asked for is free to tag'
verdict pass v1.0.0 "$proxy" 'at this commit' 'a run of the same tag again builds what Go recorded'
verdict refuse v1.0.1 "$proxy" 'this version is spent' 'a moved tag makes a release that is not the Go version'
verdict refuse v1.0.2 "$proxy" 'names no commit' 'a check that cannot compare must not pass'

kill "$server_pid" 2>/dev/null
wait "$server_pid" 2>/dev/null || true
server_pid=""
verdict refuse v9.9.9 "$proxy" 'did not answer' 'a check that cannot run must not pass, and must say why'

if ((FAILURES)); then
	echo "release-tag-go-proxy: $FAILURES failure(s)"
	exit 1
fi
echo "OK: release-tag-go-proxy passes a free or unmoved version and refuses the rest"
