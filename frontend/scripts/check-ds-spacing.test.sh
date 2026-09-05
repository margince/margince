#!/usr/bin/env bash
# The diff-scoped spacing gates' test — it gates their CENSUS, not their verdict.
#
# check-ds-spacing.sh and check-ds-spacing-roles.sh both report success when
# they inspected every file they should have AND when they inspected none of
# them, so a pathspec that quietly stops matching is indistinguishable from a
# clean tree. That is exactly what happened: 'extensions/*/frontend/**/*.tsx'
# was the only pattern for the unit tier, a git pathspec `**/` requires an
# intermediate directory, and every unit screen sits DIRECTLY at
# extensions/<unit>/frontend/screen.tsx — so three shipped units were exempt
# from the rule while the gate said PASS.
#
# Four things have to hold, and each is checked separately, because any one of
# them alone can be true while the gates collect nothing:
#
#   1. the pathspecs match every depth of every tree they name;
#   2. the collector returns the TRACKED and the UNTRACKED matches — a new file
#      is the strictest case there is and `git diff` cannot see one;
#   3. both gates reach for those pathspecs, and for no others;
#   4. what they collect in THIS repo is every file of that suffix in it.
#
# The pathspec lists are read out of lib/diff-scope.sh rather than restated
# here. A test that spells its own copy of the thing under test passes against
# the copy, not against production.
#
# Usage: bash frontend/scripts/check-ds-spacing.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LIB="$SCRIPT_DIR/lib/diff-scope.sh"
GATES=("$SCRIPT_DIR/check-ds-spacing.sh" "$SCRIPT_DIR/check-ds-spacing-roles.sh")

# The gates are deliberately fail-open outside a git checkout (a source export, a
# container that refuses a dubious-ownership repo). Their test has to skip on the
# same terms rather than turn `make fe-ds-gates` red for a reason that has
# nothing to do with spacing.
ROOT="$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "$ROOT" ]]; then
  echo "==> DS spacing census: not a git checkout — skipped"
  exit 0
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0

fail() {
  echo "FAIL: $*" >&2
  FAILURES=$((FAILURES + 1))
}

# The library's own declarations, evaluated verbatim — along with the collector
# under test in section 2. Two lines, or the library has been restructured and
# this test is reading something else.
DECLS="$(grep -E '^DIFF_SCOPE_(TSX|CSS)=\(' "$LIB" || true)"
if [[ "$(printf '%s\n' "$DECLS" | grep -c .)" -ne 2 ]]; then
  echo "FAIL: could not read DIFF_SCOPE_TSX and DIFF_SCOPE_CSS out of $LIB — the" >&2
  echo "      library no longer declares them on one line each, so this test is" >&2
  echo "      gating nothing. Re-point it at wherever the pathspecs now live." >&2
  exit 1
fi
# shellcheck source=frontend/scripts/lib/diff-scope.sh
. "$LIB"

# ---------------------------------------------------------------------------
# 1. The property, on a synthetic tree: each list must match every depth of
#    every tree it names — a file directly in it and a file nested under it.
#    Synthetic because the real repo ships no unit CSS, so the *.css half of the
#    defect is unreachable here and a census over real files would prove nothing
#    about it. `git ls-files` is the pathspec engine `git diff` and
#    `git ls-files --others` in the collector both match with.
# ---------------------------------------------------------------------------
REPO="$TMP/repo"
SHAPES=(
  frontend/src/App
  frontend/src/screens/deals/DealList
  extensions/probe/frontend/screen
  extensions/probe/frontend/views/panel
)
for shape in "${SHAPES[@]}"; do
  mkdir -p "$REPO/$(dirname "$shape")"
  : >"$REPO/$shape.tsx"
  : >"$REPO/$shape.css"
done
git -C "$REPO" init --quiet
for shape in "${SHAPES[@]}"; do
  git -C "$REPO" add -- "$shape.tsx" "$shape.css"
done

check_shape_census() {
  local kind="$1"
  shift
  local want got
  want="$(printf '%s\n' "${SHAPES[@]/%/.$kind}" | sort)"
  got="$(git -C "$REPO" ls-files -- "$@" | sort)"
  if [[ "$got" != "$want" ]]; then
    fail "the *.$kind pathspecs miss files the gates claim to inspect:"
    diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2 || true
  fi
}

check_shape_census tsx "${DIFF_SCOPE_TSX[@]}"
check_shape_census css "${DIFF_SCOPE_CSS[@]}"

# ---------------------------------------------------------------------------
# 2. The collector itself, against a commit: it must return the file this
#    branch EDITED and the file it merely added, at both depths. Reading the
#    arrays proves what they say; only running the function proves that both
#    halves of the sweep still expand them. An untracked file that stopped being
#    collected would exempt every brand-new stylesheet, which is the change with
#    the most new code in it.
# ---------------------------------------------------------------------------
git -C "$REPO" -c user.email=census@test -c user.name=census \
  commit --quiet -m "baseline" --no-gpg-sign
