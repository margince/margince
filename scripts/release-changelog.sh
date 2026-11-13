#!/usr/bin/env bash
# The changelog a dist release carries, from every non-merge commit subject
# since its patch base.
#
# The dist service publishes no release without a changelog. The patch of a
# release starts at its patch base, so the changelog is built from the commit
# subjects since that base: it describes what the patch carries. A base that was skipped
# leaves its changes to the next patch, and to the next changelog too. Merge
# commits are left out, so a change that reached main through another branch
# is listed by its own subject.
#
# With a base, the subjects on stdin go through scripts/release-notes.sh: the
# same sections as the GitHub release, under one "Changes" heading, and a
# compare line that names the base by its dist version and links its git
# revision. A release with no subject a user notices gets only the compare
# line. A base with no subject since it is refused: the patch would be empty.
# The first release on a line has no base, reads no stdin and writes one fixed
# sentence. The dist page shows the version above the file, so the file names
# no version.
#
# Git does not check the bytes of a subject, and history is not rewritten, so
# an invalid UTF-8 byte in one subject would stop every later release. iconv
# drops such bytes before the subjects are read. The release CLI puts the file
# into its canonical form (line endings, Unicode normalization, one final line
# feed) and refuses an empty or too large one, so this script does not.
#
# WHAT IT READS: subjects on stdin and its arguments. No git, no network, so its
# own test can feed subjects and judge the answer.
#
# Usage: git log --no-merges --format=%s <base_revision>..<revision> |
#          bash scripts/release-changelog.sh <version> <owner/repo> <revision> <base_version> <base_revision>
#        bash scripts/release-changelog.sh <version> <owner/repo> <revision>   # the first release
#
# A refused call exits 1 and prints nothing on stdout: the workflow writes
# stdout to the file it uploads.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# The room for the notes. The dist service takes at most 1 MiB, and a character
# takes at most four bytes.
ROOM=250000

refuse() {
	printf 'release-changelog: %s\n' "$1" >&2
	exit 1
}

if [[ $# -ne 3 && $# -ne 5 ]]; then
	refuse "pass the version, owner/repo and revision, and for a base its version and revision; subjects on stdin."
fi
version="$1" repo="$2" revision="$3"
base_version="${4:-}" base_revision="${5:-}"
if [[ -z "$version" || -z "$repo" || -z "$revision" ]]; then
	refuse "the version, owner/repo and revision must not be empty."
fi

if [[ $# -eq 3 ]]; then
	printf 'The first release on this line: the patch is the whole tree.\n'
	exit 0
fi

if [[ -z "$base_version" || -z "$base_revision" ]]; then
	refuse "the base version and base revision must not be empty."
fi
subjects="$(iconv -f UTF-8 -t UTF-8 -c)"
if [[ -z "${subjects//[[:space:]]/}" ]]; then
	refuse "no commit subject since the patch base $base_version ($base_revision); the patch would be empty."
fi

notes="$(printf '%s\n' "$subjects" |
	bash "$SCRIPT_DIR/release-notes.sh" "$base_revision" "$revision" "$repo" "$ROOM" "$base_version")"

# Only the compare line: nothing a user notices, so no section to head.
if [[ "$notes" == '### '* ]]; then
	printf '## Changes\n\n'
fi
printf '%s\n' "$notes"
