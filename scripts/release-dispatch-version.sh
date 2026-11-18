#!/usr/bin/env bash
# The version of a dist release started by hand, or a refusal.
#
# Someone types the version, so it is checked against the dist grammar before
# the lane drafts anything. The dist service keeps one release line, so a
# release goes out from the main branch only.
#
# Usage: bash scripts/release-dispatch-version.sh <version> <full ref>
# Prints `version=<version>`, for appending to $GITHUB_OUTPUT.
set -euo pipefail

# shellcheck source=scripts/lib-dist-version.sh
source "$(dirname "$0")/lib-dist-version.sh"

version="${1-}"
ref="${2-}"

if [[ ! "$version" =~ $DIST_VERSION_RE ]]; then
	echo "release-dispatch-version: '$version' is not a dist version." >&2
	echo "  Type three numbers with no prefix or suffix, such as 1.0.0." >&2
	exit 1
fi
if [[ "$ref" != refs/heads/main ]]; then
	echo "release-dispatch-version: '$ref' is not the main branch." >&2
	echo "  Start the run from main: gh workflow run release.yml --ref main -f version=$version" >&2
	exit 1
fi

echo "version=$version"
