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
# Every function under test is LIFTED from the scripts, or sourced from them,
# rather than reimplemented here: a copy would prove nothing about production.
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

# The identity helper is SOURCED, not lifted: it is a library, and sourcing it is
# what production does.
# shellcheck source=scripts/lib-devstate.sh
. "$root/scripts/lib-devstate.sh"

echo "lib-devstate.sh: a slug is safe for a path, a database, and a bucket"

check "cfg-retire" "$(dev_sanitize_slug "cfg-retire")" \
      "an already-legal name is unchanged"
check "feat-ai-keys" "$(dev_sanitize_slug "feat/ai-keys")" \
      "a slash becomes a hyphen — a slug reaches the filesystem"
check "abc" "$(dev_sanitize_slug "ABC")" \
      "uppercase is folded — Postgres would fold it anyway"
check "a-b" "$(dev_sanitize_slug "a...b")" \
      "a run of illegal characters collapses to one hyphen"
check "ab" "$(dev_sanitize_slug "-ab-")" \
      "no leading or trailing hyphen"
check "a-b" "$(dev_sanitize_slug "a_b")" \
      "an underscore folds to a hyphen, so the slug alphabet is already S3-legal"

echo "lib-devstate.sh: attachment bytes are not shared between stacks"

check "margince-dev" "$(dev_bucket_for_slug "")" \
      "the primary worktree keeps the bucket it already has"
check "margince-dev-cfg-retire" "$(dev_bucket_for_slug "cfg-retire")" \
      "a linked worktree gets its own"

# Injectivity is the property, not the spelling. While `_` was legal in a slug
# and illegal in a bucket, `a_b` and `a-b` were two worktrees sharing one bucket
# — the shared-Redis defect again, in the store holding attachment bytes.
same=0
for a in a-b a_b; do
    for b in a-b a_b; do
        [[ "$a" == "$b" ]] && continue
        [[ "$(dev_bucket_for_slug "$(dev_sanitize_slug "$a")")" \
           == "$(dev_bucket_for_slug "$(dev_sanitize_slug "$b")")" ]] && same=1
    done
done
check "1" "$same" \
      "two directory names differing only by _ vs - fold to ONE slug, so they cannot take two buckets that collide"

echo "lib-devstate.sh: the primary stack's directory is reserved"

check "1" "$(dev_resolve_slug "$DEV_BASE_SLUG_DIR" >/dev/null 2>&1; echo $?)" \
      "DEV_SLUG=_base is refused — it is the primary worktree's own state directory"
check "1" "$(dev_resolve_slug "Not A Slug" >/dev/null 2>&1; echo $?)" \
      "an illegal DEV_SLUG is refused rather than silently rewritten"
check "1" "$(dev_resolve_slug "has_underscore" >/dev/null 2>&1; echo $?)" \
      "an underscore in an explicit DEV_SLUG is refused — the same name has to work as a bucket"
check "mine" "$(dev_resolve_slug "mine")" \
      "a legal DEV_SLUG is returned unchanged"

echo "lib-devstate.sh: every half of a seed resolves the SAME stack"

# `make seed-dev` has two halves — API calls against a base URL, and psql against
# a database name — and they were resolved independently. Once a linked worktree
# stopped using :8080 and `margince`, the API records went to the worktree's
# database and the SQL fixture to the primary's. It surfaced as a NOT NULL
# violation on app_user.workspace_id, which reads as a broken fixture rather than
# as two halves pointing at two databases.
probe_state="$(mktemp -d)"
MARGINCE_DEV_STATE_DIR="$probe_state" DEV_SLUG=alpha \
    bash -c '. "'"$root"'/scripts/lib-devstate.sh"; printf "%s %s" "$(dev_app_base_url)" "$(dev_database_name)"' \
    > "$probe_state/answer"
check "http://localhost:8080 margince_dev_alpha" "$(cat "$probe_state/answer")" \
      "with no recorded stack the URL falls back to :8080 but the database is still the slug's own"

mkdir -p "$probe_state/alpha"
printf 'SLUG=alpha\nFE_PORT=8093\nAPI_PORT=18093\nREDIS_DB=70\n' >"$probe_state/alpha/env"
MARGINCE_DEV_STATE_DIR="$probe_state" DEV_SLUG=alpha \
    bash -c '. "'"$root"'/scripts/lib-devstate.sh"; printf "%s %s" "$(dev_app_base_url)" "$(dev_database_name)"' \
    > "$probe_state/answer2"
check "http://localhost:8093 margince_dev_alpha" "$(cat "$probe_state/answer2")" \
      "once the stack is recorded both halves name it — the API base takes the claimed port"
rm -rf "$probe_state"

