#!/usr/bin/env bash
# The role gate's own test — it gates the VERDICT, which its sibling's census
# cannot reach.
#
# A diff-scoped gate is otherwise only ever exercised against whatever the
# current branch happens to hold, so "it passed" and "it looked at nothing"
# print the same line. Every case below therefore runs the real gate against a
# fixture checkout (MARGINCE_DS_ROOT) whose diff is known, and reads what it
# said about a named file.
#
# Three properties, and the third is the one that decays quietly:
#
#   1. each arm fires on the shape it exists for;
#   2. the exempt shapes stay silent — a zero reset, a role token spelled
#      correctly, a screen's own element nested inside a primitive, a waiver.
#      A gate that fires on correct code teaches people to stop reading it,
#      which costs more than the misses it prevents;
#   3. the primitive corpus is DERIVED from the design system rather than
#      listed in the gate. A primitive invented inside the fixture — a name
#      that appears nowhere in this repository — must be protected the moment
#      it carries spacing, or the gate has quietly become a second copy of a
#      design system it will stop tracking.
#
# Usage: bash frontend/scripts/check-ds-spacing-roles.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-ds-spacing-roles.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0

fail() {
  echo "FAIL: $*" >&2
  FAILURES=$((FAILURES + 1))
}

# A fixture checkout with a design system, a screen, and origin/main pointing at
# the committed state — the baseline every verdict below is measured against.
# The design system is the SMALLEST one that is still a design system: a token
# layer carrying the four roles, and one primitive that carries spacing.
build_fixture() {
  local repo="$1"
  mkdir -p "$repo/frontend/src/design-system" "$repo/frontend/src/screens"
  cat >"$repo/frontend/src/design-system/tokens.css" <<'CSS'
:root {
  --space-2: 8px;
  --space-3: 12px;
  --space-4: 16px;
  --space-5: 20px;
  --space-6: 24px;
  --gapCards: var(--space-4);
  --padCard: var(--space-4);
  --padPanel: var(--space-5);
  --gapActions: var(--space-2);
}
CSS
  cat >"$repo/frontend/src/design-system/atoms.css" <<'CSS'
.panel-head {
  padding: 0 var(--padPanel);
}
.novel-primitive {
  gap: var(--gapActions);
}
CSS
  printf '.screen-root {\n  display: grid;\n}\n' >"$repo/frontend/src/screens/x.css"
  git -C "$repo" init --quiet
  git -C "$repo" add -A
  git -C "$repo" -c user.email=roles@test -c user.name=roles \
    commit --quiet -m "baseline" --no-gpg-sign
  git -C "$repo" update-ref refs/remotes/origin/main HEAD
}

# Runs the gate over a fixture whose screen stylesheet has GAINED $2, and
# leaves the output in $TMP/out. Each case gets its own checkout so one case's
# additions cannot be read as another's.
run_case() {
  local name="$1" added="$2" repo="$TMP/$name"
  build_fixture "$repo"
  printf '%s\n' "$added" >>"$repo/frontend/src/screens/x.css"
  MARGINCE_DS_ROOT="$repo" "$GATE" >"$TMP/out" 2>&1 && return 0
  return 1
}

# A finding names the screen and says which rule it broke.
expect_finding() {
  local name="$1" added="$2" wanted="$3"
  if run_case "$name" "$added"; then
    fail "$name: the gate passed on:"
    printf '%s\n' "$added" | sed 's/^/      /' >&2
    return
  fi
  if ! grep -q "frontend/src/screens/x.css" "$TMP/out"; then
    fail "$name: the gate failed without naming the screen it read:"
    sed 's/^/      /' "$TMP/out" >&2
    return
  fi
  grep -q -- "$wanted" "$TMP/out" \
    || fail "$name: the finding does not say \"$wanted\":$(printf '\n')$(sed 's/^/      /' "$TMP/out")"
}

expect_clean() {
  local name="$1" added="$2"
  if ! run_case "$name" "$added"; then
    fail "$name: the gate reported correct code:"
    sed 's/^/      /' "$TMP/out" >&2
  fi
}

