#!/usr/bin/env bash
# Prove check-migration-versions.sh fires on each defect it names, stays silent
# on the lookalikes that are not defects, and — the part that matters most —
# admits a declared baseline reset ONLY when the tree really is one.
#
# WHY THIS EXISTS. That script's `reset_admitted` decides whether the gate
# ENFORCES or merely reports. Admitting wrongly disarms a collision check while
# the run still prints OK, which is the quietly-disarmed-gate shape this repo is
# careful about, and the declaration that triggers it lives permanently in
# ci.yml. Every sibling gate here has a self-test; this one decided a verdict
# without one.
#
# Each case builds a THROWAWAY git repository with its own base commit and
# working tree, so nothing here depends on the state of the real one and the
# cases cannot interfere with each other or with a concurrent `make`.
set -uo pipefail
SELF="$(CDPATH= cd -P -- "$(dirname -- "$0")" && pwd)/$(basename -- "$0")"
cd "$(dirname "$0")/.."

GATE_SRC="$(pwd)/scripts/check-migration-versions.sh"
fails=0
case_fails=0
ran=0

# fixture <dir> — a git repo whose base commit carries three core migrations.
fixture() {
  local dir="$1"
  mkdir -p "$dir/backend/migrations/core" "$dir/scripts"
  # The gate derives its repo root from $0, so a copy has to live inside the
  # fixture; pointing the real one at a fixture directory just makes it examine
  # the real tree and report a skip, which every case would then "pass".
  cp "$GATE_SRC" "$dir/scripts/check-migration-versions.sh"
  git -C "$dir" init -q --template=
  # BEFORE the first command that can run a hook. This is the check that makes
  # the header's hermeticity claim a held one rather than a promise: whatever
  # route a caller finds to inject config, an injected core.hooksPath shows up
  # here. Placed after `commit` it fired but too late — the hook had already
  # executed, which is the whole thing it exists to prevent.
  #
  # The STATUS is judged, not just the output. `git config <key>` prints nothing
  # both when the key is unset (exit 1, the clean case) and when it cannot read
  # the config at all (a fatal, some other status) — so testing emptiness alone
  # read a broken git as a clean configuration, which is this check failing short
  # in the one direction a check must not.
  local hooks_path hooks_status
  hooks_path="$(git -C "$dir" config core.hooksPath 2>/dev/null)"
  hooks_status=$?
  case "$hooks_status" in
    0) [ -z "$hooks_path" ] || return 1 ;;
    1) : ;;
    *) return 1 ;;
  esac
  git -C "$dir" config user.email probe@example.com
  git -C "$dir" config user.name probe
  for v in 0001_alpha 0002_beta 0003_gamma; do
    printf -- '-- probe\n' > "$dir/backend/migrations/core/$v.up.sql"
    printf -- '-- probe\n' > "$dir/backend/migrations/core/$v.down.sql"
  done
  git -C "$dir" add -A
  git -C "$dir" commit -qm base
  git -C "$dir" branch -q base
  # Assert the repository is the one we meant to build. Without this, a git that
  # resolved somewhere else leaves an empty $dir and a green case: the gate finds
  # no namespace, skips, and exits 0, which is what four of the ten cases want.
  [ -d "$dir/.git" ] || return 1
  [ -f "$dir/backend/migrations/core/0001_alpha.up.sql" ] || return 1
}

# HERMETIC — and stated as the enumeration below plus a CHECK, not as a promise.
# This header has twice claimed "nothing the caller exported decides a verdict
# here" and twice been wrong (the gate's own switch, then GIT_CONFIG_COUNT). So
# the variables it disarms are listed, and `fixture` verifies the OUTCOME on the
# repository it builds rather than trusting that the list is complete.
#
# The gate's own switch, first. ci.yml sets MIGRATION_VERSIONS_BASELINE_RESET=1
# on the `make check-backend` STEP, which is where this target runs, so every
# case below that expects the gate to ENFORCE inherited the declaration, ran in
# REPORTING mode, and saw exit 0 where it wanted exit 1. It failed in CI and
# passed on a developer's machine, which is the wrong way round. That step is now
# this line's standing regression test: it is the one environment that sets the
# variable, so removing the unset goes red on the next push.
unset MIGRATION_VERSIONS_BASELINE_RESET
unset MIGRATION_VERSIONS_REQUIRE_BASE

