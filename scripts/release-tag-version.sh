#!/usr/bin/env bash
# The release tag IS the version. It names the build, both desktop bundles, the
# release page and the ref the D13 deployment pins to, and it is what a user
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

# vMAJOR.MINOR.PATCH, optionally a pre-release suffix. A `v0.1` or a `v0.1.2.3`
# names no version this scheme can order, and a release page carrying one is
# worse than a push that refused in seconds.
if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
	echo "release-tag-version: $tag is not a release version." >&2
	echo "  Expected vMAJOR.MINOR.PATCH, optionally -rc.1 / -beta.2." >&2
	echo "  Delete the tag, and push one that names a version." >&2
	exit 1
fi

# A suffixed tag publishes as a pre-release, so `v0.0.1-rc.1` does not become
# the download the release page offers by default. The same answer is what keeps
# a candidate out of the D13 production promotion, which selects non-suffixed
# tags only. One tag grammar, two shelves.
prerelease=false
case "$tag" in *-*) prerelease=true ;; esac

echo "version=$tag"
echo "prerelease=$prerelease"
