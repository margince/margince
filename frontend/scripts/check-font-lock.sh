#!/usr/bin/env bash
# Font-lock gate: the three-family type
# rule (design §2) — every font-family declaration under frontend/src or an
# extension unit's frontend names only Outfit (display), Geist (body), or Geist
# Mono (mono).
#
# Allowed besides the three families: the generic stack fallbacks the §2
# token definitions name (system-ui, sans-serif, ui-monospace, monospace) and
# var(--f-*) references, which resolve inside tokens.css.
#
# Fail-closed grep arm on top of the vitest conformance suite — same
# discipline even if the test tree regresses.
#
# Usage: frontend/scripts/check-font-lock.sh   (wired into `make frontend-check`)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="$(cd "$SCRIPT_DIR/.." && pwd)/src"
# The unit trees are swept too. A unit's screen is shipped UI in the same
# bundle, rendered on the same page, by an author the core team did not review
# line by line — so a gate that stopped at frontend/src would hold the core to a
# standard the extension tier escapes, which is the wrong way round. EXT_DIR is
# overridable so the gate's own test can point it at a fixture.
EXT_DIR="${MARGINCE_EXT_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)/extensions}"

FILES=()
while IFS= read -r -d '' f; do FILES+=("$f"); done < <(
  find "$SRC_DIR" -type f \( -name "*.ts" -o -name "*.tsx" -o -name "*.css" \) \
    -not -name "*.test.*" \
    -not -name "schema.d.ts" \
    -print0 2>/dev/null
)
while IFS= read -r -d '' f; do FILES+=("$f"); done < <(
  find "$EXT_DIR" -type f \( -name "*.ts" -o -name "*.tsx" -o -name "*.css" \) \
    -path "*/frontend/*" \
    -not -path "*/node_modules/*" \
    -not -name "*.test.*" \
    -print0 2>/dev/null
)

# An empty scan means the gate is pointed at the wrong tree — fail closed.
if [[ "${#FILES[@]}" -eq 0 ]]; then
  echo "FAIL: font-lock found no files under $SRC_DIR or $EXT_DIR — the gate is miswired" >&2
  exit 1
fi

echo "==> Font-lock check (${#FILES[@]} files under frontend/src + extensions/*/frontend)"

EXIT=0

# For each font-family declaration, strip everything allowed; any residue is
# a family outside the three-family rule.
while IFS= read -r hit; do
  value=$(echo "$hit" | grep -oE "font-family\s*:[^;]+" | head -1)
  [[ -z "$value" ]] && continue
  stripped=$(echo "$value" \
    | sed -E 's/font-family\s*://g' \
    | sed -E 's/var\(--[A-Za-z0-9-]+\)//g' \
    | sed -E 's/Geist Mono//g' \
    | sed -E 's/Geist//g' \
    | sed -E 's/Outfit//g' \
    | sed -E 's/system-ui//g' \
    | sed -E 's/sans-serif//g' \
    | sed -E 's/ui-monospace//g' \
    | sed -E 's/monospace//g' \
    | tr -d '",'"'"',; \t')
  if [[ -n "$stripped" ]]; then
    echo "FAIL (family outside the three-family rule): $hit"
    EXIT=1
  fi
done < <(
  printf '%s\0' "${FILES[@]}" \
    | xargs -0 grep -nHE "font-family\s*:" 2>/dev/null \
  || true
)

if [[ "$EXIT" == "0" ]]; then
  echo "PASS — only Outfit / Geist / Geist Mono (+ generic fallbacks)"
else
  echo ""
  echo "Allowed: Outfit, Geist, Geist Mono; generics system-ui,"
  echo "sans-serif, ui-monospace, monospace; var(--f-*) token references."
fi

exit $EXIT