git -C "$REPO" update-ref refs/remotes/origin/main HEAD
printf '.a { gap: var(--space-2); }\n' >"$REPO/frontend/src/App.css"
printf '.b { gap: var(--space-2); }\n' >"$REPO/extensions/probe/frontend/fresh.css"

COLLECTED="$(diff_scope_changed "$REPO" "$(diff_scope_base "$REPO")" "${DIFF_SCOPE_CSS[@]}" | sort -u)"
for expected in frontend/src/App.css extensions/probe/frontend/fresh.css; do
  grep -qx "$expected" <<<"$COLLECTED" \
    || fail "diff_scope_changed did not collect $expected — it returned: $(tr '\n' ' ' <<<"$COLLECTED")"
done

# ---------------------------------------------------------------------------
# 3. The gates reach for those lists and for nothing else. A quoted *.tsx /
#    *.css glob anywhere in a gate is a second authority — the shape this pair
#    already regressed into once, when each gate carried its own copy.
# ---------------------------------------------------------------------------
grep -q -F '"${DIFF_SCOPE_TSX[@]}"' "${GATES[0]}" \
  || fail "check-ds-spacing.sh no longer expands \$DIFF_SCOPE_TSX — it is collecting *.tsx some other way"
for gate in "${GATES[@]}"; do
  grep -q -F '"${DIFF_SCOPE_CSS[@]}"' "$gate" \
    || fail "$(basename "$gate") no longer expands \$DIFF_SCOPE_CSS — it is collecting *.css some other way"

  # A PATH-shaped glob — one carrying a directory separator — is the second
  # authority this looks for. A bare '*.css' is not one: the roles gate sweeps
  # the design system with `find -name '*.css'`, which names no tree and cannot
  # drift away from one. Comment lines are dropped first, because the pathspec
  # trap is explained in prose and prose is not a call site.
  STRAY="$(grep -nE "'[^']*/[^']*\*[^']*\.(tsx|css)'" "$gate" | grep -vE '^[0-9]+:[[:space:]]*#' || true)"
  if [[ -n "$STRAY" ]]; then
    fail "$(basename "$gate") names a *.tsx/*.css pathspec of its own instead of the shared one:"
    printf '%s\n' "$STRAY" >&2
  fi
done

# ---------------------------------------------------------------------------
# 4. The census against THIS repo: the pathspecs must collect every tracked
#    file of that suffix under the two trees the gates say they gate. The
#    expected set is derived from the trees rather than from the patterns, so
#    the two cannot agree by both being wrong — and it is spelled with the same
#    depth-spanning semantics a git pathspec `*` has, so a unit that vendors a
#    sub-tree is expected on both sides rather than failing as a surplus.
# ---------------------------------------------------------------------------
check_repo_census() {
  local kind="$1"
  shift
  local want got
  want="$(git -C "$ROOT" ls-files -- frontend/src extensions \
    | grep -E "^(frontend/src/|extensions/.+/frontend/).*\.$kind\$" | sort || true)"
  got="$(git -C "$ROOT" ls-files -- "$@" | sort)"
  if [[ "$got" != "$want" ]]; then
    fail "the *.$kind pathspecs do not collect exactly the tracked *.$kind under frontend/src and extensions/*/frontend:"
    diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2 || true
  fi
}

check_repo_census tsx "${DIFF_SCOPE_TSX[@]}"
check_repo_census css "${DIFF_SCOPE_CSS[@]}"

# A count, not a verdict — but only where there is something to count.
# extensions/ is the fork-owned tier: a downstream shipping only backend units
# has no unit frontend at all, and "the gates inspect nothing there" is then a
# true statement about an empty tree rather than a broken pathspec.
EXT_TSX="$(git -C "$ROOT" ls-files -- "${DIFF_SCOPE_TSX[@]}" | grep -c '^extensions/' || true)"
EXT_SCREENS="$(git -C "$ROOT" ls-files -- 'extensions/*/frontend/*' | grep -c . || true)"
if [[ "$EXT_SCREENS" -gt 0 && "$EXT_TSX" -eq 0 ]]; then
  fail "the unit tier ships $EXT_SCREENS frontend file(s) and the *.tsx pathspecs collect 0 of them — the gates are inspecting nothing there"
fi

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "The spacing gates' collectors are no longer reaching every file they" >&2
  echo "report on. Note that a git pathspec is not :(glob): its '*' spans" >&2
  echo "directory separators, so ONE '<tree>/*.tsx' covers every depth, while" >&2
  echo "'<tree>/**/*.tsx' requires an intermediate directory and misses the" >&2
  echo "files sitting directly in <tree>." >&2
  exit 1
fi

echo "==> DS spacing census: the *.tsx and *.css pathspecs reach every depth of every gated tree, tracked and untracked, from both gates ($EXT_TSX unit-tier *.tsx collected)"
