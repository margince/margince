#!/usr/bin/env bash
# The checked-out release commit is on main, or the release is refused.
#
# A release built from a commit that never merged is a release nobody
# reviewed. Both release lanes run this before they build, so the GitHub
# release and the dist release refuse the same tags.
#
# origin/main is asserted apart from the ancestry it feeds. A checkout without
# it cannot fetch it any more, and `merge-base` against an absent ref fails the
# same way a tag off main does. One message per cause, or a fetch-depth mistake
# reads as "your tag is not on main".
#
# WHAT IT READS: the tag, for the message, and the git checkout it runs in.
#
# Usage: bash scripts/release-tag-on-main.sh v1.0.0
set -euo pipefail

tag="${1:-}"
if [[ -z "$tag" ]]; then
	echo "release-tag-on-main: pass the tag." >&2
	exit 1
fi
if ! git rev-parse --verify --quiet origin/main >/dev/null; then
	echo "release-tag-on-main: origin/main is not in this checkout." >&2
	echo "  The ancestry check cannot run, so this refuses rather than" >&2
	echo "  passing blind. Check the checkout step's fetch-depth." >&2
	exit 1
fi
if ! git merge-base --is-ancestor HEAD origin/main; then
	echo "release-tag-on-main: $tag does not point at a commit on main." >&2
	echo "  Merge the work first, then tag the merge commit." >&2
	exit 1
fi
echo "release-tag-on-main: $tag is on main."
