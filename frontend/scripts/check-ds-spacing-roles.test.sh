#!/usr/bin/env bash
# The role gate's own test — it gates the VERDICT, which a census cannot reach.
#
# "The gate passed" and "the gate looked at nothing" print the same line, so
# every case below runs the REAL gate over a fixture tree whose contents are
# known (MARGINCE_CORE_DIR, MARGINCE_EXT_DIR) and reads what it said about a
# named file.
#
# Four properties, and the last two are the ones that decay quietly:
#
#   1. each arm fires on the shape it exists for;
#   2. the exempt shapes stay silent — a zero reset, a role token spelled
#      correctly, an element inside a named surface, a screen's own class
#      nested in a primitive, a waiver. A gate that fires on correct code
#      teaches people to stop reading it, which costs more than the misses it
#      prevents;
#   3. the primitive corpus is DERIVED from the design system rather than
#      listed in the gate. A primitive invented inside the fixture — a name
#      that appears nowhere in this repository — must be protected the moment
#      it carries spacing, or the gate has quietly become a second copy of a
#      design system it will stop tracking;
#   4. the unit tier is read to the bottom. A gate that reads a smaller tree
#      than it claims reports PASS and there is no failing assertion to notice.
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

# The smallest tree that is still a design system: a token layer carrying the
# four roles, one primitive that carries spacing, and one class this tier spaces
# WITHOUT owning it.
build_fixture() {
  local core="$1"
  mkdir -p "$core/design-system" "$core/screens"
  cat >"$core/design-system/tokens.css" <<'CSS'
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
  cat >"$core/design-system/atoms.css" <<'CSS'
.panel-head {
  padding: 0 var(--padPanel);
}
.novel-primitive {
  gap: var(--gapActions);
}
.host-surface .x-guest {
  margin-top: var(--gapCards);
}
CSS
  printf '.screen-root {\n  display: grid;\n}\n' >"$core/screens/x.css"
}

# Runs the gate over a fixture core tree whose screen stylesheet carries $2,
# leaving the output in $TMP/out. Each case gets its own tree, so one case's
# rules cannot be read as another's.
run_case() {
  local name="$1" rules="$2" core="$TMP/$name"
  build_fixture "$core"
  printf '%s\n' "$rules" >>"$core/screens/x.css"
  MARGINCE_CORE_DIR="$core" MARGINCE_EXT_DIR="$TMP/no-units" \
    "$GATE" >"$TMP/out" 2>&1 && return 0
  return 1
}

expect_finding() {
  local name="$1" rules="$2" wanted="$3"
  if run_case "$name" "$rules"; then
    fail "$name: the gate passed on:"
    printf '%s\n' "$rules" | sed 's/^/      /' >&2
    return
  fi
  if ! grep -q "screens/x.css" "$TMP/out"; then
    fail "$name: the gate failed without naming the screen it read:"
    sed 's/^/      /' "$TMP/out" >&2
    return
  fi
  grep -q -- "$wanted" "$TMP/out" \
    || fail "$name: the finding does not say \"$wanted\":$(printf '\n')$(sed 's/^/      /' "$TMP/out")"
}

expect_clean() {
  local name="$1" rules="$2"
  if ! run_case "$name" "$rules"; then
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
  padding-block: var(--space-2);
}' \
  'design-system primitive'

# A declaration ending at the closing brace rather than at a semicolon is the
# same declaration. It is spelled that way in enough of the tree that a parser
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

# A variant spelled in the house's vocabulary is a decision that moves with the
# token rather than a second opinion — which is the whole difference between it
# and the rung the same screen used to write.
expect_clean primitive-varied-by-role \
  '.x-aside .panel-head {
  padding-top: var(--padPanel);
}'

expect_clean zero-reset \
  '.x-panel {
  padding: 0;
}
.x-row .panel-head {
  padding-top: 0;
}'

# The subject is the last class of the last COMPOUND: these rules space the
# screen's own note and a text box, not the surfaces they sit in.
expect_clean own-element-inside-a-primitive \
  '.panel-body .x-note {
  margin-top: var(--space-3);
}'

expect_clean element-inside-a-named-surface \
  '.x-card input {
  padding: var(--space-3);
}'

# `:is()` carries commas of its own, and a selector list is split on the ones
# BETWEEN selectors. Split naively and `.x-note:is(.compact, .x-card)` — a note,
# narrowed — reads as a second selector ending in `.x-card`, and the gate demands
# a card's inset for something that is not a card.
expect_clean commas-inside-a-pseudo \
  '.x-note:is(.compact, .x-card) {
  padding: var(--space-3);
}'

expect_clean waived \
  '.x-actions {
  gap: var(--space-3); /* ds:ignore a filter strip, not a button row */
}'

# The same waiver on a line of its own, waiving what follows it — which is where
# a CSS comment normally goes, and the only place a long reason fits without the
# formatter breaking the value across three lines to make room for it.
expect_clean waived-on-its-own-line \
  '.x-cards {
  /* ds:ignore a masonry column, not the page stack */
  gap: var(--space-6);
}'

