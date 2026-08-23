#!/usr/bin/env bash
# The two gates each resolve $0 through its symlinks before deriving their
# directory, and that block cannot be shared: finding a library needs the answer
# it produces, and through a symlink `dirname "$0"` is the link's directory.
#
# So it is duplicated deliberately, and this asserts the two copies are the SAME
# — which is what makes deliberate duplication safe rather than merely
# explained. The array refactor that fixed the visited-path set had to be made
# twice, by hand, and nothing would have noticed if only one had landed.
set -uo pipefail
cd "$(cd -P -- "$(dirname -- "$0")" && pwd)/.."

# The block runs from the marker comment to the line that sets SELF_DIR.
extract() {
  awk '/^# Resolve \$0 through any symlinks BEFORE deriving the directory/ { on = 1 }
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
