#!/usr/bin/env bash
# VERSION names the release being tagged, or the release is refused.
#
# VERSION is the version a release PR sets and a reviewer reads. The tag is
# what the release workflow publishes. Checked here so the two never name
# different releases: a tag cut from a commit whose VERSION still says the last
# release would ship a tree that names itself wrongly.
#
# WHAT IT READS: its arguments and the file it is given. No git, no network, so
# its own test can plant a VERSION and judge the answer.
#
# Usage: bash scripts/release-version-file.sh v1.0.0 VERSION
set -euo pipefail

tag="${1:-}"
file="${2:-}"
if [[ -z "$tag" || -z "$file" ]]; then
	echo "release-version-file: pass the tag and the file, e.g. release-version-file.sh v1.0.0 VERSION" >&2
	exit 1
fi
if [[ ! -f "$file" ]]; then
	echo "release-version-file: $file does not exist." >&2
	exit 1
fi

# One line, the version without the tag's v. A second line or a stray space is
# refused rather than trimmed: a reader that runs `cat VERSION` gets the bytes.
want="${tag#v}"
if [[ "$(cat "$file")" != "$want" || "$(wc -l <"$file" | tr -d ' ')" != 1 ]]; then
	echo "release-version-file: $file says '$(head -c 80 "$file")'; $tag needs exactly one line, '$want'." >&2
	echo "  Set it in the release commit and merge that change." >&2
	echo "  Then delete the tag, and tag the merge commit again: docs/how-to/cut-a-release.md." >&2
	exit 1
fi
echo "release-version-file: $file carries $want."
