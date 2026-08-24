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

# expect <name> <want-exit> <declared-reset> <mutation-fn>
expect() {
  local name="$1" want="$2" declared="$3" mutate="$4"
  ran=$((ran + 1))
  local dir
  dir="$(mktemp -d)"
  fixture "$dir"
  "$mutate" "$dir"

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

expect "an untouched tree passes"                        0 plain    untouched
expect "a migration stamped above the base passes"       0 plain    adds_above
expect "two migrations at one version fail"              1 plain    collides
expect "a migration sorting below the base fails"        1 plain    sorts_below
expect "one version claimed twice in the tree fails"     1 plain    duplicates_in_tree
expect "a consolidation fails when NOT declared"         1 plain    consolidates

# The four that decide whether the escape hatch is a gate or a hole.
expect "a declared consolidation is admitted"            0 declared consolidates
expect "a declared reset with a survivor is refused"     1 declared partial_rewrite
expect "a declared rename that does not collapse is refused" 1 declared renames_without_collapsing
expect "a declared reset does not excuse a real collision in an untouched namespace" \
                                                         0 declared untouched

# ---- census --------------------------------------------------------------
# The number of cases is pinned, so a case deleted or an `expect` line that
# stops running reports here rather than as a smaller green run.
declared_cases="$(grep -c '^expect "' "$SELF")"
if [ "$ran" -ne "$declared_cases" ]; then
  printf 'FAIL  ran %s case(s) but %s are declared — a case stopped running\n' \
    "$ran" "$declared_cases" >&2
  fails=$((fails + 1))
fi

if [ "$fails" -ne 0 ]; then
  printf 'FAIL: test-migration-versions — %s of %s case(s) failed\n' "$fails" "$ran" >&2
  exit 1
fi
printf 'OK: test-migration-versions — %s case(s), including every branch of the baseline-reset declaration\n' "$ran"
