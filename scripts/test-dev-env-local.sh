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

echo "dev.sh: a default the launch supplies yields to .env.local, so the file stays the named source"

# shellcheck source=scripts/lib-devstate.sh
. "$root/scripts/lib-devstate.sh"
# Each default is lifted as the one statement dev.sh runs at boot.
launch_defaults="$(grep -E '^[[:space:]]*(\[\[ "\$ai_fake" == false \]\] \|\| )?stack_default MARGINCE_(PUBLIC_BASE_URL|API_BASE_URL|AI_FAKE|BLOBSTORE_BUCKET) ' "$dev" || true)"
check 4 "$(grep -c . <<<"$launch_defaults" || true)" "the four launch defaults are found where dev.sh sets them"
# launched VAR FILE-LINE — "<exported value>|<value the script holds>" after
# .env.local held FILE-LINE and the launch defaults ran.
launched() { # var file-line
    ( cd "$envdir"; printf '%s\n' "$2" >.env.local; unset "$1"
      fe_port=8093 api_port=18093 ai_fake=true slug=probe
      read_env_local; eval "$launch_defaults"
      printf '%s|%s' "$(env | grep "^$1=" | cut -d= -f2- || true)" "${!1:-}" )
}
while read -r var given fallback; do
    check "$fallback|$fallback" "$(launched "$var" "")" \
          "$var: with nothing naming it, the stack's default is exported to both roles"
    check "|$given" "$(launched "$var" "$var=$given")" \
          "$var: named in .env.local, it is left to --env-file, so the page names the file"
done <<EOF
MARGINCE_PUBLIC_BASE_URL https://tunnel.example.test http://localhost:8093
MARGINCE_API_BASE_URL https://api.tunnel.example.test http://localhost:18093
MARGINCE_AI_FAKE false true
MARGINCE_BLOBSTORE_BUCKET margince-scratch $(dev_bucket_for_slug probe)
EOF

# A flag outranks .env.local, so a launch flag for anything an engineer can set
# would take the choice and the provenance back. Only what the stack owns rides
# on the command line; comment lines are dropped so prose cannot hide a flag.
launches="$(awk 'BEGIN { RS="" } /\.\/bin\/(api|worker) --/ { print; found++ }
    END { if (found != 2) exit 1 }' "$dev" | grep -v '^[[:space:]]*#' || true)"
check "--addr --config --config-file --dsn --redis " \
      "$(grep -oE '(^|[[:space:]])--[a-z-]+' <<<"$launches" | tr -d ' \t' | sort -u | tr '\n' ' ' || true)" \
      "the api and the worker are launched with only the flags this stack owns"
# An array expanded on the launch line is a flag the census above cannot read.
check "env_file_flag " \
      "$(grep -oE '\$\{[a-z_]+\[@\]' <<<"$launches" | sed 's/^..//; s/\[@\]$//' | sort -u | tr '\n' ' ' || true)" \
      "and the one array they expand is --env-file itself"

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
           MARGINCE_BLOBSTORE_SECRET_KEY MARGINCE_BLOBSTORE_REGION MARGINCE_BLOBSTORE_BUCKET \
           MARGINCE_PUBLIC_BASE_URL MARGINCE_API_BASE_URL MARGINCE_AI_FAKE; do
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