# ---------------------------------------------------------------------------
# 1. Each arm fires on the shape it exists for.
# ---------------------------------------------------------------------------
expect_finding actions-rung \
  '.x-actions {
  gap: var(--space-5);
}' \
  'var(--gapActions)'

expect_finding card-inset-rung \
  '.x-card {
  padding: var(--space-3);
}' \
  'var(--padCard)'

expect_finding card-stack-rung \
  '.x-cards {
  gap: var(--space-6);
}' \
  'var(--gapCards)'

expect_finding primitive-respaced \
  '.x-memory .panel-head {
  padding-top: var(--space-4);
}' \
  'design-system primitive'

# A declaration ending at the closing brace rather than at a semicolon is the
# same declaration. It is spelled this way in enough of the tree that a parser
# which lost it would report a clean sweep over real violations.
expect_finding no-trailing-semicolon \
  '.x-brief-actions {
  gap: var(--space-3)
}' \
  'var(--gapActions)'

# ---------------------------------------------------------------------------
# 2. The exempt shapes stay silent.
# ---------------------------------------------------------------------------
expect_clean role-spelled \
  '.x-actions {
  gap: var(--gapActions);
}'

expect_clean zero-reset \
  '.x-panel {
  padding: 0;
}
.x-row .panel-head {
  padding-top: 0;
}'

# The subject is the LAST class in the selector: this rule spaces the screen's
# own note, which happens to live inside a panel body. The screen owns it.
expect_clean own-element-inside-a-primitive \
  '.panel-body .x-note {
  margin-top: var(--space-3);
}'

expect_clean waived \
  '.x-actions {
  gap: var(--space-3); /* ds:ignore a filter strip, not a button row */
}'

# A rung in a context that has NO role is still the right answer — the gate
# holds the three contexts it can name and nothing else.
expect_clean unnamed-context \
  '.x-timeline-entry {
  gap: var(--space-3);
  padding: var(--space-6);
}'

# ---------------------------------------------------------------------------
# 3. The corpus is derived from the design system, not listed in the gate.
#    `.novel-primitive` exists only inside the fixture: no list written today
#    could contain it, so a gate that fires here is reading the design system
#    it protects.
# ---------------------------------------------------------------------------
expect_finding derived-corpus \
  '.x-shelf > .novel-primitive {
  margin-top: var(--space-4);
}' \
  '.novel-primitive is a design-system primitive'

# ---------------------------------------------------------------------------
# 4. The gate fails LOUDLY where it would otherwise pass everything. Both of
#    these are the census that fails short: an empty corpus and a role nobody
#    defines each disarm an arm while every line of output still says PASS.
# ---------------------------------------------------------------------------
REPO="$TMP/renamed-role"
build_fixture "$REPO"
sed -i.bak 's/--padCard:/--padCardV2:/' "$REPO/frontend/src/design-system/tokens.css"
rm -f "$REPO/frontend/src/design-system/tokens.css.bak"
if MARGINCE_DS_ROOT="$REPO" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed while demanding --padCard, which tokens.css no longer defines"
elif ! grep -q -- "--padCard" "$TMP/out"; then
  fail "the gate failed on a renamed role token without naming it:"
  sed 's/^/      /' "$TMP/out" >&2
fi

REPO="$TMP/empty-corpus"
build_fixture "$REPO"
printf '.panel-head {\n  color: red;\n}\n' >"$REPO/frontend/src/design-system/atoms.css"
if MARGINCE_DS_ROOT="$REPO" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed with an empty primitive corpus — every screen would clear the primitive arm"
elif ! grep -q "corpus is empty" "$TMP/out"; then
  fail "the gate failed on an empty corpus without saying so:"
  sed 's/^/      /' "$TMP/out" >&2
fi

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "check-ds-spacing-roles.sh no longer says what it claims to say. Run it" >&2
  echo "against one of the fixtures under $TMP by hand:" >&2
  echo "  MARGINCE_DS_ROOT=<fixture> frontend/scripts/check-ds-spacing-roles.sh" >&2
  exit 1
fi

echo "==> DS spacing roles: both arms fire, the exempt shapes stay silent, and the primitive corpus is read from the design system"
