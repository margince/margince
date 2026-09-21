#!/usr/bin/env bash
# The font-lock gate's own test — it gates the VERDICT of both arms.
#
# "The gate passed" and "the gate looked at nothing" print the same line, so
# every case runs the REAL gate over a fixture tree whose contents are known
# (MARGINCE_SRC_DIR, MARGINCE_EXT_DIR) and reads what it said about a named file.
#
# Two properties:
#   1. each refused shape is refused, and the finding names the planted file;
#   2. the shapes the rule allows stay silent — code, pre, samp and .code-block
#      in every spelling a stylesheet uses, the --fontFamilyMono token in tokens.css,
#      a var() reference with a fallback, and `inherit`. A gate that fires on
#      correct code teaches readers to stop reading it. (A grep reads comments
#      as it reads code, so a comment naming the class is refused here too;
#      design-system/mono.test.ts is the arm that reads past one.)
#
# Usage: bash frontend/scripts/check-font-lock.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-font-lock.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0

fail() {
  echo "FAIL: $*" >&2
  FAILURES=$((FAILURES + 1))
}

EMPTY_EXT="$TMP/no-extensions"
mkdir -p "$EMPTY_EXT"

# plant <tree> <relative path> <contents>
plant() {
  mkdir -p "$1/$(dirname "$2")"
  printf '%s\n' "$3" >"$1/$2"
}

# ---------------------------------------------------------------------------
# 1. Refused: one tree per shape, so a finding can only be about that shape.
# ---------------------------------------------------------------------------
REFUSED=(
  "screens/a.css|.foo { font-family: var(--fontFamilyMono); }"
  "screens/a.css|.foo { font: 12px/1 \"Geist Mono\", monospace; }"
  "screens/a.css|.foo, code { font-family: var(--fontFamilyMono); }"
  # One declaration spelled over four lines, under a selector list over two.
  "screens/a.css|"$'.foo,\ncode {\n  font-family:\n    var(--fontFamilyMono);\n}'
  "screens/a.css|code .label { font-family: ui-monospace; }"
  "screens/a.css|@media (width > 1px) { .foo { font-family: monospace; } }"
  "screens/a.css|pre { .label { font-family: var(--fontFamilyMono); } }"
  "screens/a.css|:root { --code-face: var(--fontFamilyMono); }"
  "screens/a.css|.t-mono { color: inherit; }"
  "screens/a.tsx|export const A = () => <span className=\"t-mono\">1</span>;"
  "screens/a.tsx|export const A = () => <b style={{ fontFamily: \"var(--fontFamilyMono)\" }} />;"
  "screens/a.tsx|const face = { fontFamily: 'monospace' };"
  # The three-family arm. A fourth family fails however it is spelled — and the
  # second of these is the one the var() stripping could hide: a fallback is what
  # a document that never defined the token actually renders in, so it is held to
  # the rule exactly like a bare family name.
  "screens/a.css|.foo { font-family: \"Comic Sans MS\"; }"
  "screens/a.css|.foo { font-family: var(--fontFamilyBody, \"Comic Sans MS\"); }"
)

index=0
for entry in "${REFUSED[@]}"; do
  index=$((index + 1))
  rel="${entry%%|*}"
  body="${entry#*|}"
  tree="$TMP/refused-$index"
  plant "$tree" "$rel" "$body"
  if MARGINCE_SRC_DIR="$tree" MARGINCE_EXT_DIR="$EMPTY_EXT" "$GATE" >"$TMP/out" 2>&1; then
    fail "the gate passed a refused shape in $rel: $body"
  elif ! grep -qF "$tree/$rel" "$TMP/out"; then
    fail "the gate failed without naming $rel for: $body"
    sed 's/^/      /' "$TMP/out" >&2
  fi
done

# ---------------------------------------------------------------------------
# 2. Allowed: one tree holding every allowed shape at once, which must pass.
# ---------------------------------------------------------------------------
CLEAN="$TMP/clean"
plant "$CLEAN" design-system/tokens.css ':root {
  --fontFamilyMono: "Geist Mono", ui-monospace, monospace;
}'
plant "$CLEAN" design-system/base.css 'code,
pre,
samp {
  font-family: var(--fontFamilyMono);
}
.t-num {
  font-variant-numeric: tabular-nums;
}'
plant "$CLEAN" screens/b.css '.foo code { font-family: var(--fontFamilyMono); }
pre.code-block:hover { font-family: monospace; }
.panel > .code-block { font-family: var(--fontFamilyMono); }
.a { & code { font-family: var(--fontFamilyMono); } }
pre { &:hover { font-family: var(--fontFamilyMono); } }
@media (width > 1px) { samp { font-family: ui-monospace; } }
.label { font-family: var(--fontFamilyBody); }
.reset { font-family: inherit; }
.figure { font-family: var(--fontFamilyHeading, var(--fontFamilyBody)); }
.stack { font-family: var(--fontFamilyBody, Geist), sans-serif; }'
plant "$CLEAN" screens/b.tsx 'export const B = () => <span className="t-num t-monochrome">1</span>;
const sheet = "pre code { font-family: var(--fontFamilyMono); }";'

if ! MARGINCE_SRC_DIR="$CLEAN" MARGINCE_EXT_DIR="$EMPTY_EXT" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate refused a tree holding only allowed shapes:"
  sed 's/^/      /' "$TMP/out" >&2
fi

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "check-font-lock.sh: $FAILURES verdict(s) wrong. Mono is for code: the gate" >&2
  echo "must refuse every shape above that dresses something else in it, and" >&2
  echo "must leave code, pre, samp, .code-block and the --fontFamilyMono token alone." >&2
  exit 1
fi

echo "==> font-lock verdicts: ${#REFUSED[@]} refused shapes named, the allowed shapes pass"
