#!/usr/bin/env bash
# Font-lock gate, two rules over one tree.
#
# 1. The three-family rule (design §2): every font-family declaration under
#    frontend/src or an extension unit's frontend names only Outfit (display),
#    Geist (body), or Geist Mono (code). Allowed besides the three families: the
#    generic stack fallbacks the §2 token definitions name (system-ui,
#    sans-serif, ui-monospace, monospace), var(--f-*) references, which resolve
#    inside tokens.css, and the `inherit` keyword, which names no family at all
#    — it takes whatever the root already resolved, and the root is the one
#    place a family is chosen.
#
# 2. Mono is for code. The code face is worn only by an element that IS code —
#    a rule whose selectors all have `code`, `pre`, `samp` or `.code-block` as
#    their subject. So this refuses:
#      - the `t-mono` class, anywhere, comments included (a figure takes t-num);
#      - a `font-family` / `font` declaration naming a mono family on any other
#        rule, and a custom property carrying one other than the `--fontFamilyMono`
#        token in design-system/tokens.css;
#      - a mono family inside a quoted TS/TSX string (an inline fontFamily, a
#        style string, a constant) unless that string is itself a code rule.
#
# Fail-closed grep arm on top of the vitest suites (conformance.test.ts for the
# families, design-system/mono.test.ts for mono) — same discipline even if the
# test tree regresses. check-font-lock.test.sh plants each shape and requires
# a verdict about it.
#
# Usage: frontend/scripts/check-font-lock.sh   (wired into `make frontend-check`)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# Both roots are overridable so the gate's own tests can point it at fixtures.
SRC_DIR="${MARGINCE_SRC_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)/src}"
# The unit trees are swept too. A unit's screen is shipped UI in the same
# bundle, rendered on the same page, by an author the core team did not review
# line by line — so a gate that stopped at frontend/src would hold the core to a
# standard the extension tier escapes, which is the wrong way round. EXT_DIR is
# overridable for the same reason as SRC_DIR.
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

# A var() reference names no family here — the token resolves in tokens.css —
# so the whole reference comes out. It is removed INNERMOST FIRST, one layer per
# pass, because a fallback may itself be a var(): `var(--fontFamilyHeading,
# var(--fontFamilyBody))` reduces to `var(--fontFamilyHeading, )` and only then to nothing.
#
# What must NOT happen is a var() swallowing its fallback whole: a rule written
# `var(--fontFamilyBody, "Comic Sans MS")` renders in Comic Sans on any document that
# never defined the token, so the fallback is held to the rule like any other
# value. That is why the second pattern requires the fallback to be EMPTY by the
# time it fires — an unstripped family inside one is residue, and residue fails.
strip_allowed() {
  sed -E 's/font-family[[:space:]]*://g' \
    | sed -E 's/Geist Mono//g' \
    | sed -E 's/Geist//g' \
    | sed -E 's/Outfit//g' \
    | sed -E 's/system-ui//g' \
    | sed -E 's/sans-serif//g' \
    | sed -E 's/ui-monospace//g' \
    | sed -E 's/monospace//g' \
    | sed -E 's/(^|[^A-Za-z0-9_-])inherit([^A-Za-z0-9_-]|$)/\1\2/g'
}

# For each font-family declaration, strip everything allowed; any residue is
# a family outside the three-family rule.
while IFS= read -r hit; do
  # A declaration whose value starts on the next line matches nothing here, and
  # under pipefail that no-match would end the whole gate with a bare exit 1.
  value=$(echo "$hit" | grep -oE "font-family\s*:[^;]+" | head -1 || true)
  [[ -z "$value" ]] && continue
  stripped=$(echo "$value" | strip_allowed)
  while :; do
    peeled=$(echo "$stripped" \
      | sed -E 's/var\([[:space:]]*--[A-Za-z0-9-]+[[:space:]]*\)//g' \
      | sed -E 's/var\([[:space:]]*--[A-Za-z0-9-]+[[:space:]]*,[[:space:]]*\)//g')
    [[ "$peeled" == "$stripped" ]] && break
    stripped="$peeled"
  done
  stripped=$(echo "$stripped" | tr -d '",'"'"',; \t')
  if [[ -n "$stripped" ]]; then
    echo "FAIL (family outside the three-family rule): $hit"
    EXIT=1
  fi