lift port_listeners
lift read_registry
lift pick_free_db
lift pick_free_port
lift claim_stack
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
# path but that it is one absolute path per machine.
default_root="$(dev_state_root)"
default_verdict=shared
case "$default_root" in
    # Inside the checkout: per-worktree by construction.
    "$root"/*) default_verdict="inside the checkout ($default_root)" ;;
    # RELATIVE: the scripts cd to the worktree top before using it, so a relative
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

echo "dev.sh: claim_stack reserves before anything binds, and refuses rather than doubling up"

# The whole allocation path, not its parts. The unit checks above can all pass
# while the function that calls them is wired wrong — which is what happened: the
# reservation went to the machine-global registry while the run directory still
# pointed inside the worktree, so the two were different files.
rm -rf "$tmp_root"/*
claimed="$(claim_stack "alpha")"
read -r alpha_db alpha_port <<<"$claimed"
check "64" "$alpha_db" "the first stack claims the bottom of the Redis block"
check "8081" "$alpha_port" "and the bottom of the port range"
check "1" "$([ -f "$tmp_root/alpha/env" ] && echo 1 || echo 0)" \
      "the reservation is on disk BEFORE anything binds — a claim visible only once the stack is up is not a claim"

claimed="$(claim_stack "beta")"
read -r beta_db beta_port <<<"$claimed"
check "65" "$beta_db" "a second slug cannot be handed the first slug's Redis database"
check "8082" "$beta_port" "nor its port"

claimed="$(claim_stack "alpha")"
read -r again_db again_port <<<"$claimed"
check "$alpha_db $alpha_port" "$again_db $again_port" \
      "the same slug reclaims exactly what it had, so a restart keeps its URL and its streams"

# The blocks are finite. Running out must refuse rather than double up: a shared
# bus is the silent failure this whole mechanism exists to prevent. And the
# refusal must be a non-zero STATUS, because dev.sh's caller acts on that — a
# printed FAIL with a zero status let the stack start with no port at all.
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
check "1" "$(claim_stack newcomer >/dev/null 2>&1; echo $?)" \
      "claim_stack propagates that refusal as a non-zero status, which is what stops the boot"

unset MARGINCE_DEV_STATE_DIR

echo "the state home is spelled once, across every script that composes it"

# Every script that needs the state home must get it from lib-devstate.sh. This
# is here because the lifted checks above structurally cannot see it, and because
# a one-file version of this check already missed the real defect: it greppped
# dev.sh alone, so dev-logs.sh kept composing `.tmp/dev/<slug>/dev.log` and
# `make dev-logs` looked for a file `make dev` no longer wrote — with a message
# that named the wrong reason ("is the stack up?").
#
# The corpus is every script in the tree, derived rather than listed, for the
# reason the rulebook gives: a list of two paths stops describing the tree the
# first time a third script needs the state home.
stray=$(grep -rlE '^[^#]*\.tmp/dev' "$root/scripts" 2>/dev/null \
        | grep -v 'test-dev-isolation.sh' | sort || true)
check "" "$stray" \
      "no script composes .tmp/dev itself — the state home comes from lib-devstate.sh"

echo "lib-testdb.sh: the integration lane's template is per worktree"

# Lifted from lib-testdb.sh and exercised inside THROWAWAY repositories, because
# the answer depends on whether the caller sits in a primary worktree or a linked
# one, and this checkout can only ever be one of the two.
testdb="$root/scripts/lib-testdb.sh"
eval "$(awk '
    index($0, "_testdb_worktree_slug() ") == 1 { inside = 1 }
    inside                                     { print }
    inside && $0 == "}"                        { exit }
' "$testdb")"

probe="$(mktemp -d)"
git init -q "$probe/primary"
(
    cd "$probe/primary"
    git -c user.email=t@t -c user.name=t commit -q --allow-empty -m init
    git worktree add -q "$probe/linked" --detach
    # A name long enough that margince_test_<slug> would cross Postgres's 63-byte
    # identifier limit, and a second one sharing its first 49 characters.
    git worktree add -q "$probe/$(printf 'w%.0s' $(seq 1 60))-one" --detach
    git worktree add -q "$probe/$(printf 'w%.0s' $(seq 1 60))-two" --detach
)

check "" "$(cd "$probe/primary" && _testdb_worktree_slug)" \
      "a primary worktree yields no slug, so CI and the main checkout keep margince_test"
check "linked" "$(cd "$probe/linked" && _testdb_worktree_slug)" \
      "a linked worktree yields its own name, so a parallel branch cannot rebuild your template"

long_one="margince_test_$(cd "$probe/$(printf 'w%.0s' $(seq 1 60))-one" && _testdb_worktree_slug)"
long_two="margince_test_$(cd "$probe/$(printf 'w%.0s' $(seq 1 60))-two" && _testdb_worktree_slug)"
check "1" "$([ "${#long_one}" -le 63 ] && echo 1 || echo 0)" \
      "a long worktree name still yields a template name inside Postgres's 63-byte identifier limit"
check "1" "$([ "$long_one" != "$long_two" ] && echo 1 || echo 0)" \
      "two long names sharing a prefix still get different templates, so neither rebuilds the other's schema"
rm -rf "$probe"

if [ "$failures" -gt 0 ]; then
    printf 'FAIL: %d check(s) failed\n' "$failures" >&2
    exit 1
fi
printf 'OK: dev isolation checks passed\n'
