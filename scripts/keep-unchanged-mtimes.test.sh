#!/usr/bin/env bash
# keep-unchanged-mtimes.sh's own test. The case that matters most is the one
# where it must NOT act: a file whose content changed, or one a developer had
# already edited, that kept an old time would let Go replay a stale test result.
#
# Usage: bash scripts/keep-unchanged-mtimes.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TOOL="$SCRIPT_DIR/keep-unchanged-mtimes.sh"
# shellcheck source=scripts/lib-git-fixture.sh
source "$SCRIPT_DIR/lib-git-fixture.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

mtime() { perl -MTime::HiRes=stat -e 'printf "%.6f\n", (stat $ARGV[0])[9]' "$1"; }

# Quote and dollar in the checkout path: the tool must hand it to git as one
# argument, or its view of what is dirty goes wrong.
repo="$TMP/re\"po\$x"
git init -q "$repo"
cd "$repo"
printf 'same\n' > same.txt
printf 'old\n' > changed.txt
printf 'edited\n' > edited.txt
# git reads a path that opens with a quote as a C-quoted string.
printf 'quoted\n' > '"quoted.txt'
git add -A
git -c user.email=t@example.com -c user.name=t commit -qm base
# A time far in the past, so "kept" and "rewritten now" cannot be confused.
touch -t 200001010000 same.txt changed.txt edited.txt
old="$(mtime same.txt)"
printf 'edited by hand\n' > edited.txt
touch -t 200001010001 edited.txt
edited_at="$(mtime edited.txt)"

regenerate() {
	printf 'same\n' > same.txt
	printf 'new\n' > changed.txt
	# Back to the committed bytes, which are NOT the bytes the last test run read.
	printf 'edited\n' > edited.txt
	printf 'quoted\n' > '"quoted.txt'
	printf 'x\n' > untracked.txt
}
snapshot="$TMP/snapshot"
"$TOOL" save "$snapshot"
regenerate
"$TOOL" restore "$snapshot" || fail "restore failed on the tree it saved"
[[ "$(mtime same.txt)" == "$old" ]] ||
	fail "a file rewritten with its own bytes got a new mtime; Go would re-run every test that reads it"
[[ "$(mtime changed.txt)" != "$old" ]] ||
	fail "a file whose content changed kept its old mtime; Go would replay a stale test result"
[[ "$(mtime edited.txt)" != "$edited_at" && "$(mtime edited.txt)" != "$old" ]] ||
	fail "a hand-edited file the generator wrote back to the committed bytes kept the time of the edit; Go would replay a result from the edited content"
[[ -f untracked.txt ]] || fail "the command did not run"

if "$TOOL" restore "$TMP/never-saved" 2>/dev/null; then
	fail "restore without a snapshot passed; a recipe that lost its save step would keep no time and say nothing"
fi

if (( FAILURES )); then
	echo "FAIL: keep-unchanged-mtimes — $FAILURES case(s)" >&2
	exit 1
fi
echo "OK: keep-unchanged-mtimes — same bytes keep their time, changed or edited files do not, a restore with no save fails"