done < <(
  printf '%s\0' "${FILES[@]}" \
    | xargs -0 grep -nHE "font-family\s*:" 2>/dev/null \
  || true
)

# ---------------------------------------------------------------------------
# Mono is for code.
# ---------------------------------------------------------------------------

while IFS= read -r hit; do
  echo "FAIL (the t-mono class — mono is for code; a figure takes t-num): $hit"
  EXIT=1
done < <(
  printf '%s\0' "${FILES[@]}" \
    | xargs -0 grep -nHE '(^|[^A-Za-z0-9_-])t-mono([^A-Za-z0-9_-]|$)' 2>/dev/null \
  || true
)

# Selector-aware, because the question is WHICH element a declaration dresses:
# `.foo code` dresses code and `code .foo` does not. One unit per declaration,
# carried across lines; a nested rule whose subject starts with `&` answers
# with its parent's verdict, and an at-rule block (`@media`) inherits it.
# shellcheck disable=SC2016 # the awk program is single-quoted on purpose
MONO_CSS_SCANNER='
function decomment(line,   out, p) {
  out = ""
  while (length(line) > 0) {
    if (incomment) {
      p = index(line, "*/")
      if (p == 0) return out
      line = substr(line, p + 2)
      incomment = 0
    } else {
      p = index(line, "/*")
      if (p == 0) return out line
      out = out substr(line, 1, p - 1) " "
      line = substr(line, p + 2)
      incomment = 1
    }
  }
  return out
}
function is_mono(value,   lower) {
  # The token is matched as written — it has exactly one spelling in the tree,
  # capitals included — while a FAMILY name is cased however its author typed it.
  lower = tolower(value)
  return value ~ /var\([ \t]*--fontFamilyMono[ \t]*\)/ || lower ~ /geist mono/ || lower ~ /monospace/
}
# 1 = code, 0 = not code, 2 = "the parent`s verdict" (a subject starting with &).
function part_verdict(part,   n, arr, comp) {
  while (gsub(/\([^()]*\)/, "", part)) {}
  gsub(/\[[^]]*\]/, "", part)
  gsub(/[>+~]/, " ", part)
  gsub(/^[ \t]+|[ \t]+$/, "", part)
  n = split(part, arr, /[ \t]+/)
  comp = arr[n]
  sub(/:.*/, "", comp)
  if (comp ~ /^&/) return 2
  return (comp ~ /^(code|pre|samp)([^A-Za-z0-9_-]|$)/ || comp ~ /\.code-block([^A-Za-z0-9_-]|$)/) ? 1 : 0
}
function list_verdict(sel, parent,   n, parts, i, depth, ch, cur, v) {
  n = 0; depth = 0; cur = ""
  for (i = 1; i <= length(sel); i++) {
    ch = substr(sel, i, 1)
    if (ch == "(") depth++
    else if (ch == ")") { if (depth > 0) depth-- }
    if (ch == "," && depth == 0) { parts[++n] = cur; cur = ""; continue }
    cur = cur ch
  }
  parts[++n] = cur
  for (i = 1; i <= n; i++) {
    v = part_verdict(parts[i])
    if (v == 2) v = parent
    if (v != 1) return 0
  }
  return 1
}
function judge(   decl, prop, value) {
  decl = buf
  gsub(/^[ \t]+|[ \t]+$/, "", decl)
  if (decl !~ /^(font-family|font|--[A-Za-z0-9_-]+)[ \t]*:/) return
  prop = decl; sub(/[ \t]*:.*/, "", prop)
  value = decl; sub(/^[^:]*:/, "", value)
  if (!is_mono(value)) return
  if (prop ~ /^--/) {
    if (prop == "--fontFamilyMono" && FILENAME ~ /design-system\/tokens\.css$/) return
    printf "%s:%d: %s carries a mono family — the one code-face token is --fontFamilyMono in tokens.css\n", FILENAME, declline, prop
  } else if (!code[depth]) {
    printf "%s:%d: %s names a mono family on a rule that does not dress code, pre, samp or .code-block\n", FILENAME, declline, prop
  }
}
FNR == 1 { depth = 0; code[0] = 0; buf = ""; incomment = 0 }
{
  line = decomment($0)
  for (i = 1; i <= length(line); i++) {
    ch = substr(line, i, 1)
    if (ch == "{") {
      prelude = buf
      gsub(/^[ \t]+|[ \t]+$/, "", prelude)
      code[depth + 1] = (prelude ~ /^@/) ? code[depth] : list_verdict(prelude, code[depth])
      depth++
      buf = ""
    } else if (ch == ";" || ch == "}") {
      judge()
      if (ch == "}" && depth > 0) depth--
      buf = ""
    } else {
      if (buf ~ /^[ \t]*$/ && ch !~ /[ \t]/) declline = FNR
      buf = buf ch
    }
  }
  buf = buf " "
}
'

