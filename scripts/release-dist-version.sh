#!/usr/bin/env bash
# The version of a dist release, or a refusal, checked before the lane drafts.
#
# A plain tag vX.Y.Z drafts the dist release X.Y.Z: the tag without its v.
# release-tag-version.sh reads the tag, so a tag has one grammar and one
# release value for both lanes. A pre-release tag has no dist version and is
# refused. A tag push and a run by hand both name the tag, so a dist release
# always describes the commit its tag names.
#
# Usage: bash scripts/release-dist-version.sh v1.0.0
# Prints `version=<x.y.z>`, for appending to $GITHUB_OUTPUT.
set -euo pipefail

# shellcheck source=scripts/lib-dist-version.sh
source "$(dirname "$0")/lib-dist-version.sh"

tag="${1-}"
read_tag="$(bash "$(dirname "$0")/release-tag-version.sh" "$tag")"
if [[ "$tag" == *-* ]]; then
	echo "release-dist-version: $tag is a pre-release; the dist service has none." >&2
	exit 1
fi
version="$(sed -n 's/^release=//p' <<<"$read_tag")"
if [[ ! "$version" =~ $DIST_VERSION_RE ]]; then
	echo "release-dist-version: '$tag' names no dist version." >&2
	echo "  Tag three numbers with a v and no suffix, such as v1.0.0." >&2
	exit 1
fi
echo "version=$version"
