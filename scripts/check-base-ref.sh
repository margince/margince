#!/usr/bin/env bash
# Prints the branch this checkout was cut from: origin/develop or origin/main.
#
# The diff-scoped gates judge the lines added since CHECK_BASE_REF. Measured
# against the wrong one of the two, a branch is charged with every line the
# other branch is ahead by.
set -eu

# The base whose merge base with HEAD is nearest is the one the branch left.
nearest="origin/main"
fewest=""
for candidate in origin/main origin/develop; do
	git rev-parse --verify -q "$candidate" >/dev/null || continue
	fork="$(git merge-base "$candidate" HEAD 2>/dev/null)" || continue
	ahead="$(git rev-list --count "$fork..HEAD")"
	if [ -z "$fewest" ] || [ "$ahead" -lt "$fewest" ]; then
		nearest="$candidate"
		fewest="$ahead"
	fi
done
echo "$nearest"
