#!/usr/bin/env bash
# lib-devready.sh's own test, and the wiring that makes dev.sh, seed-dev.sh and
# verify-boot.sh use it.
#
# Docker is faked by scripts on PATH and the app port by a small HTTP server, so
# every case runs without Docker and without a stack.
#
# Usage: bash scripts/lib-devready.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LIB="$SCRIPT_DIR/lib-devready.sh"

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

work="$(mktemp -d)"
servers=""
cleanup() {
  local pid
  for pid in $servers; do kill "$pid" 2>/dev/null || true; done
  rm -rf "$work"
}
trap cleanup EXIT

# A PATH with the tools the helper needs and no docker, so "docker is missing"
# holds on a machine that has one installed.
mkdir -p "$work/base"
for tool in bash tail sed sleep cat grep; do
  ln -s "$(command -v "$tool")" "$work/base/$tool"
done

# fake_docker DIR BODY — a docker on DIR whose behaviour is the bash in BODY.
fake_docker() {
  mkdir -p "$1"
  printf '#!/usr/bin/env bash\n%s\n' "$2" >"$1/docker"
  chmod +x "$1/docker"
}

# check_docker DIR [VAR=VALUE…] — require_docker_compose with DIR first on PATH,
# leaving its exit status in STATUS and its stderr in SAID.
check_docker() {
  local dir="$1"
  shift
  set +e
  SAID="$(env -i HOME="$HOME" PATH="$dir:$work/base" "$@" \
    "$(command -v bash)" -c "source '$LIB'; require_docker_compose" 2>&1)"
  STATUS=$?
  set -e
}

echo "require_docker_compose: a machine without Docker or Compose is refused by name"

check_docker "$work/none"
[[ "$STATUS" = "1" ]] || fail "no docker on PATH exited $STATUS, want 1"
[[ "$SAID" == *"docker is not on PATH"* ]] || fail "no docker said '$SAID', want it named"

fake_docker "$work/no-compose" 'if [[ "$1" == compose ]]; then echo "docker: unknown command: docker compose" >&2; exit 1; fi; exit 0'
check_docker "$work/no-compose"
[[ "$STATUS" = "1" ]] || fail "docker without its Compose plugin exited $STATUS, want 1"
[[ "$SAID" == *"Compose plugin is not"* ]] || fail "docker without Compose said '$SAID', want the missing plugin named"
[[ "$SAID" == *"unknown command: docker compose"* ]] || fail "docker without Compose dropped docker's own stderr: '$SAID'"
[[ "$SAID" == *"docker-compose-plugin"* ]] || fail "docker without Compose said '$SAID', want how to install it"

fake_docker "$work/hangs" 'if [[ "$1" == compose ]]; then exec sleep 30; fi; exit 0'
started=$SECONDS
check_docker "$work/hangs" DOCKER_CHECK_SECONDS=1
[[ "$STATUS" = "1" ]] || fail "a compose that hangs exited $STATUS, want 1"
[[ "$SAID" == *"did not answer within 1s"* ]] || fail "a compose that hangs said '$SAID', want the bound named"
((SECONDS - started < 10)) || fail "a compose that hangs held the check $((SECONDS - started))s, past its 1s bound"

fake_docker "$work/no-daemon" 'if [[ "$1" == info ]]; then echo "Cannot connect to the Docker daemon at unix:///var/run/docker.sock" >&2; exit 1; fi; exit 0'
check_docker "$work/no-daemon"
[[ "$STATUS" = "1" ]] || fail "a daemon that does not answer exited $STATUS, want 1"
[[ "$SAID" == *"Cannot connect to the Docker daemon"* ]] || fail "a dead daemon dropped docker's own stderr: '$SAID'"
[[ "$SAID" == *"docker context ls"* ]] || fail "a dead daemon said '$SAID', want the context check named"

fake_docker "$work/ok" 'exit 0'
check_docker "$work/ok"
[[ "$STATUS" = "0" ]] || fail "a working docker exited $STATUS, want 0"
[[ -z "$SAID" ]] || fail "a working docker said '$SAID', want silence"

# The Makefile's COMPOSE override is what db-up runs, so it is what is checked.
printf '#!/usr/bin/env bash\nexit 0\n' >"$work/no-compose/other-compose"
chmod +x "$work/no-compose/other-compose"
check_docker "$work/no-compose" "COMPOSE=other-compose -f some.yml"
[[ "$STATUS" = "0" ]] || fail "COMPOSE naming a working command exited $STATUS ('$SAID'), want 0"

echo "dev.sh: ensure_infra runs the Docker check before make db-up"
ensure_infra_src="$(awk '/^ensure_infra\(\) /{inside=1} inside{print} inside && /^}/{exit}' "$SCRIPT_DIR/dev.sh")"
mkdir -p "$work/none"
printf '#!/usr/bin/env bash\necho reached >>"%s/make-calls"\n' "$work" >"$work/none/make"
chmod +x "$work/none/make"
set +e
SAID="$(env -i HOME="$HOME" PATH="$work/none:$work/base" "$(command -v bash)" -c \
  "source '$LIB'; infra_retry_count=0 infra_retry_pause=0; $ensure_infra_src
ensure_infra" 2>&1)"
STATUS=$?
set -e
[[ "$STATUS" = "1" ]] || fail "ensure_infra without docker exited $STATUS, want 1"
[[ "$SAID" == *"docker is not on PATH"* ]] || fail "ensure_infra without docker said '$SAID', want the preflight's message"
[[ ! -f "$work/make-calls" ]] || fail "ensure_infra ran make db-up on a machine without docker"

