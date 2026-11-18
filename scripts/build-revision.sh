#!/usr/bin/env bash
# The commit a local build names in its binaries, from a checkout or from an
# unpacked source download.
#
# A checkout answers with `git rev-parse HEAD`. A source download from GitHub
# has no .git, but `git archive` wrote the commit into REVISION (export-subst in
# .gitattributes). In a checkout REVISION holds the unfilled placeholder, which
# names no commit, so it is never read there. With neither, the answer is "dev",
# which internal/shared/buildinfo reads as "this build does not know".
#
# Usage: bash scripts/build-revision.sh [tree]   (default: this repository)
set -euo pipefail

tree="${1:-$(cd "$(dirname "$0")/.." && pwd)}"

if [[ -e "$tree/.git" ]] && revision="$(git -C "$tree" rev-parse HEAD 2>/dev/null)"; then
	echo "$revision"
	exit 0
fi
if [[ -f "$tree/REVISION" ]]; then
	revision="$(cat "$tree/REVISION")"
	if [[ "$revision" =~ ^[0-9a-f]{40}$ ]]; then
		echo "$revision"
		exit 0
	fi
fi
echo dev