CSS_FILES=()
SCRIPT_FILES=()
for f in "${FILES[@]}"; do
  case "$f" in
    *.css) CSS_FILES+=("$f") ;;
    *) SCRIPT_FILES+=("$f") ;;
  esac
done
# Read byte-wise (LC_ALL=C): a stylesheet carries UTF-8 in its comments and
# strings, and a multibyte-aware awk stops at the first sequence it cannot
# decode. The scanner's exit status is checked rather than piped away, because
# a scanner that died half-way reads a smaller tree and prints nothing — which
# is exactly what a clean tree prints too.
MONO_CSS_OUT="$(mktemp)"
trap 'rm -f "$MONO_CSS_OUT"' EXIT
if [[ "${#CSS_FILES[@]}" -gt 0 ]]; then
  if ! LC_ALL=C awk "$MONO_CSS_SCANNER" "${CSS_FILES[@]}" >"$MONO_CSS_OUT"; then
    echo "FAIL: the mono scanner stopped before reading every stylesheet — the gate is miswired"
    EXIT=1
  fi
  while IFS= read -r hit; do
    echo "FAIL (mono outside code): $hit"
    EXIT=1
  done <"$MONO_CSS_OUT"
fi

# A quoted string naming the family. A string that is itself a code rule
# (`"pre code { font-family: var(--fontFamilyMono) }"`) is the one shape let through.
if [[ "${#SCRIPT_FILES[@]}" -gt 0 ]]; then
  while IFS= read -r hit; do
    if grep -qE '(^|[^A-Za-z0-9_-])(code|pre|samp|\.code-block)[^{"'"'"'`]*\{[^}]*(fontFamilyMono|Geist Mono|monospace)' <<<"$hit"; then
      continue
    fi
    echo "FAIL (mono family in a string — mono is for code): $hit"
    EXIT=1
  done < <(
    printf '%s\0' "${SCRIPT_FILES[@]}" \
      | xargs -0 grep -nHE '["'"'"'`][^"'"'"'`]*(var\(--fontFamilyMono\)|Geist Mono|monospace)[^"'"'"'`]*["'"'"'`]' 2>/dev/null \
    || true
  )
fi

if [[ "$EXIT" == "0" ]]; then
  echo "PASS — only Outfit / Geist / Geist Mono (+ generic fallbacks), and mono on code alone"
else
  echo ""
  echo "Allowed: Outfit, Geist, Geist Mono; generics system-ui,"
  echo "sans-serif, ui-monospace, monospace; var(--fontFamily*) token references,"
  echo "their fallbacks held to the same rule; and the inherit keyword."
  echo "Mono is for code: only code, pre, samp and .code-block wear it; a figure"
  echo "aligns with t-num (font-variant-numeric: tabular-nums) in the body face."
fi

exit $EXIT
