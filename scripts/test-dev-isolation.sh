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

# No recorded stack, and a slug: there is no URL to fall back to. :8080 belongs to
# a DIFFERENT stack, so answering it would send the API half of a seed there while
# the database half goes to margince_dev_<slug> — the split this pair exists to
# close. Refusing is the only honest answer.
check "1" "$(MARGINCE_DEV_STATE_DIR="$probe_state" DEV_SLUG=alpha \
    bash -c '. "'"$root"'/scripts/lib-devstate.sh"; dev_app_base_url' >/dev/null 2>&1; echo $?)" \
      "with no recorded stack a linked worktree gets a refusal, not :8080"

# The PRIMARY worktree is the exception, and not by special-casing: its stack is
# the shared one and its port is fixed, so :8080 is the answer rather than a guess.
# Checked from a real primary repository — DEV_SLUG='' cannot fake it, because an
# empty value is exactly what makes dev_resolve_slug ask the worktree.
primary_probe="$(mktemp -d)"
git init -q "$primary_probe/repo"
check "http://localhost:8080" "$(cd "$primary_probe/repo" && \
    MARGINCE_DEV_STATE_DIR="$probe_state" DEV_SLUG='' \
    bash -c '. "'"$root"'/scripts/lib-devstate.sh"; dev_app_base_url')" \
      "the primary worktree still answers :8080 with nothing recorded"
rm -rf "$primary_probe"

mkdir -p "$probe_state/alpha"
printf 'SLUG=alpha\nFE_PORT=8093\nAPI_PORT=18093\nREDIS_DB=70\n' >"$probe_state/alpha/env"
MARGINCE_DEV_STATE_DIR="$probe_state" DEV_SLUG=alpha \
    bash -c '. "'"$root"'/scripts/lib-devstate.sh"; printf "%s %s" "$(dev_app_base_url)" "$(dev_database_name)"' \
    > "$probe_state/answer2"
check "http://localhost:8093 margince_dev_alpha" "$(cat "$probe_state/answer2")" \
      "once the stack is recorded both halves name it — the API base takes the claimed port"
rm -rf "$probe_state"

echo "Makefile: a target that drives a live stack names it through the helpers"

# The check above proves the HELPERS agree. It cannot prove anybody calls them,
# and for a long time the demo seeder did not: `make seed-demo` carried :8080,
# `margince` and `margince-dev` as literals, so from a linked worktree all three
# halves — records, SQL and object bytes — went to the PRIMARY worktree's stack.
# Nothing failed, because that stack answers whoever asks.
#
# So the corpus is READ from the Makefile rather than listed here: a recipe that
# reaches a running stack is one that hands the seeder or Playwright a base URL,
# and any future one is caught by the same scan. Listing the four known targets
# would be a second copy of the Makefile, and would report PASS over a fifth.
literals='localhost:8080|margince_owner:dev@[^"]*/margince"|BLOBSTORE_BUCKET=margince-dev( |$)'
offenders="$(awk '
    # A recipe line is TAB-indented; a target line is not. Track which recipe
    # each line belongs to so the report names the target a reader must fix.
    /^[a-zA-Z0-9_.-]+:/ { target = $0; sub(/:.*/, "", target) }
    /^\t/               { print target "\t" $0 }
' "$root/Makefile" | grep -Ei "$literals" | grep -v '^help\t' || true)"
check "" "$offenders" \
      "no recipe hardcodes the primary stack's URL, database or bucket — they come from lib-devstate.sh"

# And the scan can still SEE one. A census that only ever reports clean is
# indistinguishable from a census that stopped reading, so plant the defect that
# was actually shipped and require the pattern to catch it.
planted="$(printf 'seed-demo:\n\tBASE_URL=http://localhost:8080 pnpm e2e\n' \
    | awk '/^[a-zA-Z0-9_.-]+:/ { t = $0; sub(/:.*/, "", t) } /^\t/ { print t "\t" $0 }' \
    | grep -Ec "$literals" || true)"
check "1" "$planted" \
      "the scan recognises a hardcoded :8080 in a recipe — otherwise the clean result above means nothing"

echo "reaching another stack is all three of it or none"

