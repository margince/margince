#!/usr/bin/env bash
# Prove check-comment-budget.sh fails on an over-budget change, passes an
# ordinary one, and does not judge a change too small to judge.
#
# The case that matters is the first. A budget gate that cannot fail reads
# exactly like a tree that is within budget, and this one is measured on a diff
# rather than on the tree, so nothing else in the repository would notice.
#
# Each case builds a throwaway repository with its own origin/main, so no case
# depends on the real tree's state or on another case.
set -uo pipefail
cd "$(dirname "$0")/.."

GATE_SRC="$(pwd)/scripts/check-comment-budget.sh"
fails=0
ran=0

# fixture <dir> — a repo with an origin/main to measure against.
fixture() {
	local dir="$1"
	mkdir -p "$dir/scripts" "$dir/backend"
	cp "$GATE_SRC" "$dir/scripts/check-comment-budget.sh"
	git -C "$dir" init -q --template=
	git -C "$dir" config user.email t@example.com
	git -C "$dir" config user.name t
	git -C "$dir" commit -q --allow-empty -m base
	git -C "$dir" branch -f origin/main HEAD
	# The gate asks for `origin/main`; a local branch of that name answers it
	# without a remote, which a throwaway repo has no way to provide.
	git -C "$dir" symbolic-ref refs/remotes/origin/main refs/heads/origin/main
}

# expect <name> <want-exit> — run the gate in $dir and judge its status.
expect() {
	local name="$1" want="$2" got
	( cd "$dir" && ./scripts/check-comment-budget.sh ) >/dev/null 2>&1
	got=$?
	ran=$((ran + 1))
	if [[ "$got" != "$want" ]]; then
		echo "FAIL: ${name} — gate exited ${got}, expected ${want}"
		fails=$((fails + 1))
	fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# An over-budget change: twenty comment lines explaining twelve of code.
dir="$tmp/over"
fixture "$dir"
{
	for i in $(seq 1 20); do echo "// why line ${i}"; done
	for i in $(seq 1 12); do echo "x${i} := ${i}"; done
} > "$dir/backend/over.go"
git -C "$dir" add -A && git -C "$dir" commit -q -m over
expect "an over-budget change fails" 1

# An ordinary change: four comment lines over twenty of code.
dir="$tmp/ok"
fixture "$dir"
{
	for i in $(seq 1 4); do echo "// why line ${i}"; done
	for i in $(seq 1 20); do echo "x${i} := ${i}"; done
} > "$dir/backend/ok.go"
git -C "$dir" add -A && git -C "$dir" commit -q -m ok
expect "an ordinary change passes" 0

# Under the floor: a three-line fix carrying a five-line why is not judged.
dir="$tmp/floor"
fixture "$dir"
{
	for i in $(seq 1 5); do echo "// why line ${i}"; done
	for i in $(seq 1 3); do echo "x${i} := ${i}"; done
} > "$dir/backend/floor.go"
git -C "$dir" add -A && git -C "$dir" commit -q -m floor
expect "a change under the floor is not judged" 0

# doc.go is a package's documentation, not an explanation of nearby code.
dir="$tmp/docgo"
fixture "$dir"
{
	for i in $(seq 1 30); do echo "// package prose ${i}"; done
	for i in $(seq 1 12); do echo "x${i} := ${i}"; done
} > "$dir/backend/doc.go"
git -C "$dir" add -A && git -C "$dir" commit -q -m docgo
expect "doc.go is exempt" 0

if (( fails )); then
	echo "FAIL: test-check-comment-budget — ${fails} of ${ran} case(s) failed"
	exit 1
fi
echo "OK: test-check-comment-budget — ${ran} cases"