# ...and it waives ONE declaration, not the rest of the rule.
expect_finding waiver-does-not-carry \
  '.x-cards {
  /* ds:ignore a masonry column, not the page stack */
  gap: var(--space-6);
  row-gap: var(--space-3);
}' \
  'var(--gapCards)'

# A rung in a context that has NO role is still the right answer — the gate
# holds the three contexts it can name and nothing else.
expect_clean unnamed-context \
  '.x-timeline-entry {
  gap: var(--space-3);
  padding: var(--space-6);
}'

# ---------------------------------------------------------------------------
# 3. The corpus is derived from the design system, not listed in the gate.
# ---------------------------------------------------------------------------
# The tier spaces classes it does NOT own: here it places a screen's `.x-guest`
# inside one of its own surfaces. Ownership is declaring a class with nothing
# above it in the selector — read the spaced set alone and every screen holding
# its own base rule for that class becomes a finding.
expect_clean spaced-but-not-owned \
  '.x-guest {
  margin-top: var(--space-3);
}'

# `.novel-primitive` exists only inside the fixture: no list written today could
# contain it, so a gate that fires here is reading the design system it protects.
expect_finding derived-corpus \
  '.x-shelf > .novel-primitive {
  margin-top: var(--space-4);
}' \
  '.novel-primitive is a design-system primitive'

# ---------------------------------------------------------------------------
# 4. The unit tier, at both depths. A unit ships screen.css directly in its
#    frontend/ and may nest whatever it likes below, so "the tier is read" and
#    "the tier is read to the bottom" are different claims — and the second is
#    the one that broke before, in this gate's sibling.
# ---------------------------------------------------------------------------
UNIT_CORE="$TMP/unit-core"
build_fixture "$UNIT_CORE"
UNITS="$TMP/units"
DEPTHS=(probe/frontend/screen.css probe/frontend/views/panel.css)
for rel in "${DEPTHS[@]}"; do
  mkdir -p "$UNITS/$(dirname "$rel")"
  printf '.unit-actions {\n  gap: var(--space-5);\n}\n' >"$UNITS/$rel"
done

if MARGINCE_CORE_DIR="$UNIT_CORE" MARGINCE_EXT_DIR="$UNITS" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed with a violation planted at every depth of the unit tier"
else
  for rel in "${DEPTHS[@]}"; do
    grep -qF "$UNITS/$rel" "$TMP/out" \
      || fail "the gate did not name the planted extensions/$rel — it is not reading that file:$(printf '\n')$(sed 's/^/      /' "$TMP/out")"
  done
fi

# ---------------------------------------------------------------------------
# 5. The gate fails LOUDLY where it would otherwise pass everything. Each of
#    these disarms an arm while every line of output still says PASS.
# ---------------------------------------------------------------------------
CORE="$TMP/renamed-role"
build_fixture "$CORE"
sed -i.bak 's/--padCard:/--padCardV2:/' "$CORE/design-system/tokens.css"
rm -f "$CORE/design-system/tokens.css.bak"
if MARGINCE_CORE_DIR="$CORE" MARGINCE_EXT_DIR="$TMP/no-units" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed while demanding --padCard, which tokens.css no longer defines"
elif ! grep -q -- "--padCard" "$TMP/out"; then
  fail "the gate failed on a renamed role token without naming it:"
  sed 's/^/      /' "$TMP/out" >&2
fi

CORE="$TMP/empty-corpus"
build_fixture "$CORE"
printf '.panel-head {\n  color: red;\n}\n' >"$CORE/design-system/atoms.css"
if MARGINCE_CORE_DIR="$CORE" MARGINCE_EXT_DIR="$TMP/no-units" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed with an empty primitive corpus — every screen would clear the primitive arm"
elif ! grep -q "corpus is empty" "$TMP/out"; then
  fail "the gate failed on an empty corpus without saying so:"
  sed 's/^/      /' "$TMP/out" >&2
fi

CORE="$TMP/no-sheets"
mkdir -p "$CORE/design-system"
cp "$TMP/actions-rung/design-system/tokens.css" "$CORE/design-system/tokens.css"
cp "$TMP/actions-rung/design-system/atoms.css" "$CORE/design-system/atoms.css"
if MARGINCE_CORE_DIR="$CORE" MARGINCE_EXT_DIR="$TMP/no-units" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed having scanned no stylesheet at all — the census failing short, reported as a clean tree"
elif ! grep -q "scanned no stylesheets" "$TMP/out"; then
  fail "the gate failed on an empty sweep without saying so:"
  sed 's/^/      /' "$TMP/out" >&2
fi

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "check-ds-spacing-roles.sh no longer says what it claims to say. Run it" >&2
  echo "against one of the fixtures under $TMP by hand:" >&2
  echo "  MARGINCE_CORE_DIR=<fixture> frontend/scripts/check-ds-spacing-roles.sh" >&2
  exit 1
fi

echo "==> DS spacing roles: both arms fire, the exempt shapes stay silent, the unit tier is read at both depths, and the primitive corpus is read from the design system"
