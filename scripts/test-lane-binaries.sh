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
source "$root_dir/scripts/lib-lanebinaries.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fails=0

fail() {
  echo "FAIL: $*" >&2
  fails=$((fails + 1))
}

# A checkout of its own, so the cases can edit source without touching this one.
repo="$work/repo"
mkdir -p "$repo/backend" "$repo/docs"
git -C "$repo" init -q
printf 'package main\n' > "$repo/backend/main.go"
printf 'notes\n' > "$repo/docs/notes.md"
git -C "$repo" add .
git -C "$repo" -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false commit -qm base

# 1. The build empties only a directory it made.
foreign="$work/foreign"
mkdir -p "$foreign"
printf 'keep me\n' > "$foreign/thesis.txt"
if lane_build_dir_prepare "$foreign" "$repo" > "$work/out" 2>&1; then
  fail "a directory with a file the build did not write was emptied"
fi
[[ -f "$foreign/thesis.txt" ]] || fail "the foreign file did not survive the refused build"
grep -q 'did not write' "$work/out" || fail "the refusal does not say why: $(cat "$work/out")"

bins="$work/bins"
lane_build_dir_prepare "$bins" "$repo" > /dev/null || fail "a new directory was refused"
[[ -f "$bins/$LANE_BIN_STAMP" ]] || fail "the build wrote no $LANE_BIN_STAMP"
printf 'old\n' > "$bins/leftover.test"
lane_build_dir_prepare "$bins" "$repo" > /dev/null || fail "the build's own directory was refused"
[[ ! -e "$bins/leftover.test" ]] || fail "a second build kept a binary from the first"

empty="$work/empty"
mkdir -p "$empty"
lane_build_dir_prepare "$empty" "$repo" > /dev/null || fail "an empty directory without a stamp was refused"

# 2. A shard runs only binaries built from the source in front of it.
expect_check() { # label want(pass|fail)
  local label="$1" want="$2" got=pass
  lane_bin_dir_check "$bins" "$repo" > "$work/out" 2>&1 || got=fail
  [[ "$got" == "$want" ]] || fail "$label: want $want, got $got ($(cat "$work/out"))"
}
lane_build_dir_prepare "$bins" "$repo" > /dev/null
expect_check "the checkout the build compiled" pass

printf 'package main\n\nfunc f() {}\n' > "$repo/backend/main.go"
expect_check "an uncommitted edit under backend/" fail
grep -q 'make test-integration' "$work/out" || fail "the mismatch does not name the rebuild: $(cat "$work/out")"
git -C "$repo" checkout -q -- backend/main.go
expect_check "the edit undone" pass

printf 'package main\n' > "$repo/backend/new.go"
expect_check "a new untracked file under backend/" fail
rm "$repo/backend/new.go"

printf 'more notes\n' >> "$repo/docs/notes.md"
expect_check "an edit outside the compiled source" pass
git -C "$repo" checkout -q -- docs/notes.md

git -C "$repo" -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false commit -q --allow-empty -m next
expect_check "a different commit" fail

rm "$bins/$LANE_BIN_STAMP"
expect_check "a directory the build did not fill" fail

# 3. The clock reads with a decimal point in any locale the machine has.
for locale in de_DE.UTF-8 fr_FR.UTF-8 C; do
  now="$(LC_ALL="$locale" bash -c 'source "$1"; lane_now' _ "$root_dir/scripts/lib-lanebinaries.sh" 2>/dev/null)"
  [[ "$now" =~ ^[0-9]+(\.[0-9]+)?$ ]] || fail "lane_now under $locale printed '$now'"
done

if (( fails > 0 )); then
  echo "test-lane-binaries: $fails failure(s)" >&2
  exit 1
fi
echo "OK: test-lane-binaries — a foreign directory survives, stale binaries are refused, and the clock reads the same in every locale"
