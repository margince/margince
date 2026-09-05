#!/usr/bin/env bash
# Spacing ROLE gate: a screen may not re-shape a design-system primitive — its
# interval or its type — and in a context the design language has a name for it
# spells the role rather than the rung.
#
# check-ds-spacing.sh beside this one holds the VOCABULARY — spacing is written
# as `var(--space-N)`, never as a raw px. This one holds the GRAMMAR, which is
# the half that was missing: `gap: var(--space-2)` and `gap: var(--space-5)`
# between two buttons are both tokenized and only one of them is right, so the
# tree carried 8, 12 and 20 for one relationship and every one of them passed.
#
# Two arms, and each answers a question a reader of one diff cannot:
#
#   primitive — the rule's subject is a class the design system both shapes and
#               declares on its own (`.panel-head`, `.card-actions`, `.btn`,
#               `.t-label`), and the rule re-shapes it in the kind the tier owns:
#               an interval (padding, margin, gap) on a class the tier spaces, or
#               a type (font-size, line-height, letter-spacing) on a class the
#               tier sizes. A screen that does either has made a second opinion
#               about a shape shared with every other screen, and the two then
#               drift. A spacing variant spelled in the house's own vocabulary is
#               not a second opinion and passes: `padding: var(--padCard)` on a
#               rail's panel body says which surface it means, and moves when
#               that surface is retuned. A type variant has no such vocabulary —
#               every size is a rung — so any re-size is a finding, and a genuine
#               one is waived with its reason.
#
#   role      — the subject names a context the design language has an answer
#               for, and the declaration does not use it:
#                 *-actions   gap / column-gap  -> var(--gapActions)
#                 *-cards     gap / row-gap     -> var(--gapCards)
#                 *-card, *-panel  padding      -> var(--padCard) / var(--padPanel)
#               The three mappings are spelled below rather than derived,
#               because the mapping from a selector's shape to a role IS the
#               rule — there is nothing else in the tree to read it off. What IS
#               derived is that each role token exists: renaming one in
#               tokens.css without renaming it here fails this gate rather than
#               quietly disarming the arm that demanded it.
#
# The SUBJECT of a rule is the last class of its last compound — what it styles,
# not what it is nested under and not an element inside it. `.pe-memory
# .panel-head` re-spaces the panel's head; `.panel-body .pe-note` spaces the
# screen's own note inside one, and `.pe-card input` spaces a text box. A value
# that measures nothing (`padding: 0`, `margin: 0 auto`) is not a rhythm
# decision and is never reported.
#
# WHAT IT DELIBERATELY DOES NOT CATCH: a card stack under a name that does not
# say so. `.co-tiles` holding four `<Card>`s is exactly the subject of the
# `--gapCards` rule and this gate cannot see it, because a stylesheet does not
# know what a container holds. That question is answerable only against the TSX,
# and a gate that guessed at it from class names would fire on correct code —
# which teaches people to stop reading its output, and costs more than the
# misses it prevents.
#
# WHOLE-TREE, unlike its diff-scoped sibling: the tree was cleared to zero
# before this bar was armed, so the rule is simply that screen CSS is clean. It
# also has to be whole-tree to hold what a diff cannot see — a class PROMOTED
# into the design system turns every untouched screen rule about it into a
# second opinion, and not one line of that diff is in a screen at all.
#
# frontend/src/design-system/ is EXEMPT: it DEFINES both the primitives and the
# roles, so every rule there is the owner writing its own interval. So is
# mcp-apps/ for the PRIMITIVE arm — see primitive_arm_for below.
#
# A genuine variant with no role to name it is waived in-line, with a reason:
#   /* ds:ignore <reason> */
#
# Usage: frontend/scripts/check-ds-spacing-roles.sh   (wired into `make fe-ds-gates`)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCANNER="$SCRIPT_DIR/lib/spacing-roles.awk"

# The trees to read. Both are overridable so this gate's own test can point it
# at a fixture — the same reason check-space-tokens.sh takes MARGINCE_EXT_DIR,
# and the only way this gate's VERDICT gets tested rather than only its census.
CORE_DIR="${MARGINCE_CORE_DIR:-$(cd "$SCRIPT_DIR/../src" && pwd)}"
EXT_DIR="${MARGINCE_EXT_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)/extensions}"

DESIGN_SYSTEM="$CORE_DIR/design-system"
TOKENS="$DESIGN_SYSTEM/tokens.css"

# The roles, and the contexts they answer for. Asserted against tokens.css
# first, because a role token that no longer exists would make every comparison
# below fail-open on a value nobody can spell.
ROLE_ACTIONS="--gapActions"
ROLE_CARDS="--gapCards"
PAD_CARD="--padCard"
PAD_PANEL="--padPanel"

if [[ ! -f "$TOKENS" ]]; then
  echo "FAIL: $TOKENS not found — this gate is reading the wrong tree" >&2
  exit 1
fi
for role in "$ROLE_ACTIONS" "$ROLE_CARDS" "$PAD_CARD" "$PAD_PANEL"; do
  if ! grep -qE "^[[:space:]]*$role[[:space:]]*:" "$TOKENS"; then
    echo "FAIL: this gate demands $role and tokens.css does not define it." >&2
    echo "      Rename it in both places, or the arm that demands it holds" >&2
    echo "      screens to a token that resolves to nothing." >&2
    exit 1
  fi
