#!/usr/bin/env bash
# Every custom property the tree USES without a fallback must be DEFINED
# somewhere in the tree.
#
# An undefined custom property does not fall back to a smaller value — it
# resolves to nothing, and the declaration is dropped. `--space-5` was missing
# from the scale while six rules across the tree spelled it, so a drawer that
# declared `padding: var(--space-5)` rendered with NO padding and clipped its
# own heading against the viewport edge. `--text-muted` and `--text-sm` were
# never tokens at all, and two settings surfaces asked for them: that copy
# rendered as ordinary body text, in the primary ink, and read as a styling
# choice nobody made. Nothing failed: not the typecheck, not the unit tests
# (jsdom does not resolve custom properties), not the spacing gate, which only
# reads raw px.
#
# This is the fitness-function form of that bug: derive the obligation from the
# tree rather than maintain a list. `var(--x, fallback)` is deliberately exempt
# — a spelled fallback is what an intentionally optional property looks like
# (the rail reads `--shellEase` that way, because it also renders outside the
# app shell that defines it).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

# The unit trees are swept too. A unit's screen is shipped UI in the same
# bundle, rendered on the same page, and a `var(--x)` nothing defines drops its
# declaration there exactly as it does in core. Custom properties are global to
# the document, so one tree may legitimately use what the other declares — which
# is why both trees feed BOTH sides of the comparison, not just the used side.
# EXT_DIR is overridable so the gate's own test can point it at a fixture.
EXT_DIR="${MARGINCE_EXT_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)/extensions}"

# A unit ships no frontend at all in a backend-only downstream, so an empty
# result here is a true statement about an empty tree rather than a miswiring —
# the emptiness that matters is checked below, over both trees together.
ext_frontend() {
  find "$EXT_DIR" -type f \( "$@" \) \
    -path "*/frontend/*" \
    -not -path "*/node_modules/*" 2>/dev/null || true
}

TOKENS="src/design-system/tokens.css"
if [[ ! -f "$TOKENS" ]]; then
  echo "FAIL: $TOKENS not found"
  exit 1
fi

styles=$(
  find src -type f -name '*.css'
  ext_frontend -name '*.css'
)
sources=$(
  find src -type f \( -name '*.css' -o -name '*.tsx' -o -name '*.ts' \)
  ext_frontend -name '*.css' -o -name '*.tsx' -o -name '*.ts'
)
if [[ -z "$styles" || -z "$sources" ]]; then
  echo "FAIL: scanned no files — the gate is miswired"
  exit 1
fi

# A declaration in any stylesheet counts: tokens.css owns the design scale, and
# a component sheet may own a property of its own (--shellAnim, --pageColumn).
declared=$(echo "$styles" | tr '\n' '\0' | xargs -0 grep -hoE -- '^[[:space:]]*--[a-zA-Z0-9-]+[[:space:]]*:' 2>/dev/null \
  | tr -d ' \t' | sed 's/:$//' | sort -u)
# A component may also mint one on an element it renders: style={{ "--x": … }}.
inline=$(echo "$sources" | tr '\n' '\0' | xargs -0 grep -hoE -- '"--[a-zA-Z0-9-]+":' 2>/dev/null \
  | tr -d '"' | sed 's/:$//' | sort -u)
defined=$(printf '%s\n%s\n' "$declared" "$inline" | grep -v '^$' | sort -u)

used=$(echo "$sources" | tr '\n' '\0' | xargs -0 grep -hoE -- 'var\(--[a-zA-Z0-9-]+\)' 2>/dev/null \
  | sed 's/^var(//;s/)$//' | sort -u)

missing=$(comm -13 <(echo "$defined") <(echo "$used"))

if [[ -n "$missing" ]]; then
  echo "FAIL: the tree uses custom properties nothing defines"
  echo ""
  while read -r token; do
    [[ -z "$token" ]] && continue
    echo "  $token — used in:"
    echo "$sources" | tr '\n' '\0' | xargs -0 grep -ln -- "var($token)" 2>/dev/null \
      | sed 's/^/      /'
  done <<< "$missing"
  echo ""
  echo "An undefined custom property resolves to NOTHING: the declaration is"
  echo "dropped and the element renders with no value at all. Define it in"
  echo "$TOKENS, use a property that exists, or spell a fallback if it is"
  echo "genuinely optional."
  exit 1
fi

echo "OK: every custom property used is defined"