# git's environment, second, and this half is about damage rather than a wrong
# verdict. `git -C "$dir"` changes the WORKING DIRECTORY only — it does not
# override GIT_DIR. With GIT_DIR exported, `git init` never creates $dir/.git and
# every command below lands in the surrounding repository instead: the identity
# writes overwrite its local user.name/user.email, and `commit` puts a real
# commit on its current branch deleting the tracked tree. `rm -rf "$dir"` then
# takes away the temp dir and leaves that behind, and `fixture` returns 0, so
# nothing reports it.
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_CEILING_DIRECTORIES

# The caller's git CONFIG, third. `git init` honours a global init.templateDir,
# which copies hooks into the fixture that `git commit` then EXECUTES, and a
# global absolute core.hooksPath runs the developer's real hooks against a temp
# directory. (The repo-local core.hooksPath the Makefile sets is relative, so it
# cannot reach out of the fixture.)
#
# /dev/null for both scopes is NOT sufficient on its own, which is why the count
# variables are here too: GIT_CONFIG_COUNT with GIT_CONFIG_KEY_n/VALUE_n injects
# settings that survive both scopes being empty. Measured — with COUNT=1 and
# KEY_0=core.hooksPath, `git config core.hooksPath` inside a fresh fixture
# returns the caller's path and the injected pre-commit hook RUNS. Unsetting
# COUNT is what disarms the indexed pairs; GIT_CONFIG_PARAMETERS is the internal
# spelling of the same thing.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_TERMINAL_PROMPT=0
unset GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS

# expect <name> <want-exit> <declared-reset> <mutation-fn> <want-diagnostic>
#
# The DIAGNOSTIC is required, not decoration. An exit code alone makes every
# negative case satisfied by any failure at all: a gate broken so that it dies on
# an unset variable exits 1 for all six of them, and this harness would report
# ten green cases while checking nothing. So each case also names a string the
# gate's own output must contain, which ties the verdict to the defect the case
# planted rather than to the fact that something went wrong.
expect() {
  local name="$1" want="$2" declared="$3" mutate="$4" diagnostic="$5"
  ran=$((ran + 1))
  local dir
  dir="$(mktemp -d)"

  # Setup and mutation failures are FAILURES, not silence. Unchecked, a fixture
  # that failed to build or a mutation that planted nothing leaves a clean tree
  # behind — and a clean tree passes, so an expect-0 case would go green having
  # exercised none of what it names.
  if ! fixture "$dir" >/dev/null 2>&1; then
    printf 'FAIL  %s\n      building the fixture failed, so nothing was tested\n' "$name" >&2
    rm -rf "$dir"
    fails=$((fails + 1))
    case_fails=$((case_fails + 1))
    return
  fi
  if ! "$mutate" "$dir" >/dev/null 2>&1; then
    printf 'FAIL  %s\n      the mutation %s failed, so the planted defect is not there\n' \
      "$name" "$mutate" >&2
    rm -rf "$dir"
    fails=$((fails + 1))
    case_fails=$((case_fails + 1))
    return
  fi

  local out rc=0
  if [ "$declared" = "declared" ]; then
    out="$(cd "$dir" && MIGRATION_VERSIONS_BASELINE_RESET=1 ./scripts/check-migration-versions.sh base 2>&1)" || rc=$?
  else
    out="$(cd "$dir" && ./scripts/check-migration-versions.sh base 2>&1)" || rc=$?
  fi
  rm -rf "$dir"

  if [ "$rc" -ne "$want" ]; then
    printf 'FAIL  %s\n      exit %s, want %s\n' "$name" "$rc" "$want" >&2
    printf '%s\n' "$out" | sed 's/^/      | /' >&2
    fails=$((fails + 1))
    case_fails=$((case_fails + 1))
    return
  fi
  if ! grep -qF -- "$diagnostic" <<<"$out"; then
    printf 'FAIL  %s\n      exit %s was right, but the gate never said %s —\n' \
      "$name" "$rc" "'$diagnostic'" >&2
    printf '      so this case was satisfied by some OTHER outcome than the one it planted\n' >&2
    printf '%s\n' "$out" | sed 's/^/      | /' >&2
    fails=$((fails + 1))
    case_fails=$((case_fails + 1))
    return
  fi
  printf 'ok    %s\n' "$name"
}