done

# The corpus: every class the design system both SHAPES and declares on its own,
# kept by kind — the classes it spaces and the classes it sizes — because a class
# it only sizes has no interval a screen could contradict. Derived on each run
# from the tier that owns them — a list here would be a second copy of the
# design system, correct on the day it was written and wrong by the next
# primitive.
#
# The intersection, rather than the shaped set alone, because this tier also
# spaces classes it does not own: `.mw-conversation .ob-conv-thread` places a
# SCREEN's thread inside the workbench, and reading that as ownership would turn
# the screen's own base rule into a finding. A class declared with nothing above
# it in the selector is what owning it looks like.
CLAIMS="$(mktemp)"
OWNED="$(mktemp)"
FINDINGS="$(mktemp)"
trap 'rm -f "$CLAIMS" "$OWNED" "$FINDINGS"' EXIT

find "$DESIGN_SYSTEM" -type f -name '*.css' -print0 \
  | xargs -0 awk -f "$SCANNER" -v mode=owned 2>/dev/null \
  | sort -u >"$CLAIMS" || true

for kind in spaced sized; do
  comm -12 \
    <(awk -v kind="$kind" '$1 == kind { print $2 }' "$CLAIMS" | sort -u) \
    <(awk '$1 == "own" { print $2 }' "$CLAIMS" | sort -u) \
    | sed "s/^/$kind /" >>"$OWNED"
done

SPACED_COUNT="$(grep -c '^spaced ' "$OWNED" || true)"
SIZED_COUNT="$(grep -c '^sized ' "$OWNED" || true)"
if [[ "$SPACED_COUNT" -eq 0 || "$SIZED_COUNT" -eq 0 ]]; then
  echo "FAIL: no design-system class is both shaped and declared on its own —" >&2
  echo "      the corpus is empty, so the primitive arm would pass everything." >&2
  echo "      $DESIGN_SYSTEM is either not the design system, or the scanner no" >&2
  echo "      longer reads it." >&2
  exit 1
fi

# Both trees, because a unit's screen is shipped UI in the same bundle and
# re-spaces the same primitives. A unit ships no frontend at all in a
# backend-only downstream, so an empty extension result is a true statement
# about an empty tree; what may not be empty is the pair, checked below.
SHEETS=()
while IFS= read -r f; do
  [[ -n "$f" ]] && SHEETS+=("$f")
done < <(
  find "$CORE_DIR" -type f -name '*.css' -not -path "$DESIGN_SYSTEM/*" 2>/dev/null || true
  find "$EXT_DIR" -type f -name '*.css' -path '*/frontend/*' -not -path '*/node_modules/*' 2>/dev/null || true
)

if [[ "${#SHEETS[@]}" -eq 0 ]]; then
  echo "FAIL: scanned no stylesheets — the gate is miswired" >&2
  exit 1
fi

echo "==> DS spacing roles (${#SHEETS[@]} stylesheets, ${SPACED_COUNT} spaced + ${SIZED_COUNT} sized primitives)"

# A stylesheet that does not load the design system's CLASS layer is its own
# document, and a class in it collides with nothing. `mcp-apps/` is that case:
# each view is a standalone document served to a third-party host, and view.css
# imports tokens.css and nothing else — so `.empty` there is the view's own
# element rather than the atom of the same name. The ROLE arm still applies,
# because the roles come from the token layer it does load.
primitive_arm_for() {
  case "$1" in
    "$CORE_DIR"/mcp-apps/*) echo 0 ;;
    *) echo 1 ;;
  esac
}

for f in "${SHEETS[@]}"; do
  awk -f "$SCANNER" \
    -v mode=check \
    -v target="$f" \
    -v primitives="$(primitive_arm_for "$f")" \
    -v roleActions="$ROLE_ACTIONS" \
    -v roleCards="$ROLE_CARDS" \
    -v padCard="$PAD_CARD" \
    -v padPanel="$PAD_PANEL" \
    "$OWNED" "$f" >>"$FINDINGS"
done

EXIT=0
report_arm() {
  local tag="$1" heading="$2" hits
  hits="$(awk -F'\t' -v tag="$tag" '$1 == tag { print "  " $2 }' "$FINDINGS")"
  [[ -n "$hits" ]] || return 0
  echo ""
  echo "FAIL: $heading"
  echo "$hits"
  EXIT=1
}

report_arm primitive "a screen re-shapes a design-system primitive"
report_arm role "a rung is spelled where the design language has a role"

if [[ "$EXIT" == "0" ]]; then
  echo "PASS — every gated interval is spelled as the role it plays"
else
  echo ""
  echo "A primitive carries its own interval and its own type: space or size your"
  echo "own element inside it, or change the primitive so every screen moves"
  echo "together. In a context that"
  echo "has a role, use the role rather than the rung it happens to equal today:"
  printf '  %-34s between two buttons side by side\n' "var($ROLE_ACTIONS)"
  printf '  %-34s between sibling card surfaces\n' "var($ROLE_CARDS)"
  printf '  %-34s inside a card / inside a panel\n' "var($PAD_CARD) / var($PAD_PANEL)"
  echo "A genuine variant with no role to name it is waived in-line, with a"
  echo "reason, on the declaration:"
  echo "  /* ds:ignore <reason> */"
fi

exit $EXIT
