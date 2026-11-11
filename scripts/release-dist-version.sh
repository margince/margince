#!/usr/bin/env bash
# The dist release version, derived from a v* tag by one fixed rule, so it can
# be reproduced from the tag and the repository's tags alone.
#
# THE RULE. vX.Y.Z maps to YYYY.EDITION.Z:
#   YYYY     the year, in UTC, that vX.Y.0 was tagged;
#   EDITION  vX.Y.0's place, from 1, among that year's vA.B.0 tags in version
#            order;
#   Z        the patch, unchanged.
# A patch keeps its minor's year and edition, so dist versions order as the
# tags do.
#
# A pre-release tag has no dist version, because the dist grammar has no
# pre-release. It prints dist=false, and the dist lane drafts nothing.
#
# WHAT IT READS: the tag, and the v*.*.0 tags with their dates. Run it where
# every tag is fetched.
#
# Usage: bash scripts/release-dist-version.sh v1.2.3
# Prints, for appending to $GITHUB_OUTPUT:
#   dist=true
#   version=2026.2.3
set -euo pipefail

tag="${1:-}"
bash "$(dirname "$0")/release-tag-version.sh" "$tag" > /dev/null
case "$tag" in *-*)
	echo "dist=false"
	exit 0
	;;
esac

core="${tag#v}"
major="${core%%.*}"
rest="${core#*.}"
minor="${rest%%.*}"
patch="${rest#*.}"
base="${major}.${minor}.0"

year="$(TZ=UTC git for-each-ref --format='%(creatordate:format-local:%Y)' "refs/tags/v$base")"
if [[ -z "$year" ]]; then
	echo "release-dist-version: $tag has no v$base tag to take its year and edition from." >&2
	echo "  Tag v$base first: a patch keeps its minor's year and edition." >&2
	exit 1
fi

# Every awk here reads to the end. One that exits on its match would close the
# pipe, and under pipefail the sort it cut off fails the script.
edition="$(TZ=UTC git for-each-ref --format='%(refname:short) %(creatordate:format-local:%Y)' 'refs/tags/v*' |
	awk -v y="$year" '$2 == y && $1 ~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.0$/ { print substr($1, 2) }' |
	LC_ALL=C sort -t. -k1,1n -k2,2n |
	awk -v b="$base" '{ n++ } $0 == b { found = n } END { print found }')"

echo "dist=true"
echo "version=${year}.${edition}.${patch}"
