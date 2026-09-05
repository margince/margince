#!/usr/bin/env bash
# The design-system script gates' own test — it gates their SCOPE, not their
# verdict.
#
# A gate that reads a smaller tree than it claims reports PASS and there is no
# failing assertion to notice. That is how the unit tier went unread: of the
# five design-system gates, three swept extensions/*/frontend and two swept
# frontend/src alone, all five printed one PASS into one lane, and a unit screen
# could declare Comic Sans or spell a custom property nothing defines with
# nothing to say so.
#
# So the property under test is the SCOPE: point the gate's extension tier at a
# fixture carrying exactly the violation that gate exists to catch, and it has
# to name that file. Pointing it at a clean fixture, it must not.
#
# The subject list is READ FROM the `fe-ds-gates` target, which is what defines
# a design-system script gate — a sixth gate added there and forgotten here
# fails this test rather than joining the tier unread. That is the half that
# must not fail short: a gate can only leave the census by being named below,
# with its reason.
#
# The verdicts are deliberately assertions about the FIXTURE path, never about
# the gate's exit code alone: frontend/src is scanned on the same run, so
# "the gate failed" would also be true when someone else's in-flight edit is
# what failed it, and the scope claim would be proved by an unrelated defect.
#
# Usage: bash frontend/scripts/check-ext-frontend-walk.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0

fail() {
  echo "FAIL: $*" >&2
  FAILURES=$((FAILURES + 1))
}

# One violation per gate, in the shape that gate reads. U+1F680 is spelled as
# its UTF-8 bytes rather than as \U0001F680: the dev and CI host ships bash 3.2,
# whose printf knows \x and not \U, and the \U form there yields the literal
# eight characters — which no emoji range matches, so the gate would have been
# right to pass and this test would have been measuring its own typo.
GLYPH="$(printf '\xf0\x9f\x9a\x80')"
CASES=(
  "check-ds-purity.sh|const brand = \"#ff0000\";"
  "check-icon-glyph.sh|const label = \"launch $GLYPH\";"
  "check-font-lock.sh|const sheet = \"font-family: Comic Sans MS;\";"
  "check-space-tokens.sh|const gap = \"var(--space-not-a-token)\";"
  "check-contract-fetch.sh|const rows = await fetch(\"/v1/deals\");"
)

# Named, with the reason, because a gate silently absent from the census is the
# defect this file exists to catch.
#   check-ds-spacing.sh   — diff-scoped against origin/main rather than
#                           tree-scoped, so a fixture outside the checkout is
#                           invisible to it by design. Its own census lives in
#                           check-ds-spacing.test.sh.
#   check-ds-spacing.test.sh — that census; a test, not a gate.
#   check-ds-spacing-roles.sh — whole-tree, but CSS-only, and the fixtures this
#                           file plants are *.tsx: a stylesheet gate has nothing
#                           to say about them. Its own suite plants unit
#                           STYLESHEETS at both depths and requires the gate to
#                           name each, which is this file's property proved in
#                           the shape that gate reads.
#   check-ds-spacing-roles.test.sh — that suite; a test, not a gate.
#   check-contract-fetch.test.sh — the contract-fetch gate's own census, which
#                           holds the two properties this file cannot see: that
#                           the refused mount is DERIVED from crm.yaml, and that
#                           a waiver without a reason is still a finding. A test,
#                           not a gate; the gate itself is measured above.
#   check-ext-frontend-walk.test.sh — this file.
EXCUSED=(
  check-ds-spacing.sh
  check-ds-spacing.test.sh
  check-ds-spacing-roles.sh
  check-ds-spacing-roles.test.sh
  check-contract-fetch.test.sh
  check-ext-frontend-walk.test.sh
)

