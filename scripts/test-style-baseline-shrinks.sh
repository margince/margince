#!/usr/bin/env bash
# check-style-baseline-shrinks.sh's own test, one throwaway git repository per case.
set -uo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=scripts/lib-git-fixture.sh
source "$(pwd)/scripts/lib-git-fixture.sh"
unset STYLE_BASELINE_REQUIRE_BASE

GATE_SRC="$(pwd)/scripts/check-style-baseline-shrinks.sh"
BASELINE="backend/internal/shared/kernel/langcopy/langcopytest/stylebaseline.txt"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fails=0
ran=0

BASE_TEXT='# header
en.dash	"one — two"
en.dash	"three — four"
'

# repo <dir>: a fresh repository with its own copy of the gate, which finds its root from $0.
repo() {
  local dir="$1"
  mkdir -p "$dir/scripts" "$dir/$(dirname "$BASELINE")"
  cp "$GATE_SRC" "$dir/scripts/check-style-baseline-shrinks.sh"
  git -C "$dir" init -q --template=
  git -C "$dir" config user.email probe@example.com
  git -C "$dir" config user.name probe
}

commit_base() {
  git -C "$1" add -A
  git -C "$1" commit -qm base
  git -C "$1" branch -q base
}

# expect <name> <dir> <pass|fail> <text the output holds> [VAR=value...]
expect() {
  local name="$1" dir="$2" want="$3" holds="$4" out got=pass
  shift 4
  ran=$((ran + 1))
  out="$(env "$@" bash "$dir/scripts/check-style-baseline-shrinks.sh" base 2>&1)" || got=fail
  if [[ "$got" != "$want" ]] || [[ "$out" != *"$holds"* ]]; then
    echo "FAIL: $name: want $want with \"$holds\", got $got:" >&2
    printf '%s\n' "$out" >&2
    fails=$((fails + 1))
  fi
}

# with_base <dir>: a repository whose branch `base` holds BASE_TEXT.
with_base() {
  repo "$1"
  printf '%s' "$BASE_TEXT" >"$1/$BASELINE"
  commit_base "$1"
}

with_base "$work/added"
printf 'en.dash\t"five — six"\n' >>"$work/added/$BASELINE"
expect added "$work/added" fail 'en.dash	"five — six"'

with_base "$work/swapped"
printf '# header\nen.dash\t"one — two"\nen.dash\t"three — five"\n' >"$work/swapped/$BASELINE"
expect swapped "$work/swapped" fail 'en.dash	"three — five"'

with_base "$work/removed"
printf '# header\nen.dash\t"one — two"\n' >"$work/removed/$BASELINE"
expect removed "$work/removed" pass 'adds no line'

with_base "$work/comment"
printf '# a new header\n# and a second line\n\nen.dash\t"one — two"\nen.dash\t"three — four"\n' >"$work/comment/$BASELINE"
expect comment "$work/comment" pass 'adds no line'

repo "$work/first-cut"
commit_base "$work/first-cut"
printf '%s' "$BASE_TEXT" >"$work/first-cut/$BASELINE"
expect first-cut "$work/first-cut" pass 'first cut'

repo "$work/recreated"
printf '%s' "$BASE_TEXT" >"$work/recreated/$BASELINE"
git -C "$work/recreated" add -A
git -C "$work/recreated" commit -qm first
git -C "$work/recreated" rm -q "$BASELINE"
commit_base "$work/recreated"
mkdir -p "$work/recreated/$(dirname "$BASELINE")"
printf '%s' "$BASE_TEXT" >"$work/recreated/$BASELINE"
expect recreated "$work/recreated" fail 'en.dash	"one — two"'

repo "$work/no-base"
git -C "$work/no-base" commit -q --allow-empty -m head
expect no-base-skips "$work/no-base" pass 'skip check-style-baseline-shrinks'
expect no-base-required "$work/no-base" fail 'git fetch origin main' STYLE_BASELINE_REQUIRE_BASE=1

repo "$work/unrelated"
printf '%s' "$BASE_TEXT" >"$work/unrelated/$BASELINE"
git -C "$work/unrelated" add -A
git -C "$work/unrelated" commit -qm head
orphan="$(git -C "$work/unrelated" commit-tree -m other "$(git -C "$work/unrelated" mktree </dev/null)")"
git -C "$work/unrelated" branch -q base "$orphan"
expect unrelated "$work/unrelated" fail 'share no merge base'

if [[ "$fails" -gt 0 ]]; then
  echo "test-style-baseline-shrinks: $fails of $ran case(s) failed" >&2
  exit 1
fi
echo "OK: test-style-baseline-shrinks — $ran case(s)"
