#!/usr/bin/env bash
# The frontend length gate's own test — and above all the RATCHET's, which is
# the half that fails silently.
#
# A ratchet has three refusals beyond the plain cap, and every one of them is a
# rule nobody meets until it fires: a waived file that GREW, a waived file that
# has come back under the cap and left a stale entry standing, and an entry for
# a file that no longer exists. None of those can be exercised against the real
# tree, where each would be a real failure — so the tree here is fake, and the
# gate reads it through FE_FILE_LENGTH_ROOT and FE_FILE_LENGTH_WAIVERS.
#
# The caps are lowered for the fixtures rather than writing thousand-line files:
# the gate takes them from the environment already, and a 12-line ceiling
# exercises the same comparisons a 500-line one does.
#
# Each property plants a defect the gate must catch, with a
# legitimate shape it must not:
#
#   1. A file under the cap passes.                (the control)
#   2. A product file over the cap fails.
#   3. A .test.tsx file over the PRODUCT cap passes — it has the wider one.
#   4. …and over the wider one fails, so 3 is not simply "tests are exempt".
#   5. All four fixture spellings take the wider ceiling.
#   6. A waived file at its frozen count passes.
#   7. A waived file one line past it FAILS.       (the ratchet)
#   8. A waived file back under the cap FAILS.     (the stale entry)
#   9. A waiver naming a file that is gone FAILS.
#
# Usage: bash scripts/check-fe-file-length.test.sh
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-fe-file-length.sh"
FAILURES=0

fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# lines writes a file of exactly n lines under the fake source root.
lines() {
  local path="$TMP/src/$1" count="$2"
  mkdir -p "$(dirname "$path")"
  : >"$path"
  for ((i = 0; i < count; i++)); do echo "const line$i = $i;" >>"$path"; done
}

# scriptlines writes one under the fake BUILD-SCRIPT root, the second tree the
# gate walks.
scriptlines() {
  local path="$TMP/scripts/$1" count="$2"
  mkdir -p "$(dirname "$path")"
  : >"$path"
  for ((i = 0; i < count; i++)); do echo "const line$i = $i;" >>"$path"; done
}

