#!/usr/bin/env bash
# Spacing ROLE gate: new screen CSS may not re-space a design-system primitive,
# and in a context that has a named role it spells that role rather than a rung.
#
# check-ds-spacing.sh beside this one holds the VOCABULARY — spacing is written
# as `var(--space-N)`, never as a raw px. This one holds the GRAMMAR, which is
# the half that was missing: `gap: var(--space-2)` and `gap: var(--space-5)`
# between two buttons are both tokenized and only one of them is right, so the
# tree carried 8, 12 and 20 for one relationship and every one of them passed.
#
# Two arms, and each answers a question a reader of one diff cannot:
#
#   primitive — the rule's subject is a class the design system already spaces
#               (`.panel-head`, `.card-actions`, `.btn`). The primitive owns its
#               own interval; a screen that re-spaces one has made a second
#               opinion about a shape shared with every other screen, and the
#               two then drift. The corpus is DERIVED from
#               frontend/src/design-system/*.css on every run, so a primitive
#               added tomorrow is protected the day it exists.
#
#   role      — the subject names a context the design language has an answer
#               for, and the declaration does not use it:
#                 *-actions   gap / column-gap  → var(--gapActions)
#                 *-cards     gap / row-gap     → var(--gapCards)
#                 *-card, *-panel  padding      → var(--padCard) / var(--padPanel)
#               The three mappings are spelled below rather than derived,
#               because the mapping from a selector's shape to a role IS the
#               rule — there is nothing else in the tree to read it off. What IS
#               derived is that each role token exists: renaming one in
#               tokens.css without renaming it here fails this gate rather than
#               quietly disarming the arm that demanded it.
#
# The SUBJECT of a rule is the last class in its selector — what it styles, not
# what it is nested under. `.pe-memory .panel-head` re-spaces the panel's head;
# `.panel-body .pe-note` spaces the screen's own note inside one, which is the
# screen's business. A value that measures nothing (`padding: 0`, `margin: 0
# auto`) is not a rhythm decision and is never reported.
#
# WHAT IT DELIBERATELY DOES NOT CATCH: a card stack under a name that does not
# say so. `.co-tiles` holding four `<Card>`s is exactly the subject of the
# `--gapCards` rule and this gate cannot see it, because a stylesheet does not
# know what a container holds. That question is answerable only against the TSX,
# and a gate that guessed at it from class names would fire on correct code —
# which teaches people to stop reading its output, and costs more than the
# misses it prevents.
#
# frontend/src/design-system/ is EXEMPT: it DEFINES both the primitives and the
# roles, so every rule there is the owner writing its own interval.
#
# DIFF-SCOPED, like its sibling: only the lines THIS branch adds versus the
# merge-base with origin/main. The pre-existing backlog is not gated — write it
# right the first time. A genuine one-off is waived in-line, with a reason:
#   /* ds:ignore <reason> */
#
# Usage: frontend/scripts/check-ds-spacing-roles.sh   (wired into `make fe-ds-gates`)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=frontend/scripts/lib/diff-scope.sh
. "$SCRIPT_DIR/lib/diff-scope.sh"

SCANNER="$SCRIPT_DIR/lib/spacing-roles.awk"

# The checkout to gate. Overridable so this gate's own test can point it at a
# fixture repository and read the verdict — the same reason check-space-tokens.sh
# takes MARGINCE_EXT_DIR. A diff-scoped gate is otherwise untestable except
# against whatever the current branch happens to hold.
REPO_ROOT="${MARGINCE_DS_ROOT:-$(diff_scope_root "$SCRIPT_DIR")}"
if [[ -z "$REPO_ROOT" ]]; then
  echo "==> DS spacing roles: not a git checkout — skipped"
  exit 0
fi

BASE="$(diff_scope_base "$REPO_ROOT")"
if [[ -z "$BASE" ]]; then
  echo "==> DS spacing roles: no origin/main baseline — skipped"
  exit 0
fi