# A stack is an API base, a database and a bucket. Overriding one and letting the
# other two resolve from this worktree is the same split the scan above exists to
# catch, arriving by the front door: each half succeeds, so nothing reports it.
#
# The rule is tested where it LIVES, not through `make seed-demo`: a check that
# drove the recipe needed the demo dataset and a reachable API, so in CI it died
# before reaching the rule and reported the rule broken.
check "none" "$(dev_seed_override "" "" "")" \
      "no override at all resolves this worktree's stack, as every seed does by default"
check "all" "$(dev_seed_override "postgres://x/other" "http://localhost:9" "other-bucket")" \
      "all three together name the stack they say — the flags exist to be used"
for partial in "dsn:::" ":api::" "::bucket:"; do
    dsn="$(printf '%s' "$partial" | cut -d: -f1)"
    api="$(printf '%s' "$partial" | cut -d: -f2)"
    bucket="$(printf '%s' "$partial" | cut -d: -f3)"
    check "1" "$(dev_seed_override "$dsn" "$api" "$bucket" >/dev/null 2>&1; echo $?)" \
          "a partial override ($partial) is refused rather than splitting one seed across two stacks"
done

# The seeder takes the API base as a flag too, so the same split arrives through
# SEED_ARGS: `-api` there lands after the resolved one and wins, while the
# database and the bucket stay this worktree's. A pass-through nobody reads is
# how the three-name rule above was walked around.
for flag in "-api http://other" "-api=http://other" "--api http://other" "-limit 5 -api http://other"; do
    check "1" "$(dev_seed_override "" "" "" "$flag" >/dev/null 2>&1; echo $?)" \
          "SEED_ARGS='$flag' is refused rather than moving the API leg alone"
done
check "none" "$(dev_seed_override "" "" "" "-dry-run -limit 5")" \
      "the flags a seed run actually passes are still passed — the check is about -api, not about SEED_ARGS"

# And the Makefile still ASKS, with the flags. The rule refusing in a library
# nothing calls is the same as no rule, and this is the seam the recipes go
# through; a prelude that dropped the fourth argument would silently stop
# checking the flag path while every case above kept passing.
check "1" "$(grep -q 'seed_override=.*dev_seed_override.*SEED_ARGS' "$root/Makefile" && echo 1 || echo 0)" \
      "the seed prelude asks the library about the pass-through flags too"

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
# dev.sh sets this in the shell that calls claim_stack — the reservation records
# who is starting the stack, so a run that loses the port race cannot delete a
# claim another run is still booting against.
export STACK_STARTER_PID=$$
claimed="$(claim_stack "alpha")"
read -r alpha_db alpha_port <<<"$claimed"
check "64" "$alpha_db" "the first stack claims the bottom of the Redis block"
check "8081" "$alpha_port" "and the bottom of the port range"
check "1" "$([ -f "$tmp_root/alpha/env" ] && echo 1 || echo 0)" \
      "the reservation is on disk BEFORE anything binds — a claim visible only once the stack is up is not a claim"
check "1" "$(grep -c "^STARTER_PID=$$\$" "$tmp_root/alpha/env")" \
      "and it records who is starting it, which is what stops a losing run deleting a winner's claim"

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

echo "dev.sh: a stack's servers are findable when the state file no longer names them"

# The defect: the state file records ONE worker pid and every `make dev`
# overwrites it. An earlier run's worker then belonged to no record, and since a
# worker binds no port, the port backstop that catches the api could not see it
# either — so `make dev-stop` reported success over a worker it left running.
# Four accumulated against one database. They share a River leader election, and
# the leader is what inserts the periodic jobs: the oldest one held the lease and
# scheduled only the job kinds its own binary knew, so a job kind added later was
# never enqueued once. Every other lane kept running, which is what made it read
# as a broken feature rather than as a process nobody had stopped.
#
# Lifted, so these checks exercise the matcher production calls.
lift stack_server_pids