# run answers the gate's exit status over the fake tree and a waiver ledger
# built from the arguments ("<count> <path>" each, paths relative to the root).
run() {
  local waivers="$TMP/waivers.txt"
  : >"$waivers"
  local entry
  for entry in "$@"; do
    # The gate records paths as `find` prints them, which is root-prefixed. An
    # entry naming `scripts/…` is frozen under the build-script root, which is
    # how the ratchet is reached in the tree the gate learned second.
    local path="${entry#* }"
    case "$path" in
      scripts/*) echo "${entry% *} $TMP/${path}" >>"$waivers" ;;
      *) echo "${entry% *} $TMP/src/${path}" >>"$waivers" ;;
    esac
  done
  FE_FILE_LINE_CAP=10 FE_TEST_FILE_LINE_CAP=20 \
    FE_FILE_LENGTH_ROOT="$TMP/src" FE_FILE_LENGTH_SCRIPTS_ROOT="$TMP/scripts" \
    FE_FILE_LENGTH_WAIVERS="$waivers" \
    bash "$GATE" >/dev/null 2>&1
}

reset() { rm -rf "$TMP/src" "$TMP/scripts"; mkdir -p "$TMP/src" "$TMP/scripts"; }

# report answers what the gate SAID, for the cases where the refusal and the
# reason are different claims.
report() {
  local waivers="$TMP/waivers.txt"
  : >"$waivers"
  FE_FILE_LINE_CAP=10 FE_TEST_FILE_LINE_CAP=20 \
    FE_FILE_LENGTH_ROOT="$TMP/src" FE_FILE_LENGTH_SCRIPTS_ROOT="$TMP/scripts" \
    FE_FILE_LENGTH_WAIVERS="$waivers" \
    bash "$GATE" 2>&1
}

# 1. The control. Without it every refusal below could be the fixture failing
# to build a tree at all.
reset
lines "screens/small.tsx" 5
if ! run; then
  fail "a tree under the cap does not pass — the fixture is wrong, and every case below is meaningless"
fi

# 2. The plain cap on product code.
reset
lines "screens/big.tsx" 11
if run; then fail "a product file over the cap passed"; fi

# 3. A test file takes the WIDER ceiling: over the product cap, under the test
# cap, and therefore fine.
reset
lines "screens/big.test.tsx" 15
if ! run; then fail "a test file between the two caps was refused the wider ceiling"; fi

# 4. …but the wider one is a ceiling and not an exemption.
reset
lines "screens/huge.test.tsx" 21
if run; then fail "a test file over the TEST cap passed — the wider ceiling read as no ceiling"; fi

# 5. Every fixture spelling the tree uses. A predicate that knew only the
# dotted form would hand the other three the product cap.
for name in "screens/a.fixtures.ts" "screens/fixtures.ts" "screens/fixture.ts" "screens/test-fixtures.ts"; do
  reset
  lines "$name" 15
  if ! run; then fail "$name was not read as a fixture, so it took the product cap"; fi
done

# 6. A waived file AT its frozen count is the ordinary state of the debt list.
reset
lines "screens/waived.tsx" 30
if ! run "30 screens/waived.tsx"; then
  fail "a waived file at exactly its frozen count was refused"
fi

# 7. THE RATCHET. One line more than the ledger records.
reset
lines "screens/waived.tsx" 31
if run "30 screens/waived.tsx"; then
  fail "a waived file that GREW passed — the ratchet is the whole point of the ledger"
fi

# 8. The other direction: a file that has come back under the cap must lose its
# entry, or the cap stays disarmed for it forever.
reset
lines "screens/waived.tsx" 9
if run "30 screens/waived.tsx"; then
  fail "a waived file back under the cap passed with its entry still standing"
fi

# 9. An entry for a file that is gone certifies nothing while reading as though
# it does, and is a hole the next file can arrive through under that name.
reset
lines "screens/small.tsx" 5
if run "30 screens/deleted.tsx"; then
  fail "a waiver naming a file that no longer exists passed"
fi

# 10. The build-script tree is counted at all. The gate read frontend/src and
# nothing else, so a file here grew to a thousand lines with no gate to say so.
reset
scriptlines "lib/layout.ts" 15
if run; then fail "a build script over the cap passed — the second tree is not being counted"; fi

# 11. Plain ESM, which is half of what the build scripts are written in.
reset
scriptlines "build-apps.mjs" 15
if run; then fail "an .mjs build script over the cap passed — the extension list missed it"; fi

# 12. A test spelled .mjs takes the wider ceiling, as its .ts twin does.
reset
scriptlines "fe-uat.test.mjs" 15
if ! run; then fail "a .mjs test between the two caps was refused the wider ceiling"; fi

# 13. A waived build script AT its frozen count passes. This is the pair's
# load-bearing half: a ledger entry whose file the gate never walks reads as an
# entry for a deleted file, which fails for its own reason and would let the
# growth case below pass over a tree nobody counted.
reset
scriptlines "lib/layout.ts" 30
if ! run "30 scripts/lib/layout.ts"; then
  fail "a waived build script at its frozen count was refused — the ledger does not reach this tree"
fi

# 14. The ratchet reaches the second tree. One line more than the ledger
# records, which is what freezing a current size is worth.
reset
scriptlines "lib/layout.ts" 31
if run "30 scripts/lib/layout.ts"; then
  fail "a waived build script grew past its frozen count and passed"
fi

# 15. A root that is not there is NAMED. `find` already refuses it, by erroring
# on the missing path and failing the pipeline, so the refusal is not what this
# asks about: an operator reading `find: no such file` has to work out that a
# census just went uncounted. The gate says which root it could not read.
reset
lines "screens/small.tsx" 5
rm -rf "$TMP/scripts"
# Captured rather than piped: the gate exits non-zero here, and under pipefail
# that status would be the pipeline's however the grep went.
said="$(report)"
if ! grep -q "is not a directory, so this census would read short" <<<"$said"; then
  fail "a missing root is refused without being named, leaving the reader to infer that a tree went uncounted"
fi

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "check-fe-file-length.test.sh: $FAILURES case(s) failed" >&2
  exit 1
fi

# Counted from the numbered properties below rather than written down, because
# a total somebody keeps in step by hand is a total that stops being true.
echo "PASS — check-fe-file-length.sh holds on all $(grep -c '^# [0-9][0-9]*\.' "$0") properties"
