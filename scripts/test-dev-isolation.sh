#!/usr/bin/env bash
# test-dev-isolation.sh — prove two worktrees get two stacks.
#
# The defect this exists for: the registry that hands out Redis logical
# databases used to live under the WORKTREE's own .tmp/dev/, so a second
# worktree read an empty registry and claimed database 64 as well. Two stacks
# then shared one consumer group, and whichever worker read an event first
# resolved it against its own database, found nothing, and acked it. The other
# stack's event was gone — and a projection that never runs is indistinguishable
# from a broken feature.
#
# Every function under test is LIFTED from scripts/dev.sh rather than
# reimplemented here, the way test-dev-dsn.sh does it: a copy would prove
# nothing about production.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
dev="$root/scripts/dev.sh"
failures=0

check() { # want got description
    if [ "$1" = "$2" ]; then
        printf '  ok   %s\n' "$3"
    else
        printf '  FAIL %s\n       want: %s\n       got:  %s\n' "$3" "$1" "$2" >&2
        failures=$((failures + 1))
    fi
}

# lift NAME — eval one function out of dev.sh, so every check below exercises
# production rather than a copy of it.
#
# awk rather than the `sed -n '/^name() {/,/^}$/p'` idiom test-dev-dsn.sh uses:
# that idiom needs the function name written as a literal inside single quotes,
# and this one takes it as an argument. Interpolating it means double quotes,
# and macOS ships bash 3.2, which brace-expands the `{/,/^}` in that pattern
# even when it is double-quoted — sed then receives two mangled programs, prints
# nothing, and every lifted function is silently missing. A test whose subject
# failed to load still runs, which is the part that makes it worth avoiding.
lift() { # function name
    eval "$(awk -v fn="$1" '
        index($0, fn "() ") == 1 { inside = 1 }
        inside                   { print }
        inside && $0 == "}"      { exit }
    ' "$dev")"
}

lift sanitize_slug

echo "dev.sh: a slug is safe for a path and for CREATE DATABASE"

check "cfg-retire" "$(sanitize_slug "cfg-retire")" \
      "an already-legal name is unchanged"
check "feat-ai-keys" "$(sanitize_slug "feat/ai-keys")" \
      "a slash becomes a hyphen — a slug reaches the filesystem"
check "abc" "$(sanitize_slug "ABC")" \
      "uppercase is folded — Postgres would fold it anyway"
check "a-b" "$(sanitize_slug "a...b")" \
      "a run of illegal characters collapses to one hyphen"
check "ab" "$(sanitize_slug "-ab-")" \
      "no leading or trailing hyphen"

# lift_const NAME — eval one top-level assignment out of dev.sh.
#
# The block bounds are READ from production rather than restated here. A test
# that carried its own copy of "64..79" would keep passing unchanged after
# somebody widened the block in dev.sh, and would then be asserting a range that
# no longer exists — a gate that fails short reports PASS over a smaller subject
# and there is nothing to notice.
lift_const() { # name
    eval "$(grep -m1 "^$1=" "$dev")"
}

lift bucket_for_slug

echo "dev.sh: attachment bytes are not shared between stacks"

check "margince-dev" "$(bucket_for_slug "")" \
      "the primary worktree keeps the bucket it already has"
check "margince-dev-cfg-retire" "$(bucket_for_slug "cfg-retire")" \
      "a linked worktree gets its own"
check "margince-dev-a-b" "$(bucket_for_slug "a_b")" \
      "an underscore becomes a hyphen — S3 bucket names admit no underscore, slugs do"

lift dev_state_root
lift read_registry
lift pick_free_db
lift pick_free_port
lift_const DEV_REDIS_DB_MIN
lift_const DEV_REDIS_DB_MAX
lift_const DEV_FE_PORT_MIN
lift_const DEV_FE_PORT_MAX
lift_const DEV_API_PORT_OFFSET