# ---------------------------------------------------------------------------
# 1. Every script the fe-ds-gates lane runs is either measured here or excused
#    by name — and every case here is still in that lane.
# ---------------------------------------------------------------------------
LANE="$(
  awk '/^fe-ds-gates:/ { inrecipe = 1; next }
       inrecipe && /^\t/ { print; next }
       inrecipe { exit }' "$ROOT/Makefile" \
    | grep -oE 'frontend/scripts/[A-Za-z0-9._-]+\.sh' \
    | sed 's|.*/||' | sort -u
)"
if [[ -z "$LANE" ]]; then
  echo "FAIL: no check scripts read out of the fe-ds-gates target in $ROOT/Makefile" >&2
  echo "      — the lane has moved or been renamed, so this test is measuring" >&2
  echo "      nothing. Re-point it at wherever the design-system gates now run." >&2
  exit 1
fi

covered() {
  local name="$1" entry
  for entry in "${CASES[@]}"; do
    [[ "${entry%%|*}" == "$name" ]] && return 0
  done
  for entry in "${EXCUSED[@]}"; do
    [[ "$entry" == "$name" ]] && return 0
  done
  return 1
}

while IFS= read -r name; do
  covered "$name" || fail "fe-ds-gates runs $name and this test neither measures its scope nor excuses it by name"
done <<<"$LANE"

for entry in "${CASES[@]}"; do
  gate="${entry%%|*}"
  grep -qx "$gate" <<<"$LANE" || fail "this test measures $gate, which fe-ds-gates no longer runs — the row is stale"
done

# ---------------------------------------------------------------------------
# 2. Each gate, against a unit-tier fixture: it must see the violation, and it
#    must not invent one.
#
#    BOTH depths, because "the tier is read" and "the tier is read to the
#    bottom" are different claims and the second is the one that broke before:
#    the spacing gate's unit pathspec required an intermediate directory and so
#    missed every screen sitting directly in frontend/ — a gate that inspected
#    the tier and none of its files. A unit ships screen.tsx at the top and may
#    nest whatever it likes below it, so a violation at either depth has to be
#    named.
# ---------------------------------------------------------------------------
DEPTHS=(probe/frontend/screen.tsx probe/frontend/views/panel.tsx)

plant() {
  local dir="$1" line="$2" rel
  for rel in "${DEPTHS[@]}"; do
    mkdir -p "$dir/$(dirname "$rel")"
    printf '%s\n' "$line" >"$dir/$rel"
  done
}

CLEAN="$TMP/clean"
plant "$CLEAN" 'const gap = "var(--space-3)";'

for entry in "${CASES[@]}"; do
  gate="${entry%%|*}"
  violation="${entry#*|}"
  dirty="$TMP/${gate%.sh}"
  plant "$dirty" "$violation"

  MARGINCE_EXT_DIR="$dirty" "$SCRIPT_DIR/$gate" >"$TMP/out" 2>&1 && verdict=0 || verdict=1
  if [[ "$verdict" -eq 0 ]]; then
    fail "$gate passed with a violation planted in extensions/probe/frontend: $violation"
  else
    for rel in "${DEPTHS[@]}"; do
      grep -qF "$dirty/$rel" "$TMP/out" && continue
      fail "$gate failed without naming the planted extensions/$rel — it is not reading that file:"
      sed 's/^/      /' "$TMP/out" >&2
    done
  fi

  MARGINCE_EXT_DIR="$CLEAN" "$SCRIPT_DIR/$gate" >"$TMP/out" 2>&1 || true
  for rel in "${DEPTHS[@]}"; do
    if grep -qF "$CLEAN/$rel" "$TMP/out"; then
      fail "$gate reported the clean extensions/$rel — the fixture proves nothing about scope while the gate also reports this:"
      sed 's/^/      /' "$TMP/out" >&2
    fi
  done
done

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "A design-system gate reads frontend/src and extensions/*/frontend, or it" >&2
  echo "holds the core to a standard the unit tier escapes. The extension walk is" >&2
  echo "the second find(1) arm in each gate, rooted at \$EXT_DIR." >&2
  exit 1
fi

echo "==> DS gate scope: ${#CASES[@]} gates each read a planted violation in extensions/*/frontend"
