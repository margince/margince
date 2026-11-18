#!/usr/bin/env bash
# The version of a dist release, or a refusal, checked before the lane drafts.
#
# A plain tag vX.Y.Z drafts the dist release X.Y.Z: the tag without its v.
# release-tag-version.sh checks the tag, so a tag has one grammar for both
# lanes. A pre-release tag has no dist version and is refused.
#
# A run started by hand carries a typed version instead. The dist service keeps
# one release line, so such a run goes out from the main branch only.
#
# Usage: bash scripts/release-dist-version.sh <full ref> [<typed version>]
# Prints `version=<x.y.z>`, for appending to $GITHUB_OUTPUT.
set -euo pipefail

# shellcheck source=scripts/lib-dist-version.sh
source "$(dirname "$0")/lib-dist-version.sh"

ref="${1-}"
typed="${2-}"

case "$ref" in
refs/tags/*)
	tag="${ref#refs/tags/}"
	read_tag="$(bash "$(dirname "$0")/release-tag-version.sh" "$tag")"
	if [[ "$tag" == *-* ]]; then
		echo "release-dist-version: $tag is a pre-release; the dist service has none." >&2
		exit 1
	fi
	version="$(sed -n 's/^release=//p' <<<"$read_tag")"
	;;
refs/heads/main)
	version="$typed"
	;;
*)
	echo "release-dist-version: '$ref' is neither a v* tag nor the main branch." >&2
	echo "  Push a tag, or start the run from main: gh workflow run release.yml --ref main -f version=1.0.0" >&2
	exit 1
	;;
esac

if [[ ! "$version" =~ $DIST_VERSION_RE ]]; then
	echo "release-dist-version: '$version' is not a dist version." >&2
	echo "  Type three numbers with no prefix or suffix, such as 1.0.0." >&2
	exit 1
fi

echo "version=$version"