# Real processes with real command lines: the matcher reads `ps`, so a stub would
# prove only that the stub matches itself. `exec -a` renames a `sleep` into an
# argv shaped like a worker's, which is the cheapest honest subject.
# Started in THIS shell, not through a command substitution: `pid=$(fake …)`
# runs the function in a subshell, and the background child is reparented and
# lost the moment that subshell exits. The pid then names nothing and every
# check below reads as "no match" — which is why the liveness check above is the
# first assertion rather than an afterthought.
fake_server() { # dsn redis — sets REPLY to the pid
    bash -c 'exec -a "./bin/worker --dsn '"$1"' --redis '"$2"'" sleep 30' &
    REPLY=$!
}
# The two DSNs differ ONLY in the database segment, and the primary's name is a
# PREFIX of the linked worktree's — which is what production hands out
# (`margince` and `margince_dev_<slug>` off one APP_DSN). An earlier fixture pair
# that differed before that segment let a substring match pass every check here
# while `make dev-stop` on the primary worktree killed every linked worktree's
# servers at once.
mine_dsn="postgres://margince_app:pw@localhost:15432/margince"
theirs_dsn="postgres://margince_app:pw@localhost:15432/margince_dev_band"
fake_server "$mine_dsn" "localhost:16379/0";    mine=$REPLY
fake_server "$theirs_dsn" "localhost:16379/64"; theirs=$REPLY
# The exec'd argv has to be visible to ps before the matcher reads it. Poll for
# it rather than sleeping a guessed interval.
for _ in 1 2 3 4 5 6 7 8 9 10; do
    [ -n "$(ps -o command= -p "$mine" 2>/dev/null)" ] && break
    sleep 0.1
done
check "1" "$([ -n "$(ps -o command= -p "$mine" 2>/dev/null)" ] && echo 1 || echo 0)" \
      "the fake worker is running — without this the four checks below would pass on an empty machine"

check "$mine" "$(stack_server_pids "$mine_dsn" "localhost:16379/0" | sort -u)" \
      "this stack's worker is found by its database and its logical Redis database, with no pid on file"
check "" "$(stack_server_pids "$mine_dsn" "localhost:16379/0" | grep -x "$theirs" || true)" \
      "a parallel worktree's worker is NOT swept — it holds a different database and a different logical database"
check "$theirs" "$(stack_server_pids "$theirs_dsn" "localhost:16379/64" | sort -u)" \
      "and that worktree can still stop its own"
# Two stacks against the shared `margince` differ ONLY in the logical database,
# so a matcher reading the DSN alone passes every check above and still sweeps a
# peer. This is the case that separates them.
check "" "$(stack_server_pids "$mine_dsn" "localhost:16379/7" | grep -x "$mine" || true)" \
      "matching the database alone is not enough — the logical Redis database is what separates two stacks on one database"

# The missing-state case, which is the one a linked worktree hits. With no claim
# to read, the registry answers logical database 0 for EVERY slug — the primary
# stack's — so a cleanup that insisted on the Redis half would ask for
# margince_dev_band on db 0, match nothing, and report success over servers it
# left running. The Redis argument is omitted there instead of guessed.
check "$theirs" "$(stack_server_pids "$theirs_dsn" | sort -u)" \
      "a linked worktree's servers are found with no Redis address — its database name already names the stack"
check "" "$(stack_server_pids "$theirs_dsn" "localhost:16379/0" | grep -x "$theirs" || true)" \
      "and guessing db 0 for it would have found nothing, which is the miss that made omitting it necessary"

# The dangerous direction, and the one the checks above cannot reach: the PRIMARY
# worktree, whose database name is a prefix of every linked worktree's. Asked
# with no Redis address, because that is what the missing-state branch passes —
# so nothing else narrows the match. A substring test answers both pids here and
# one `make dev-stop` takes down every parallel session's stack.
check "$mine" "$(stack_server_pids "$mine_dsn" | sort -u)" \
      "the primary worktree's cleanup finds ONLY its own — margince must not match margince_dev_band"
# The same boundary on the Redis half: /6 is a prefix of /64, and the block runs
# 64..79, so every two-digit logical database has a single-digit prefix.
check "" "$(stack_server_pids "$theirs_dsn" "localhost:16379/6" | grep -x "$theirs" || true)" \
      "a logical database matches whole — /6 is not /64"
kill "$mine" "$theirs" 2>/dev/null || true
wait "$mine" "$theirs" 2>/dev/null || true

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

check "" "$(cd "$probe/primary" || exit 1; _testdb_worktree_slug)" \
      "a primary worktree yields no slug, so CI and the main checkout keep margince_test"
check "linked" "$(cd "$probe/linked" || exit 1; _testdb_worktree_slug)" \
      "a linked worktree yields its own name, so a parallel branch cannot rebuild your template"