# ---- the mutations -------------------------------------------------------

untouched() { :; }

# A migration stamped above the base: the normal, correct addition.
adds_above() {
  printf -- '-- probe\n' > "$1/backend/migrations/core/1799999999_new.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/1799999999_new.down.sql"
}

# Two different migrations at one version — the outage this gate was built for.
collides() {
  git -C "$1" rm -q "backend/migrations/core/0002_beta.up.sql" "backend/migrations/core/0002_beta.down.sql" || return 1
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_different.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_different.down.sql"
}

# A new migration numbered BELOW the base's highest: applied in a different
# place on a fresh database than on one already past it.
sorts_below() {
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002x_late.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002x_late.down.sql"
}

# One shipped migration re-stamped under a new version and left everything
# else alone: the base's own outage-report case, run backwards. The new
# version sorts above base_max, so on its own it reads as a normal addition —
# the defect is what it leaves behind, not what it adds.
renumbers_one() {
  git -C "$1" rm -q "backend/migrations/core/0002_beta.up.sql" "backend/migrations/core/0002_beta.down.sql" || return 1
  printf -- '-- probe\n' > "$1/backend/migrations/core/1799999999_beta.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/1799999999_beta.down.sql"
}

# A genuine consolidation: every version replaced, and FEWER of them.
consolidates() {
  git -C "$1" rm -q backend/migrations/core/0001_alpha.up.sql \
    backend/migrations/core/0001_alpha.down.sql \
    backend/migrations/core/0002_beta.up.sql \
    backend/migrations/core/0002_beta.down.sql \
    backend/migrations/core/0003_gamma.up.sql \
    backend/migrations/core/0003_gamma.down.sql || return 1
  mkdir -p "$1/backend/migrations/core"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0001_baseline.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0001_baseline.down.sql"
}

# A broken rebase wearing the same declaration: one migration survives at the
# version AND name the base has, so it is not a consolidation.
partial_rewrite() {
  git -C "$1" rm -q backend/migrations/core/0002_beta.up.sql \
    backend/migrations/core/0002_beta.down.sql \
    backend/migrations/core/0003_gamma.up.sql \
    backend/migrations/core/0003_gamma.down.sql || return 1
  mkdir -p "$1/backend/migrations/core"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_baseline.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_baseline.down.sql"
}

# Renaming a namespace's files one-for-one: shares no version, but does not
# collapse, so it is a rebase accident rather than a consolidation.
renames_without_collapsing() {
  # Every status propagated. A `git mv` that failed left the tree unchanged while
  # the function still returned 0, so the "mutation failed" guard could not see
  # it and the case ran against a clean fixture — which passes.
  for pair in 0001_alpha:0001_one 0002_beta:0002_two 0003_gamma:0003_three; do
    git -C "$1" mv "backend/migrations/core/${pair%%:*}.up.sql" "backend/migrations/core/${pair##*:}.up.sql" || return 1
    git -C "$1" mv "backend/migrations/core/${pair%%:*}.down.sql" "backend/migrations/core/${pair##*:}.down.sql" || return 1
  done
}