# The roles, and the contexts they answer for. Read by the scanner; asserted
# against tokens.css first, because a role token that no longer exists would
# make every comparison below fail-open on a value nobody can spell.
ROLE_ACTIONS="--gapActions"
ROLE_CARDS="--gapCards"
PAD_CARD="--padCard"
PAD_PANEL="--padPanel"

DESIGN_SYSTEM="$REPO_ROOT/frontend/src/design-system"
TOKENS="$DESIGN_SYSTEM/tokens.css"
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

# The corpus: every class the design system spaces. Derived on each run from
# the tier that owns them — a list here would be a second copy of the design
# system, correct on the day it was written and wrong by the next primitive.
OWNED="$(mktemp)"
CLEANUP=("$OWNED")
trap 'rm -f "${CLEANUP[@]}"' EXIT

find "$DESIGN_SYSTEM" -type f -name '*.css' -print0 \
  | xargs -0 awk -f "$SCANNER" -v mode=owned 2>/dev/null \
  | sort -u >"$OWNED" || true

OWNED_COUNT="$(grep -c . "$OWNED" || true)"
if [[ "$OWNED_COUNT" -eq 0 ]]; then
  echo "FAIL: no design-system class carries spacing — the corpus is empty, so" >&2
  echo "      the primitive arm would pass everything. $DESIGN_SYSTEM is either" >&2
  echo "      not the design system or the scanner no longer reads it." >&2
  exit 1
fi

# The gated trees come from lib/diff-scope.sh, shared with the vocabulary gate:
# a unit's screen is shipped UI in the same bundle and re-spaces the same
# primitives, so it is read on the same terms as the core.

# Read-loop rather than mapfile — the CI/dev host ships bash 3.2.
CHANGED=()
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  [[ "$f" == frontend/src/design-system/* ]] && continue
  CHANGED+=("$f")
done < <(diff_scope_changed "$REPO_ROOT" "$BASE" "${DIFF_SCOPE_CSS[@]}")

if [[ "${#CHANGED[@]}" -eq 0 ]]; then
  echo "==> DS spacing roles: no changed frontend *.css outside the design system — nothing to gate"
  exit 0
fi

echo "==> DS spacing roles (${#CHANGED[@]} changed *.css vs ${BASE:0:12}, ${OWNED_COUNT} owned classes)"

FINDINGS="$(mktemp)"
CLEANUP+=("$FINDINGS")

for f in "${CHANGED[@]}"; do
  diff_out="$(diff_scope_added_diff "$REPO_ROOT" "$BASE" "$f")"
  # A mode-only change carries no hunks. Skipped rather than fed to the scanner,
  # whose three inputs are positional: an empty one would slide the stylesheet
  # into the diff's place.
  [[ -n "$diff_out" ]] || continue

  printf '%s\n' "$diff_out" \
    | awk -f "$SCANNER" \
        -v mode=check \
        -v target="$f" \
        -v roleActions="$ROLE_ACTIONS" \
        -v roleCards="$ROLE_CARDS" \
        -v padCard="$PAD_CARD" \
        -v padPanel="$PAD_PANEL" \
        "$OWNED" - "$REPO_ROOT/$f" >>"$FINDINGS"
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

report_arm primitive "new code re-spaces a design-system primitive"
report_arm role "new code spells a rung where the design language has a role"

if [[ "$EXIT" == "0" ]]; then
  echo "PASS — no new spacing outside its role"
else
  echo ""
  echo "A primitive carries its own interval: space your own element inside it,"
  echo "or change the primitive so every screen moves together. In a context that"
  echo "has a role, use the role rather than the rung it happens to equal today:"
  printf '  %-34s between two buttons side by side\n' "var($ROLE_ACTIONS)"
  printf '  %-34s between sibling card surfaces\n' "var($ROLE_CARDS)"
  printf '  %-34s inside a card / inside a panel\n' "var($PAD_CARD) / var($PAD_PANEL)"
  echo "A genuine one-off is waived in-line, with a reason, on the declaration:"
  echo "  /* ds:ignore <reason> */"
fi

exit $EXIT
