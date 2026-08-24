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
ran=0

# fixture <dir> — a git repo whose base commit carries three core migrations.
fixture() {
  local dir="$1"
  mkdir -p "$dir/backend/migrations/core" "$dir/scripts"
  # The gate derives its repo root from $0, so a copy has to live inside the
  # fixture; pointing the real one at a fixture directory just makes it examine
  # the real tree and report a skip, which every case would then "pass".
  cp "$GATE_SRC" "$dir/scripts/check-migration-versions.sh"
  git -C "$dir" init -q
  git -C "$dir" config user.email probe@example.com
  git -C "$dir" config user.name probe
  for v in 0001_alpha 0002_beta 0003_gamma; do
    printf -- '-- probe\n' > "$dir/backend/migrations/core/$v.up.sql"
    printf -- '-- probe\n' > "$dir/backend/migrations/core/$v.down.sql"
  done
  git -C "$dir" add -A
  git -C "$dir" commit -qm base
  git -C "$dir" branch -q base
}

# HERMETIC BY CONSTRUCTION: nothing the caller exported decides a verdict here.
#
# This line is not defensive tidiness — the harness shipped without it and went
# green having proved nothing. CI declares MIGRATION_VERSIONS_BASELINE_RESET=1
# for the whole deterministic-gates job (ci.yml sets it at job level so the
# consolidation could land), so every case below that expects the gate to ENFORCE
# inherited the declaration, ran in REPORTING mode, and saw exit 0 where it
# wanted exit 1. It failed in CI and passed on a developer's machine, which is
# the wrong way round for a gate.
#
# The deterministic-gates job is therefore this line's standing regression test:
# it is the one environment that has the variable set, so if the unset is ever
# removed, CI says so on the next push.
unset MIGRATION_VERSIONS_BASELINE_RESET

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
    return
  fi
  if ! "$mutate" "$dir" >/dev/null 2>&1; then
    printf 'FAIL  %s\n      the mutation %s failed, so the planted defect is not there\n' \
      "$name" "$mutate" >&2
    rm -rf "$dir"
    fails=$((fails + 1))
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
    return
  fi
  if ! printf '%s' "$out" | grep -qF -- "$diagnostic"; then
    printf 'FAIL  %s\n      exit %s was right, but the gate never said %s —\n' \
      "$name" "$rc" "'$diagnostic'" >&2
    printf '      so this case was satisfied by some OTHER outcome than the one it planted\n' >&2
    printf '%s\n' "$out" | sed 's/^/      | /' >&2
    fails=$((fails + 1))
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
  git -C "$1" rm -q "backend/migrations/core/0002_beta.up.sql" "backend/migrations/core/0002_beta.down.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_different.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_different.down.sql"
}

# A new migration numbered BELOW the base's highest: applied in a different
# place on a fresh database than on one already past it.
sorts_below() {
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002x_late.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002x_late.down.sql"
}

# A genuine consolidation: every version replaced, and FEWER of them.
consolidates() {
  git -C "$1" rm -q backend/migrations/core/0001_alpha.up.sql \
    backend/migrations/core/0001_alpha.down.sql \
    backend/migrations/core/0002_beta.up.sql \
    backend/migrations/core/0002_beta.down.sql \
    backend/migrations/core/0003_gamma.up.sql \
    backend/migrations/core/0003_gamma.down.sql
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
    backend/migrations/core/0003_gamma.down.sql
  mkdir -p "$1/backend/migrations/core"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_baseline.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_baseline.down.sql"
}

# Renaming a namespace's files one-for-one: shares no version, but does not
# collapse, so it is a rebase accident rather than a consolidation.
renames_without_collapsing() {
  local core="$1/backend/migrations/core"
  for pair in 0001_alpha:0001_one 0002_beta:0002_two 0003_gamma:0003_three; do
    git -C "$1" mv "backend/migrations/core/${pair%%:*}.up.sql" "backend/migrations/core/${pair##*:}.up.sql"
    git -C "$1" mv "backend/migrations/core/${pair%%:*}.down.sql" "backend/migrations/core/${pair##*:}.down.sql"
  done
  : "$core"
}

# One version claimed twice in the working tree: the namespace will not load.
duplicates_in_tree() {
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_twin.up.sql"
  printf -- '-- probe\n' > "$1/backend/migrations/core/0002_twin.down.sql"
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
expected_cases=10

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

if [ "$fails" -ne 0 ]; then
  printf 'FAIL: test-migration-versions — %s of %s case(s) failed\n' "$fails" "$ran" >&2
  exit 1
fi
printf 'OK: test-migration-versions — %s case(s), including every branch of the baseline-reset declaration\n' "$ran"