# One version claimed twice in the working tree: the namespace will not load.
duplicates_in_tree() {
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_twin.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_twin.down.sql"
}

# The base already has the outage — one version claimed twice — and this
# branch is the repair: it keeps one claimant at its version and renumbers the
# other above base_max. The vanished-version check must read this as "0002
# still exists" (beta survives), not flag beta as vanished because base's two
# 0002 rows and the tree's one no longer line up one-for-one.
repairs_a_base_collision() {
  local dir="$1"
  printf -- '-- probe\n' > "$dir/backend/migrations/core/0002_twin.up.sql"
  printf -- '-- probe\n' > "$dir/backend/migrations/core/0002_twin.down.sql"
  git -C "$dir" add -A
  git -C "$dir" commit -qm 'base gains a collision' || return 1
  git -C "$dir" branch -qf base
  git -C "$dir" rm -q backend/migrations/core/0002_twin.up.sql backend/migrations/core/0002_twin.down.sql || return 1
  printf -- '-- probe\n' > "$dir/backend/migrations/core/1799999999_twin.up.sql"
  printf -- '-- probe\n' > "$dir/backend/migrations/core/1799999999_twin.down.sql"
}

# A namespace this branch empties entirely: every migration the base carries
# in it is gone from the tree, and the outer walk must still find the
# namespace to compare against — a tree-only walk would skip it before the
# vanished-version check ever ran.
empties_a_namespace() {
  git -C "$1" rm -q backend/migrations/core/0001_alpha.up.sql \
    backend/migrations/core/0001_alpha.down.sql \
    backend/migrations/core/0002_beta.up.sql \
    backend/migrations/core/0002_beta.down.sql \
    backend/migrations/core/0003_gamma.up.sql \
    backend/migrations/core/0003_gamma.down.sql || return 1
}

# The base moves on while this branch sits: another PR's migration lands on
# base AFTER the fork point. The branch's tree lacks it, but never contained
# it either — the merge keeps it, so the vanished-version check must read the
# absence as staleness, not as a rename, or every migration landing on main
# fails every open branch until it rebases.
base_gains_unrelated() {
  local dir="$1"
  git -C "$dir" checkout -q base || return 1
  printf -- '-- probe\n' > "$dir/backend/migrations/core/1799999998_landed.up.sql"
  printf -- '-- probe\n' > "$dir/backend/migrations/core/1799999998_landed.down.sql"
  git -C "$dir" add backend/migrations/core || return 1
  git -C "$dir" commit -qm 'another PR lands a migration' || return 1
  git -C "$dir" checkout -q - || return 1
}

# The same moved-on base, but this branch ALSO adds a migration — stamped below
# the version that landed after the fork. The fork point scopes only the
# vanished check: ordering is still judged against the base's TIP, because a
# database that applied the landed migration would otherwise get this one in
# the wrong place. This is the case that fails if the fork scoping ever leaks
# into the ordering comparison.
stale_branch_adds_below_tip() {
  base_gains_unrelated "$1" || return 1
  printf -- '-- probe\n' > "$1/backend/migrations/core/1799999997_late.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/1799999997_late.down.sql"
}

# A namespace this branch introduces that the base has never heard of:
# `migrations_at_base` legitimately finds nothing in it, and that "nothing"
# must read as zero migrations to sort after, not as a shell error.
adds_a_namespace() {
  mkdir -p "$1/backend/migrations/extra"
  printf -- '-- probe\n' > "$1/backend/migrations/extra/1799999999_new.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/extra/1799999999_new.down.sql"
}

# ---- the cases -----------------------------------------------------------

expect "an untouched tree passes" \
  0 plain    untouched            "OK: check-migration-versions"
expect "a migration stamped above the base passes" \
  0 plain    adds_above           "OK: check-migration-versions"
expect "two migrations at one version fail" \
  1 plain    collides             "is claimed by two different migrations"
