#!/usr/bin/env bash
# The base resolver's own test, over a real repository.
#
# Every case is built as commits rather than mocked, because the question is
# what git resolves: a tag that moved, a tag that never existed, a `before` the
# history no longer reaches. A stub of `git rev-parse` would assert this file's
# idea of those answers rather than git's.
#
# The case that matters most is the THIRD one — publish at N-1, skip at N,
# publish at N+1 — because it is the defect. The base must be N-1, so the patch
# carries N's files; a base of N is what dropped them, and it looks identical
# from the publishing side.
#
# Usage: bash scripts/release-patch-base.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RESOLVE="$SCRIPT_DIR/release-patch-base.sh"

FAILURES=0
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# expect <name> <want> <published> <before> <fallback>
expect() {
	local name="$1" want="$2" got
	shift 2
	got="$(cd "$WORK" && bash "$RESOLVE" "$@")"
	if [ "$got" != "$want" ]; then
		printf 'FAIL: %s — base %s, want %s\n' "$name" "${got:-<none>}" "${want:-<none>}"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s\n' "$name"
}

cd "$WORK"
git init -q .
git config user.email gate@example.test
git config user.name "Release Gate"
for n in 1 2 3 4; do
	echo "$n" >"file$n"
	git add "file$n"
	git commit -qm "commit $n"
	eval "C$n=$(git rev-parse HEAD)"
done

# THE DEFECT. Published at commit 2, the lane at commit 3 did not publish, and
# commit 4 is releasing now. The base has to be 2 — the tree a consumer holds —
# so the patch carries commit 3's file. `before` says 3, which is what dropped it.
git tag released "$C2"
expect "a skipped lane leaves its files in the next patch" "$C2" released "$C3" HEAD~1

# The ordinary case is the same rule, and it reads the same: every lane
# published, so the tag and the previous tip agree.
git tag -f released "$C3" >/dev/null 2>&1
expect "an unbroken sequence resolves to the previous release" "$C3" released "$C3" HEAD~1

# NOTHING PUBLISHED YET. The first release of a repository has no previous
# state, and the previous tip is the best remaining answer rather than a
# failure — it is right whenever the lane before this one published.
git tag -d released >/dev/null 2>&1
expect "no recorded release falls back to the previous tip" "$C3" released "$C3" HEAD~1

# Branch creation carries an all-zeros before, which is not a commit and must
# not be treated as one.
expect "branch creation falls through to the fallback" "$C3" released "0000000000000000000000000000000000000000" HEAD~1

# A manual dispatch carries no before at all.
expect "a manual dispatch falls through to the fallback" "$C3" released "" HEAD~1

# A force-push discards its old tip: `before` names a commit a full fetch of the
# new tip cannot resolve. Not an error — simply not a base.
expect "an unreachable before is skipped, not reported" "$C3" released "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" HEAD~1

# And the honest empty answer: a first release with nothing behind it at all.
(
	cd "$WORK"
	git init -q first
	cd first
	git config user.email gate@example.test
	git config user.name "Release Gate"
	echo one >file
	git add file
	git commit -qm "the first commit"
	got="$(bash "$RESOLVE" released "" HEAD~1)"
	if [ -n "$got" ]; then
		printf 'FAIL: a first commit resolved a base (%s) — a release with nothing behind it owes no patch\n' "$got"
		exit 1
	fi
	printf 'ok    a repository with one commit resolves no base at all\n'
) || FAILURES=$((FAILURES + 1))

if [ "$FAILURES" -ne 0 ]; then
	printf '\nrelease-patch-base: %d case(s) failed\n' "$FAILURES" >&2
	exit 1
fi
echo "OK: release-patch-base — every case, including the skipped lane that made this a defect"
