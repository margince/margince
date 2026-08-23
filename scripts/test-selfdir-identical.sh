#!/usr/bin/env bash
# The two gates each resolve $0 through its symlinks before deriving their
# directory, and that block cannot be shared: finding a library needs the answer
# it produces, and through a symlink `dirname "$0"` is the link's directory.
#
# So it is duplicated deliberately, and this asserts the two copies are the SAME
# — which is what makes deliberate duplication safe rather than merely
# explained. An edit to one copy that is not made to the other is a gate whose
# two halves resolve their own location differently, and nothing else in the
# tree would notice.
set -uo pipefail
# CDPATH= : with one set, `cd` prints the directory it chose, and the printed
# path lands in the substitution instead of the intended one.
CDPATH=
cd "$(cd -P -- "$(dirname -- "$0")" && pwd)/.."

# From the note that explains WHY it is duplicated, down to the line that sets
# SELF_DIR. The note is inside the compared span on purpose: an explanation that
# can drift while the code stays identical is how one copy comes to be described
# correctly and the other not.
extract() {
  awk '/^# THE ONE PLACE IN THIS PAIR THAT IS DUPLICATED ON PURPOSE/ { on = 1 }
       on { print }
       /^SELF_DIR=/ { if (on) exit }' "$1"
}

a="$(extract scripts/check-one-spelling.sh)"
b="$(extract scripts/check-money-scale.sh)"

if [[ -z "$a" || -z "$b" ]]; then
  echo "FAIL: the self-resolution block was not found in both gates — this test is asserting nothing"
  exit 1
fi
if [[ "$a" != "$b" ]]; then
  echo "FAIL: the two gates resolve their own location differently. The duplication is deliberate"
  echo "      and documented, but only because the copies are identical — one of them has drifted:"
  diff <(echo "$a") <(echo "$b") | sed 's/^/    /'
  exit 1
fi
printf 'OK: both gates resolve their own location the same way (%d lines, byte-identical)\n' "$(wc -l <<< "$a")"
