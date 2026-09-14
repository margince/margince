#!/usr/bin/env bash
# Move the tag naming the last published revision — FORWARD ONLY.
#
# The tag is the base every release patch is cut from
# (scripts/release-patch-base.sh), and its whole safety argument is ordering: a
# tag that failed to move leaves the next patch WIDER than necessary, which a
# consumer applies without harm, while a tag that moved BACKWARD is the narrow
# patch that silently drops a commit's files from every patch anyone will ever
# apply.
#
# Backward is not hypothetical. The publish job holds a concurrency group and
# the tag move is a job of its own, so two releases in flight are serialized at
# the publish and NOT at the move: an older release's move can land after a
# newer one's, and `git tag -f` would then point the tag at the older revision.
#
# Two rules make that impossible rather than unlikely:
#
#   1. The new revision must be a DESCENDANT of the tag's current one. An older
#      release is an ancestor and stops here, reporting that it did nothing.
#   2. The push is a compare-and-swap (--force-with-lease against the value just
#      read), so a move that lands between the check and the push is rejected
#      rather than overwritten. A rejection is retried once against the value
#      that won, which either finds itself an ancestor and stops or moves on.
#
# Usage: scripts/publish-released-tag.sh <tag> <revision> [remote]
set -euo pipefail

tag="${1-}"
revision="${2-}"
remote="${3:-origin}"

if [ -z "$tag" ] || [ -z "$revision" ]; then
	echo "usage: publish-released-tag.sh <tag> <revision> [remote]" >&2
	exit 2
fi

target="$(git rev-parse --verify -q "$revision^{commit}")" || {
	echo "publish-released-tag: $revision is not a commit in this checkout" >&2
	exit 1
}

# The REMOTE's value, not the checkout's: the tag lives on the remote and a
# fetched copy is as old as the fetch. An empty answer is the first publish.
remote_value() {
	git ls-remote --tags "$remote" "refs/tags/$tag" | awk 'NR == 1 { print $1 }'
}

attempt() {
	local current="$1"
	if [ -z "$current" ]; then
		# No tag yet: the lease is "it does not exist", which --force-with-lease
		# spells as an empty expected value.
		git push --force-with-lease="refs/tags/$tag:" "$remote" "$target:refs/tags/$tag"
		return
	fi
	# A tag object rather than a commit resolves through ^{commit}; a lightweight
	# one already is the commit. Either way the comparison is between commits.
	local current_commit
	if ! current_commit="$(git rev-parse --verify -q "$current^{commit}")"; then
		# The tag names something this checkout has not fetched. Refusing is the
		# safe answer: moving a tag past a revision we cannot order ourselves
		# against is exactly the backward move this script exists to prevent.
		echo "publish-released-tag: $tag names $current, which this checkout cannot resolve — not moving it" >&2
		exit 1
	fi
	if [ "$current_commit" = "$target" ]; then
		echo "publish-released-tag: $tag already names $target"
		return
	fi
	if ! git merge-base --is-ancestor "$current_commit" "$target"; then
		echo "publish-released-tag: $tag names $current_commit, which $target does not descend from — a newer release has already recorded itself, so this one leaves it alone"
		return
	fi
	git push --force-with-lease="refs/tags/$tag:$current" "$remote" "$target:refs/tags/$tag"
}

current="$(remote_value)"
if attempt "$current"; then
	exit 0
fi
# One retry, against whatever won the race. A second rejection is a real
# failure: two retries would be a loop waiting for a quiet moment that a busy
# release day may never have, and the lane must report rather than wait.
echo "publish-released-tag: $tag moved under us, re-reading it" >&2
attempt "$(remote_value)"
