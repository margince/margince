#!/usr/bin/env bash
# test-lane-binaries.sh — unit test for scripts/lib-lanebinaries.sh, the
# directory of compiled integration test binaries.
#
# Each case is a way the binaries could cost a developer something with no
# failing check to show it:
#   1. a build pointed at a directory that holds other files deletes them;
#   2. a shard run after an edit tests the code from before the edit;
#   3. a duration read in a decimal-comma locale loses its fraction.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/lib-git-fixture.sh
source "$root_dir/scripts/lib-git-fixture.sh"
source "$root_dir/scripts/lib-lanebinaries.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fails=0

fail() {
  echo "FAIL: $*" >&2
  fails=$((fails + 1))
}

# A module of its own, so the cases can edit source without touching this
# checkout. gen.go is ignored by git and compiled, as build/composition is.
mod="$work/mod"
mkdir -p "$mod/lib" "$mod/docs"
printf 'module example.com/lanebin\n\ngo 1.21\n' > "$mod/go.mod"
printf 'package lib\n\nfunc Hand() int { return 1 }\n' > "$mod/lib/hand.go"
printf 'package lib\n\nfunc Gen() int { return 1 }\n' > "$mod/lib/gen.go"
printf '//go:build integration\n\npackage lib\n\nimport "testing"\n\nfunc TestHand(t *testing.T) {}\n' > "$mod/lib/hand_test.go"
printf '//go:build integration\n\npackage lib_test\n\nimport "testing"\n\nfunc TestOutside(t *testing.T) {}\n' > "$mod/lib/outside_test.go"
printf '//go:build other\n\npackage lib\n\nfunc Other() int { return 1 }\n' > "$mod/lib/other.go"
printf 'notes\n' > "$mod/docs/notes.md"
printf 'lib/gen.go\n' > "$mod/.gitignore"
git -C "$mod" init -q
git -C "$mod" add .
git -C "$mod" -c user.name=test -c user.email=test@example.com commit -qm base
export GOWORK=off GOFLAGS=-mod=mod

stamp_now() { lane_source_stamp "$mod" integration ./lib; }

# 1. The build empties only a directory it made.
foreign="$work/foreign"
mkdir -p "$foreign"
printf 'keep me\n' > "$foreign/thesis.txt"
if lane_build_dir_prepare "$foreign" "any" > "$work/out" 2>&1; then
  fail "a directory with a file the build did not write was emptied"
fi
[[ -f "$foreign/thesis.txt" ]] || fail "the foreign file did not survive the refused build"
grep -q 'did not write' "$work/out" || fail "the refusal does not say why: $(cat "$work/out")"

bins="$work/bins"
lane_build_dir_prepare "$bins" "any" > /dev/null || fail "a new directory was refused"
[[ -f "$bins/$LANE_BIN_STAMP" ]] || fail "the build wrote no $LANE_BIN_STAMP"
printf 'old\n' > "$bins/leftover.test"
lane_build_dir_prepare "$bins" "any" > /dev/null || fail "the build's own directory was refused"
[[ ! -e "$bins/leftover.test" ]] || fail "a second build kept a binary from the first"

empty="$work/empty"
mkdir -p "$empty"
lane_build_dir_prepare "$empty" "any" > /dev/null || fail "an empty directory without a stamp was refused"

# 2. A shard runs only binaries built from the source in front of it.
expect_check() { # label want(pass|fail)
  local label="$1" want="$2" got=pass now
  now="$(stamp_now)" || { fail "$label: the stamp did not compute"; return; }
  lane_bin_dir_check "$bins" "$now" > "$work/out" 2>&1 || got=fail
  [[ "$got" == "$want" ]] || fail "$label: want $want, got $got ($(cat "$work/out"))"
}
lane_build_dir_prepare "$bins" "$(stamp_now)" > /dev/null
expect_check "the source the build compiled" pass

edit_and_expect() { # label file want
  local label="$1" file="$2" want="$3"
  cp "$file" "$work/saved"
  printf '\n// edited\n' >> "$file"
  expect_check "$label" "$want"
  cp "$work/saved" "$file"
  expect_check "$label, undone" pass
}
edit_and_expect "an edit to a tracked source file" "$mod/lib/hand.go" fail
edit_and_expect "an edit to a compiled file git ignores" "$mod/lib/gen.go" fail
edit_and_expect "an edit to a test file" "$mod/lib/hand_test.go" fail
edit_and_expect "an edit to an external test file" "$mod/lib/outside_test.go" fail
edit_and_expect "an edit to go.mod" "$mod/go.mod" fail
edit_and_expect "an edit to a file the tags leave out" "$mod/lib/other.go" pass
edit_and_expect "an edit outside the compiled source" "$mod/docs/notes.md" pass

printf 'package lib\n\nfunc New() int { return 1 }\n' > "$mod/lib/new.go"
expect_check "a new file in a compiled package" fail
rm "$mod/lib/new.go"

rm "$bins/$LANE_BIN_STAMP"
expect_check "a directory the build did not fill" fail

# 3. A duration reads the same with either decimal separator. The parsing is
# checked on synthetic times, so it holds on a machine with no comma locale.
expect_elapsed() { # t0 t1 want
  local got
  got="$(lane_elapsed "$1" "$2")"
  [[ "$got" == "$3" ]] || fail "lane_elapsed $1 $2: want $3, got $got"
}
expect_elapsed 100.250000 101.750000 1.500
expect_elapsed 100,250000 101,750000 1.500
expect_elapsed 1793936610,160258 1793936612,5 2.339
expect_elapsed 1793936610 1793936611 1.000

# And on the clock itself, under every comma locale this machine has.
comma_locales=0
while IFS= read -r locale; do
  t0="$(LC_ALL="$locale" bash -c 'printf "%s" "$EPOCHREALTIME"' 2>/dev/null)"
  [[ "$t0" == *,* ]] || continue
  comma_locales=$((comma_locales + 1))
  got="$(lane_elapsed "$t0" "$t0")"
  [[ "$got" == "0.000" ]] || fail "lane_elapsed on a $locale clock reading $t0 gave $got"
done < <(locale -a 2>/dev/null | grep -iE '^(de_DE|fr_FR|it_IT|es_ES|nl_NL|pt_BR|ru_RU)\.utf-?8$' || true)
if (( comma_locales == 0 )); then
  echo "test-lane-binaries: no decimal-comma locale installed; the clock is checked on synthetic times only"
fi

if (( fails > 0 )); then
  echo "test-lane-binaries: $fails failure(s)" >&2
  exit 1
fi
echo "OK: test-lane-binaries — a foreign directory survives, stale binaries are refused, and durations read the same with either decimal separator"
