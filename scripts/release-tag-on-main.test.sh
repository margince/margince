#!/usr/bin/env bash
# The on-main check's own test: a commit on main passes, a commit off main and a
# checkout without origin/main are refused, each with its own message.
#
# Usage: bash scripts/release-tag-on-main.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/release-tag-on-main.sh"
# shellcheck source=scripts/lib-git-fixture.sh
source "$SCRIPT_DIR/lib-git-fixture.sh"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
FAILURES=0

# verdict <want: pass|refuse> <message part> <reason>, run in $WORK/repo.
verdict() {
	local want="$1" part="$2" reason="$3" out status=0
	out="$(cd "$WORK/repo" && bash "$CHECK" v1.0.0 2>&1)" || status=$?
	if [[ "$want" == pass && "$status" -ne 0 ]] || [[ "$want" == refuse && "$status" -eq 0 ]]; then
		printf 'FAIL: wanted %s, got exit %s, because %s\n' "$want" "$status" "$reason"
		FAILURES=$((FAILURES + 1))
	elif [[ "$out" != *"$part"* ]]; then
		printf 'FAIL: the message lacks %q, because %s\n  got: %s\n' "$part" "$reason" "$out"
		FAILURES=$((FAILURES + 1))
	fi
}

repo="$WORK/repo"
git init --quiet "$repo"
commit() { git -C "$repo" -c user.email=t@t -c user.name=t commit --quiet --allow-empty -m "$1"; }
commit one
main_commit="$(git -C "$repo" rev-parse HEAD)"
git -C "$repo" update-ref refs/remotes/origin/main "$main_commit"

verdict pass 'is on main' 'the main tip is a commit on main'

commit off-main
verdict refuse 'does not point at a commit on main' 'a commit main never merged was reviewed by nobody'

git -C "$repo" checkout --quiet --detach "$main_commit"
git -C "$repo" update-ref -d refs/remotes/origin/main
verdict refuse 'origin/main is not in this checkout' 'a check that cannot run must not pass, and must say why'

if ((FAILURES)); then
	echo "release-tag-on-main: $FAILURES failure(s)"
	exit 1
fi
echo "OK: release-tag-on-main passes a commit on main and refuses the rest"