expect "a migration sorting below the base fails" \
  1 plain    sorts_below          "sorts at or below"
expect "one version claimed twice in the tree fails" \
  1 plain    duplicates_in_tree   "declares one version twice"
expect "a consolidation fails when NOT declared" \
  1 plain    consolidates         "is claimed by two different migrations"
expect "a renumbered migration fails when not declared" \
  1 plain    renumbers_one        "but this branch no longer has it"
expect "a repair that keeps one collision claimant is not read as vanished" \
  0 plain    repairs_a_base_collision "repairing, not colliding"
expect "emptying a namespace still reports its vanished versions" \
  1 plain    empties_a_namespace  "but this branch no longer has it"
expect "a namespace new to the base passes" \
  0 plain    adds_a_namespace     "OK: check-migration-versions"
expect "a migration landing on base after the fork is not read as vanished" \
  0 plain    base_gains_unrelated "OK: check-migration-versions"
expect "the moved-on base still orders what this branch adds" \
  1 plain    stale_branch_adds_below_tip "sorts at or below"

# The four that decide whether the escape hatch is a gate or a hole.
expect "a declared consolidation is admitted" \
  0 declared consolidates         "a baseline reset was declared, and admitted"
expect "a declared reset with a survivor is refused" \
  1 declared partial_rewrite      "at the version AND name the base has"
expect "a declared rename that does not collapse is refused" \
  1 declared renames_without_collapsing "does not collapse"
# A REAL collision, declared. The previous version of this case ran the
# `untouched` mutation, so there was no collision in it to excuse and no finding
# either way: it asserted exit 0 against a tree byte-identical to the base and
# proved nothing about the escape hatch at all. `collides` leaves core sharing
# versions with the base, so the declaration is refused and the collision is
# enforced — which is the claim the name has always made.
expect "a declared reset does not excuse a real collision" \
  1 declared collides             "is claimed by two different migrations"

# ---- census --------------------------------------------------------------
# TWO guards, because one of them cannot see the other's defect.
#
# The derived comparison catches an `expect` line that is present but does not
# RUN — guarded away, unreachable, or lost to an early return. It cannot catch a
# case being DELETED: the deletion lowers `ran` and the grep count together, so
# they still agree and the suite reports a smaller green run. The first version
# of this census claimed to catch exactly that, which was simply untrue.
#
# So the total is also pinned. A literal here is not duplication of the case
# list: it is the one fact the case list cannot state about itself, which is how
# many of it there should be.
expected_cases=16

declared_cases="$(grep -c '^expect "' "$SELF")"
if [ "$ran" -ne "$declared_cases" ]; then
  printf 'FAIL  ran %s case(s) but %s are declared — a case stopped running\n' \
    "$ran" "$declared_cases" >&2
  fails=$((fails + 1))
fi
if [ "$declared_cases" -ne "$expected_cases" ]; then
  printf 'FAIL  %s case(s) declared, %s expected — a case was added or removed.\n' \
    "$declared_cases" "$expected_cases" >&2
  printf '      Update expected_cases in the same change, so a deletion cannot pass as a smaller green run.\n' >&2
  fails=$((fails + 1))
fi

# case_fails and fails are counted apart, because a census failure is not a case
# failure: reporting "1 of 10 case(s) failed" when all ten passed sends the
# reader looking through the cases for a defect that is in the accounting.
if [ "$fails" -ne 0 ]; then
  if [ "$case_fails" -ne 0 ]; then
    printf 'FAIL: test-migration-versions — %s of %s case(s) failed\n' "$case_fails" "$ran" >&2
  fi
  if [ "$fails" -ne "$case_fails" ]; then
    printf 'FAIL: test-migration-versions — the case census above did not add up\n' >&2
  fi
  exit 1
fi
printf 'OK: test-migration-versions — %s case(s), including every branch of the baseline-reset declaration\n' "$ran"