echo "warn_wsl_mounted_checkout: a checkout on a Windows drive under WSL is named"
wsl() { WSL_DISTRO_NAME="$1" bash -c "source '$LIB'; warn_wsl_mounted_checkout '$2'" 2>&1; }
[[ "$(wsl Ubuntu /mnt/c/src/margince)" == WARNING:* ]] || fail "a /mnt/c checkout under WSL was not warned about"
[[ -z "$(wsl Ubuntu /home/dev/src/margince)" ]] || fail "a Linux-filesystem checkout under WSL was warned about"
# Off WSL, /mnt/c is an ordinary path. A host that is itself WSL cannot run this case.
if ! grep -qi microsoft /proc/version 2>/dev/null; then
  [[ -z "$(wsl "" /mnt/c/src/margince)" ]] || fail "a /mnt/c path off WSL was warned about"
fi

echo "dev_require_app: a missed app port says whether the api or the frontend proxy is down"

# serve MODE — an HTTP server on a free port, printing that port. MODE is
# `hang` (accept, never answer), `status:<code>`, or `slow:<s>` (503 for <s>
# seconds, then 200: a proxy still warming up).
cat >"$work/server.py" <<'PY'
import http.server, socketserver, sys, time
mode, started = sys.argv[1], time.time()
class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if mode == "hang":
            time.sleep(60)
            return
        code = int(mode.split(":")[1]) if mode.startswith("status:") else (
            200 if time.time() - started >= float(mode.split(":")[1]) else 503)
        self.send_response(code)
        self.end_headers()
    def log_message(self, *args):
        pass
class Server(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True
server = Server(("127.0.0.1", 0), Handler)
print(server.server_address[1], flush=True)
server.serve_forever()
PY
serve() {
  local out="$work/port.$RANDOM"
  python3 "$work/server.py" "$1" >"$out" 2>/dev/null &
  servers="$servers $!"
  for _ in $(seq 1 50); do
    [[ -s "$out" ]] && break
    sleep 0.1
  done
  cat "$out"
}

# record FE API — the record `make dev` writes for slug `probe`.
export MARGINCE_DEV_STATE_DIR="$work/state" DEV_SLUG=probe
record() {
  mkdir -p "$MARGINCE_DEV_STATE_DIR/probe"
  printf 'SLUG=probe\nFE_PORT=%s\nAPI_PORT=%s\n' "$1" "$2" >"$MARGINCE_DEV_STATE_DIR/probe/env"
}
require_app() { # base wait_s
  set +e
  # shellcheck source=scripts/lib-devstate.sh
  SAID="$(bash -c "source '$SCRIPT_DIR/lib-devstate.sh'; source '$LIB'; dev_require_app '$1' '$2'" 2>&1)"
  STATUS=$?
  set -e
}

api_up="$(serve status:200)"
proxy_hangs="$(serve hang)"
record "$proxy_hangs" "$api_up"
started=$SECONDS
require_app "http://localhost:$proxy_hangs" 1
[[ "$STATUS" = "1" ]] || fail "a proxy that never answers exited $STATUS, want 1"
[[ "$SAID" == *"answers on its own port :$api_up"* && "$SAID" == *"ROLE=fe"* ]] \
  || fail "a hung proxy in front of a live api said '$SAID', want the frontend named"
((SECONDS - started < 30)) || fail "a hung proxy held the check $((SECONDS - started))s; each request must be bounded"

api_down="$(serve status:503)"
proxy_502="$(serve status:502)"
record "$proxy_502" "$api_down"
require_app "http://localhost:$proxy_502" 0
[[ "$STATUS" = "1" ]] || fail "a dead api exited $STATUS, want 1"
[[ "$SAID" == *"does not answer on its own port :$api_down"* && "$SAID" == *"ROLE=api"* ]] \
  || fail "a dead api behind the proxy said '$SAID', want the api named"

# Cold start: the proxy answers 503 for its first 3 seconds. The bounded wait is
# what carries it; without the wait the same stack is refused.
proxy_warming="$(serve slow:3)"
record "$proxy_warming" "$api_up"
require_app "http://localhost:$proxy_warming" 0
[[ "$STATUS" = "1" ]] || fail "a warming proxy with no wait exited $STATUS, want 1"
require_app "http://localhost:$proxy_warming" 20
[[ "$STATUS" = "0" ]] || fail "a proxy warm within the wait exited $STATUS ('$SAID'), want 0"

# A base that is not this worktree's recorded stack (CI, an explicit API_BASE):
# no guess about which side is down.
require_app "http://localhost:1" 0
[[ "$SAID" == *"Is the stack up?"* ]] || fail "an unrecorded base said '$SAID', want the plain start hint"

echo "wiring: make dev waits for the proxy; seed-dev and verify-boot wait for the app"
grep -qF 'wait_ready "http://localhost:${fe_port}/readyz"' "$SCRIPT_DIR/dev.sh" \
  || fail "dev.sh announces ready without asking for /readyz through the app port"
# The first top-level request of each: seed-dev signs in, verify-boot logs in.
for script in seed-dev.sh verify-boot.sh; do
  first_check="$(grep -n '^dev_require_app "\$API_BASE"' "$SCRIPT_DIR/$script" | head -1 | cut -d: -f1 || true)"
  first_request="$(grep -nE '^(sign_in_as_admin$|login_status=)' "$SCRIPT_DIR/$script" | head -1 | cut -d: -f1 || true)"
  [[ -n "$first_check" ]] || { fail "$script never calls dev_require_app"; continue; }
  [[ -n "$first_request" ]] || { fail "$script: found no first request to place the check against"; continue; }
  ((first_check < first_request)) || fail "$script sends its first request (line $first_request) before dev_require_app (line $first_check)"
done

if ((FAILURES > 0)); then
  echo "lib-devready: $FAILURES case(s) failed" >&2
  exit 1
fi
echo "OK: lib-devready — Docker and Compose are checked first; a missed app port names the side that failed"
