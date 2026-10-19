#!/usr/bin/env bash
# test-dev-env-local.sh — prove that what an engineer puts in .env.local reaches
# the stack `make dev` boots, and that what it does NOT reach says so.
#
# The failure this holds against is silent in both directions: a value read
# and then discarded by a default the launch line sets for the same name, which
# looks exactly like a bug in whatever was being tested. The api and the worker
# read .env.local themselves through --env-file, where the environment outranks
# it, so the script must neither export the file nor export a default over it.
#
# The division this asserts: `make dev` OWNS what makes the stack a per-worktree
# dev stack (MARGINCE_ENV, the database, the Redis db, the ports). Everything
# else it sets is a DEFAULT, and the blobstore four are the ones an engineer
# legitimately replaces — pointing at a real object store instead of the compose
# MinIO.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
dev="$root/scripts/dev.sh"
failures=0

check() { # want got description
    if [[ "$1" = "$2" ]]; then
        printf '  ok   %s\n' "$3"
    else
        printf '  FAIL %s\n       want: %s\n       got:  %s\n' "$3" "$1" "$2" >&2
        failures=$((failures + 1))
    fi
}

# The resolver itself, lifted from the script rather than reimplemented — a copy
# here would be a second version of production that proves nothing about it.
eval "$(sed -n '/^stack_default() {/,/^}$/p; /^resolve_stack_environment() {/,/^}$/p; /^read_env_local() {/,/^}$/p' "$dev")"

# resolved runs the resolver in a subshell carrying `env` and echoes one
# variable's effective value, so no case leaks its environment into the next.
resolved() { # var env-assignment...
    local var="$1"; shift
    ( export "$@" 2>/dev/null || true
      resolve_stack_environment 29000 >/dev/null
      printf '%s' "${!var}" )
}

echo "dev.sh: the object store is the engineer's to point somewhere else"

check "localhost:29000" "$(resolved MARGINCE_BLOBSTORE_ENDPOINT NOTHING=)" \
      "the compose MinIO by default"
check "s3.eu-central-1.amazonaws.com" \
      "$(resolved MARGINCE_BLOBSTORE_ENDPOINT MARGINCE_BLOBSTORE_ENDPOINT=s3.eu-central-1.amazonaws.com)" \
      "a real endpoint set in the environment wins"
check "margince-scratch" \
      "$(resolved MARGINCE_BLOBSTORE_BUCKET MARGINCE_BLOBSTORE_BUCKET=margince-scratch)" \
      "so does the bucket — the resolver leaves a set one alone"
check "AKIAEXAMPLE" \
      "$(resolved MARGINCE_BLOBSTORE_ACCESS_KEY MARGINCE_BLOBSTORE_ACCESS_KEY=AKIAEXAMPLE)" \
      "and the credentials, which is the point of naming a real store at all"
check "minioadmin" "$(resolved MARGINCE_BLOBSTORE_SECRET_KEY NOTHING=)" \
      "the throwaway compose credential is still the default"

# A value only .env.local names stays out of the exported environment, so the
# api reads it through --env-file and names the file as its source.
check "" "$( ( unset MARGINCE_BLOBSTORE_ENDPOINT
              MARGINCE_BLOBSTORE_ENDPOINT=s3.example.test
              resolve_stack_environment 29000 >/dev/null
              env | grep '^MARGINCE_BLOBSTORE_ENDPOINT=' || true ) )" \
      "a value .env.local gives is not exported over the file"

echo "dev.sh: .env.local is read, not exported, and the environment outranks it"

envdir="$(mktemp -d)"
trap 'rm -rf "$envdir"' EXIT
printf 'GEMINI_API_KEY=from-file\nMARGINCE_LOG_LEVEL=debug\ncmd=from-file\n' >"$envdir/.env.local"
read_back() { # var
    ( cd "$envdir"; cmd=script; read_env_local
      printf '%s|%s|%s' "${!1:-}" "$(env | grep -c "^$1=" || true)" "$cmd" )
}
check "from-file|0|script" "$(unset GEMINI_API_KEY; read_back GEMINI_API_KEY)" \
      "a name only the file sets reaches the script, unexported, and the script's own names stay"
check "from-env|1|script" "$(export GEMINI_API_KEY=from-env; read_back GEMINI_API_KEY)" \
      "a name the environment sets keeps the environment's value"

echo "dev.sh: the posture is this stack's, and it says so"

check "dev" "$(resolved MARGINCE_ENV MARGINCE_ENV=production)" \
      "MARGINCE_ENV=production does not boot a production posture through \`make dev\`"
check yes \
      "$( ( export MARGINCE_ENV=production; resolve_stack_environment 29000 ) | grep -q 'runs as dev regardless' && echo yes || echo no)" \
      "and the override is announced rather than applied in silence"
check no \
      "$( ( unset MARGINCE_ENV; resolve_stack_environment 29000 ) | grep -q 'runs as dev regardless' && echo yes || echo no)" \
      "with nothing set there is nothing to announce"

echo "dev.sh: the launch cannot take the choice back"

# The resolver above can be right and the stack still wrong: a command-prefix
# assignment on the launch line outranks the exported value, which is exactly
# how this was lost. Asserted against the script's text because running the
# launch means booting a real stack.
# `VAR="${VAR:-default}"` is the resolver doing its job; anything else assigning
# the name is a line that takes the answer back.
for var in MARGINCE_ENV MARGINCE_BLOBSTORE_ENDPOINT MARGINCE_BLOBSTORE_ACCESS_KEY \
           MARGINCE_BLOBSTORE_SECRET_KEY MARGINCE_BLOBSTORE_REGION; do
    check no "$(grep -E "^[[:space:]]*${var}=" "$dev" | grep -qv ':-' && echo yes || echo no)" \
          "nothing re-assigns $var outside the resolver"
done

echo "dev.sh: .env.local is read before anything asks the environment a question"

# `|| true` on both: a grep that matches nothing exits non-zero, and under
# `set -e` that would end this script with no finding printed — which reads
# exactly like a clean run of a test that never ran.
sourced_at="$(grep -n '^read_env_local$' "$dev" | head -1 | cut -d: -f1 || true)"
first_default="$(grep -nE '^[A-Z_]+="\$\{[A-Z_]+:-' "$dev" | head -1 | cut -d: -f1 || true)"
check yes "$([[ -n "$sourced_at" ]] && echo yes || echo no)" \
      "the script calls read_env_local at all"
check yes "$([[ -n "$first_default" ]] && echo yes || echo no)" \
      "and still resolves at least one default out of the environment for the order to be about"
check yes "$([[ -n "$sourced_at" && -n "$first_default" && "$sourced_at" -lt "$first_default" ]] && echo yes || echo no)" \
      "the file is read (line ${sourced_at:-none}) above the first such default (line ${first_default:-none})"

if [[ "$failures" -gt 0 ]]; then
    echo "FAIL: test-dev-env-local — $failures check(s) failed" >&2
    exit 1
fi
echo "OK: test-dev-env-local — what .env.local sets reaches the stack, and what it cannot set says so"