# port_listeners asks the OS what is bound. It is stubbed here — the one true
# boundary in this file — so the claim logic is checked against a KNOWN machine
# rather than against whatever happens to hold :8082 on the developer's laptop.
# A test that consults the real port table would pass or fail by coincidence,
# and the specific coincidence (someone else's dev server) is common.
listener_on=''
port_listeners() { # port
    [[ "$1" == "$listener_on" ]] && echo 4242
    return 0
}

echo "dev.sh: the registry is machine-global, so a second worktree sees the first"

# The DEFAULT location, checked before anything overrides it. Every assertion
# below points the root at a temp directory, so without this one the whole file
# would keep passing after somebody moved the default back under the worktree —
# and a per-worktree default is the entire defect. What matters is not the exact
# path but that it is not INSIDE this checkout, because a path inside it is
# per-worktree by construction however it is spelled.
default_root="$(dev_state_root)"
default_verdict=shared
case "$default_root" in
    # Inside the checkout: per-worktree by construction.
    "$root"/*) default_verdict="inside the checkout ($default_root)" ;;
    # RELATIVE: dev.sh cds to the worktree top before using it, so a relative
    # path is per-worktree too — and it is the spelling the old code used
    # (`.tmp/dev`). Checking only for "not under $root" would call that clean,
    # which is how this very assertion first passed over the defect it exists
    # to catch.
    /*) default_verdict=shared ;;
    *) default_verdict="relative, so resolved per worktree ($default_root)" ;;
esac
check "shared" "$default_verdict" \
      "the default registry is one absolute path per machine, not one per worktree"

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT
export MARGINCE_DEV_STATE_DIR="$tmp_root"

check "$tmp_root" "$(dev_state_root)" \
      "MARGINCE_DEV_STATE_DIR overrides the root — without it nothing here is testable"

# One worktree's stack, recorded the way dev.sh records it.
mkdir -p "$tmp_root/first"
printf 'SLUG=first\nFE_PORT=8081\nAPI_PORT=18081\nREDIS_DB=64\n' >"$tmp_root/first/env"

read_registry second
check "64" "$TAKEN_DBS" "the registry reports the other stack's Redis database"
check "8081" "$TAKEN_PORTS" "and its frontend port"
check "65" "$(pick_free_db)" \
      "a second stack takes the next free Redis database, never 64 again"
check "8082" "$(pick_free_port)" \
      "and the next free port"

# The same slug reclaims its own, so a restart keeps its URL and its streams.
read_registry first
check "64" "$MINE_DB" "the slug's own recorded Redis database is reclaimed"
check "8081" "$MINE_PORT" "and its own port"

# The registry only knows about OUR stacks. A foreign listener must still take a
# port out of play, or bind fails and wait_ready polls somebody else's server.
listener_on=8082
read_registry second
check "8083" "$(pick_free_port)" \
      "a port with a foreign listener is skipped even though no stack claims it"
listener_on=''

# The blocks are finite. Running out must refuse rather than double up: a shared
# bus is the silent failure this whole mechanism exists to prevent.
rm -rf "$tmp_root"/*
for i in $(seq "$DEV_REDIS_DB_MIN" "$DEV_REDIS_DB_MAX"); do
    mkdir -p "$tmp_root/s$i"
    printf 'SLUG=s%s\nFE_PORT=%s\nREDIS_DB=%s\n' "$i" "$((8000 + i))" "$i" >"$tmp_root/s$i/env"
done
read_registry newcomer
check "" "$(pick_free_db || true)" \
      "every Redis database claimed → pick_free_db yields nothing rather than reusing one"
check "1" "$(pick_free_db >/dev/null 2>&1; echo $?)" \
      "and it reports failure rather than returning success with an empty answer"

if [ "$failures" -gt 0 ]; then
    printf 'FAIL: %d check(s) failed\n' "$failures" >&2
    exit 1
fi
printf 'OK: dev isolation checks passed\n'
