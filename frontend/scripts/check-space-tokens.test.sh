#!/usr/bin/env bash
# The space-tokens gate's own test. Each case runs the REAL gate over a fixture
# tree (MARGINCE_SRC_DIR, MARGINCE_EXT_DIR) and reads its verdict.
#
#   1. A tree that mints no inline property passes. Its grep matches nothing,
#      which xargs reports as a failure, and the gate must not read it as one.
#   2. A property used and defined nowhere fails, and the finding names it.
#   3. A tree with no declared property fails as miswired, not as clean.
#
# Usage: bash frontend/scripts/check-space-tokens.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-space-tokens.sh"

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

# gate <tree> runs the gate over <tree> and keeps what it said in $TMP/out.
gate() {
  MARGINCE_SRC_DIR="$1" MARGINCE_EXT_DIR="$EMPTY_EXT" bash "$GATE" >"$TMP/out" 2>&1
}

clean="$TMP/clean"
plant "$clean" design-system/tokens.css $':root {\n  --space-2: 8px;\n}'
plant "$clean" screens/a.css '.a { padding: var(--space-2); }'
if ! gate "$clean"; then
  fail "a tree with no inline property failed:"
  sed 's/^/      /' "$TMP/out" >&2
fi

missing="$TMP/missing"
plant "$missing" design-system/tokens.css $':root {\n  --space-2: 8px;\n}'
plant "$missing" screens/a.css '.a { padding: var(--space-5); }'
if gate "$missing"; then
  fail "the gate passed a property nothing defines"
elif ! grep -qF -- "--space-5" "$TMP/out"; then
  fail "the gate failed without naming --space-5:"
  sed 's/^/      /' "$TMP/out" >&2
fi

undeclared="$TMP/undeclared"
plant "$undeclared" design-system/tokens.css '/* no tokens */'
plant "$undeclared" screens/a.css '.a { padding: 8px; }'
if gate "$undeclared"; then
  fail "the gate passed a tree it read no property from"
elif ! grep -qF "miswired" "$TMP/out"; then
  fail "the gate failed an empty read without saying it is miswired:"
  sed 's/^/      /' "$TMP/out" >&2
fi

if ((FAILURES > 0)); then
  echo "check-space-tokens.test.sh: $FAILURES case(s) failed" >&2
  exit 1
fi
echo "==> space tokens: a tree with no inline property passes, an undefined one is named, an empty read is refused"
