#!/usr/bin/env bash
# The release tag IS the version. It names the build, both desktop bundles, the
# release page and the ref a D13 deployment will pin to, and it is what a user
# quotes in a bug report. Nothing derives it from anything else, so a malformed
# tag is refused here or it is shipped.
#
# WHAT IT READS: one argument. No git, no network, no environment — so its own
# test can plant a tag and judge the answer.
#
# Usage: bash scripts/release-tag-version.sh v0.0.1
# Prints, for appending to $GITHUB_OUTPUT:
#   version=v0.0.1
#   prerelease=false
set -euo pipefail

tag="${1:-}"
if [[ -z "$tag" ]]; then
	echo "release-tag-version: pass the tag, e.g. release-tag-version.sh v0.0.1" >&2
	exit 1
fi

# vMAJOR.MINOR.PATCH, optionally a pre-release suffix, with every component
# spelled exactly one way. A `v0.1` or a `v0.1.2.3` names no version this scheme
# can order; a `v01.2.3` names an orderable version by a second spelling, so two
# tags could claim one release and a reader could not tell which. A release page
# carrying either is worse than a push that refused in seconds.
if [[ ! "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$ ]]; then
	echo "release-tag-version: $tag is not a release version." >&2
	echo "  Expected vMAJOR.MINOR.PATCH with no leading zeros, optionally" >&2
	echo "  -rc.1 / -beta.2." >&2
	echo "  Delete the tag, and push one that names a version." >&2
	exit 1
fi

# A suffixed tag publishes as a pre-release, so `v0.0.1-rc.1` does not become
# the download the release page offers by default. That is the whole of what
# this answer buys today, and it stands on the release page alone. The D13
# deployment's production promotion is meant to select on the same answer and
# does not yet — nothing there reads a tag — so until it does, a candidate is
# kept off the default download rather than out of an environment. One tag
# grammar, two shelves.
prerelease=false
case "$tag" in *-*) prerelease=true ;; esac

echo "version=$tag"
echo "prerelease=$prerelease"