# dev_derive_slug, in the same throwaway repositories. An empty answer means THE
# PRIMARY worktree, so a linked worktree must never produce one: a name that
# sanitises away to nothing would put it on the shared margince database and
# :8080 — the collision this mechanism removes, arrived at from the other end.
# `_` is such a name: the underscore folds to a hyphen and the hyphen is trimmed.
git -C "$probe/primary" worktree add -q "$probe/_" --detach
check "" "$(dev_sanitize_slug "_")" \
      "the sanitiser really does reduce this name to nothing, which is what makes the next check the real case"
check "1" "$([ -n "$(cd "$probe/_" || exit 1; dev_derive_slug)" ] && echo 1 || echo 0)" \
      "a name that sanitises to nothing still yields a slug, never the empty answer that means primary"

# Two worktrees whose basenames are identical must not share one slug: that would
# be one database, one Redis logical database and one bucket between two stacks.
mkdir -p "$probe/a" "$probe/b"
git -C "$probe/primary" worktree add -q "$probe/a/dup" --detach
git -C "$probe/primary" worktree add -q "$probe/b/dup" --detach
check "1" "$([ "$(cd "$probe/a/dup" || exit 1; dev_derive_slug)" != "$(cd "$probe/b/dup" || exit 1; dev_derive_slug)" ] && echo 1 || echo 0)" \
      "two worktrees with the SAME basename get different slugs, so they cannot share a database"

# And two whose names share only their FIRST 24 CHARACTERS. The readable half of a
# slug is truncated to fit the database and bucket names built from it, so
# uniqueness cannot depend on it — it has to come from the digest. Hashing the
# clone alone was a regression exactly here: every worktree of one clone shares
# that, so two long siblings folded onto one slug.
git -C "$probe/primary" worktree add -q "$probe/$(printf 'p%.0s' $(seq 1 30))-alpha" --detach
git -C "$probe/primary" worktree add -q "$probe/$(printf 'p%.0s' $(seq 1 30))-beta" --detach
long_a="$(cd "$probe/$(printf 'p%.0s' $(seq 1 30))-alpha" || exit 1; dev_derive_slug)"
long_b="$(cd "$probe/$(printf 'p%.0s' $(seq 1 30))-beta" || exit 1; dev_derive_slug)"
check "1" "$([ "${long_a:0:24}" = "${long_b:0:24}" ] && echo 1 || echo 0)" \
      "the two names really do share their first 24 characters, which is what makes the next check the real case"
check "1" "$([ "$long_a" != "$long_b" ] && echo 1 || echo 0)" \
      "and they still get different slugs — uniqueness lives in the digest, not the truncated name"

# A slug must be STABLE for as long as a stack runs. An earlier version appended
# the digest only when a twin existed, so creating a second worktree of the same
# name RENAMED a running stack — and dev-stop then resolved a slug with no record
# and could not stop it. The slug must not depend on what else exists.
slug_before="$(cd "$probe/linked" || exit 1; dev_derive_slug)"
git -C "$probe/primary" worktree add -q "$probe/c/linked" --detach
check "$slug_before" "$(cd "$probe/linked" || exit 1; dev_derive_slug)" \
      "adding a same-named worktree does NOT rename an existing one — a running stack stays stoppable"

# And moving the checkout does not rename it either. The slug comes from the
# worktree's ADMIN name under <common>/worktrees/, which git keeps across
# `git worktree move` — a path-derived slug changed here, so relocating a checkout
# renamed a running stack and dev-stop then missed its claim.
git -C "$probe/primary" worktree move "$probe/linked" "$probe/linked-moved"
check "$slug_before" "$(cd "$probe/linked-moved" || exit 1; dev_derive_slug)" \
      "moving a worktree does NOT rename it, so a running stack survives the move"

# The slug becomes margince_dev_<slug> (Postgres caps an identifier at 63 bytes)
# and margince-dev-<slug> (S3 caps a bucket name at 63), so it has to leave room
# for both prefixes however long the directory name is.
long_slug="$(cd "$probe/$(printf 'w%.0s' $(seq 1 60))-one" || exit 1; dev_derive_slug)"
check "1" "$([ "${#long_slug}" -le 40 ] && echo 1 || echo 0)" \
      "a long directory name still yields a slug short enough for the database and bucket names built from it"
case "$long_slug" in
    www*) recognisable=1 ;;
    *)    recognisable="digest only ($long_slug)" ;;
esac
check "1" "$recognisable" \
      "and it still opens with the worktree's own name — a digest-only slug would pass a length check and tell a reader nothing"

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
