#!/usr/bin/env bash
# The build revision reader's own test, on the three trees a build starts from:
# a checkout, a source download, and a tree that knows neither.
#
# Usage: bash scripts/build-revision.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
READER="$SCRIPT_DIR/build-revision.sh"
# shellcheck source=scripts/lib-git-fixture.sh
source "$SCRIPT_DIR/lib-git-fixture.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FAILURES=0
expect() {
	local tree="$1" want="$2" reason="$3" got
	got="$(bash "$READER" "$tree")"
	if [[ "$got" != "$want" ]]; then
		printf 'FAIL: %s gave %s, want %s, because %s\n' "$tree" "$got" "$want" "$reason"
		FAILURES=$((FAILURES + 1))
	fi
}

# A checkout whose REVISION still holds the placeholder, as every checkout does.
checkout="$WORK/checkout"
mkdir -p "$checkout"
git -C "$checkout" init --quiet
printf '/REVISION export-subst\n' >"$checkout/.gitattributes"
printf '$Format:%%H$\n' >"$checkout/REVISION"
git -C "$checkout" add .gitattributes REVISION
git -C "$checkout" -c user.email=t@t -c user.name=t commit --quiet -m one
head="$(git -C "$checkout" rev-parse HEAD)"
expect "$checkout" "$head" 'a checkout names its own HEAD, never the placeholder'

# The same commit as a source download: git archive fills REVISION in.
download="$WORK/download"
mkdir -p "$download"
git -C "$checkout" archive HEAD | tar -x -C "$download"
expect "$download" "$head" 'a source download has no .git, and REVISION carries the commit it was cut from'

# A tree with no .git whose REVISION was never filled in.
bare="$WORK/bare"
mkdir -p "$bare"
cp "$checkout/REVISION" "$bare/REVISION"
expect "$bare" dev 'the placeholder names no commit, and a build that does not know says so'

if ((FAILURES)); then
	echo "build-revision: $FAILURES failure(s)"
	exit 1
fi
echo "OK: build-revision names the commit of a checkout and of a source download"
