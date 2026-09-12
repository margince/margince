#!/usr/bin/env bash
# The tag mover's own test, over a real repository and a real remote.
#
# Every case pushes against a bare repository rather than mocking git, because
# the question is what git does: whether --force-with-lease rejects a stale
# expectation, and whether an ancestor is recognised as one. A stub would
# assert this file's idea of those answers.
#
# The case that matters most is the THIRD — an older release recording itself
# after a newer one already did — because it is the defect. The tag must stay
# on the newer revision; moving it back is what cuts the next patch from a base
# ahead of what consumers actually have, and it looks identical from the
# publishing side.
#
# Usage: bash scripts/publish-released-tag.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MOVE="$SCRIPT_DIR/publish-released-tag.sh"

FAILURES=0
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

TAG=released

fail() {
	printf 'FAIL: %s\n' "$1"
	FAILURES=$((FAILURES + 1))
}

# tagged answers the commit the remote's tag names, or nothing.
tagged() {
	git -C "$WORK/work" ls-remote --tags origin "refs/tags/$TAG" | awk 'NR == 1 { print $1 }'
}

commit() {
	git -C "$WORK/work" rev-parse --verify "$1"
}

git init -q --bare "$WORK/remote.git"
git init -q "$WORK/work"
cd "$WORK/work"
git config user.email gate@example.test
git config user.name "Release Gate"
git remote add origin "$WORK/remote.git"
for n in 1 2 3; do
	echo "$n" > file.txt
	git add file.txt
	git commit -qm "commit $n"
done
git push -q origin HEAD:refs/heads/main
C1="$(commit HEAD~2)"
C2="$(commit HEAD~1)"
C3="$(commit HEAD)"

# 1. The first publish, with no tag on the remote at all.
bash "$MOVE" "$TAG" "$C1" origin >/dev/null
[ "$(tagged)" = "$C1" ] || fail "the first publish did not record its revision"
printf 'ok    the first publish records its revision against no tag\n'

# 2. A later publish moves it forward.
bash "$MOVE" "$TAG" "$C3" origin >/dev/null
[ "$(tagged)" = "$C3" ] || fail "a descendant did not move the tag forward"
printf 'ok    a descendant moves it forward\n'

# 3. THE DEFECT: an older release recording itself after a newer one already did.
bash "$MOVE" "$TAG" "$C2" origin >/dev/null
if [ "$(tagged)" != "$C3" ]; then
	fail "an ancestor moved the tag BACKWARD — every later patch is then cut from a base ahead of what consumers have"
else
	printf 'ok    an ancestor leaves the newer revision alone\n'
fi

# 4. Re-recording the revision already there is a no-op and not a failure: a
#    re-run of a succeeded job must not fail the lane.
bash "$MOVE" "$TAG" "$C3" origin >/dev/null
[ "$(tagged)" = "$C3" ] || fail "re-recording the same revision disturbed the tag"
printf 'ok    re-recording the same revision changes nothing\n'

# 5. A revision this checkout does not have is refused rather than guessed at.
if bash "$MOVE" "$TAG" 0000000000000000000000000000000000000000 origin >/dev/null 2>&1; then
	fail "an unknown revision was accepted"
else
	printf 'ok    an unknown revision is refused\n'
fi

# 6. A tag naming an object this checkout cannot resolve is refused, not
#    overwritten: ordering against a revision we cannot see is exactly the
#    backward move this script exists to prevent.
git -C "$WORK/remote.git" update-ref "refs/tags/$TAG" "$(git -C "$WORK/remote.git" hash-object -w -t blob /dev/null)" 2>/dev/null || true
if git -C "$WORK/remote.git" rev-parse --verify -q "refs/tags/$TAG^{commit}" >/dev/null 2>&1; then
	printf 'ok    (skipped) this git resolved the planted tag to a commit\n'
elif bash "$MOVE" "$TAG" "$C3" origin >/dev/null 2>&1; then
	fail "a tag naming an unresolvable object was overwritten"
else
	printf 'ok    a tag this checkout cannot order against is refused\n'
fi

if [ "$FAILURES" -ne 0 ]; then
	printf 'publish-released-tag: %d case(s) failed\n' "$FAILURES" >&2
	exit 1
fi
printf 'publish-released-tag: ok — 6 case(s)\n'
