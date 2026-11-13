#!/usr/bin/env python3
"""The changelog a dist release carries, from every non-merge commit subject
since its patch base.

The dist service publishes no release without a changelog. The patch of a
release starts at its patch base, so the changelog is built from the commit
subjects since that base: it describes what the patch carries. A base that was
skipped leaves its changes to the next patch, and to the next changelog too.
Merge commits are left out, so a change that reached main through another
branch is listed by its own subject.

With a base, the subjects on stdin go through scripts/release_notes.py: the same
sections as the GitHub release, under one "Changes" heading, and a compare line
that names the base by its dist version and links its git revision. A release
with no subject a user notices gets only the compare line. A base with no
subject since it is refused: the patch would be empty. The first release on a
line has no base, reads no stdin and writes one fixed sentence. The dist page
shows the version above the file, so the file names no version.

Git does not check the bytes of a subject, and history is not rewritten, so an
invalid UTF-8 byte in one subject would stop every later release. Such bytes are
dropped before the subjects are read. The release CLI puts the file into its
canonical form (line endings, Unicode normalization, one final line feed) and
refuses an empty or too large one, so this script does not.

WHAT IT READS: subjects on stdin and its arguments. No git, no network, so its
own test can feed subjects and judge the answer.

Usage: git log --no-merges --format=%s <base_revision>..<revision> |
         python3 scripts/release_changelog.py <version> <owner/repo> <revision> <base_version> <base_revision>
       python3 scripts/release_changelog.py <version> <owner/repo> <revision>   # the first release

A refused call exits 1 and prints nothing on stdout: the workflow writes stdout
to the file it uploads.
"""

import sys

from release_notes import Refused, notes

# The room for the notes. The dist service takes at most 1 MiB, and a character
# takes at most four bytes.
ROOM = 250000

# Whitespace as POSIX [[:space:]] reads it: Python's str.isspace also takes the
# no-break spaces and the C0 separators, which are text in a subject.
NOT_SPACE = set("\x1c\x1d\x1e\x1f\x85\xa0\u2007\u202f")


def blank(text):
    """Whether text holds nothing but whitespace."""
    return all(c.isspace() and c not in NOT_SPACE for c in text)


def changelog(args, stdin):
    """The changelog for args, reading stdin only when a base is given."""
    if len(args) not in (3, 5):
        raise Refused("pass the version, owner/repo and revision, and for a base its version and revision; subjects on stdin.")
    version, repo, revision = args[:3]
    if not version or not repo or not revision:
        raise Refused("the version, owner/repo and revision must not be empty.")
    if len(args) == 3:
        return "The first release on this line: the patch is the whole tree."
    base_version, base_revision = args[3:]
    if not base_version or not base_revision:
        raise Refused("the base version and base revision must not be empty.")
    subjects = stdin.read().replace(b"\0", b"").decode("utf-8", "ignore")
    if blank(subjects):
        raise Refused(f"no commit subject since the patch base {base_version} ({base_revision}); the patch would be empty.")
    text = notes(subjects.split("\n"), base_revision, revision, repo, ROOM, base_version)
    # Only the compare line: nothing a user notices, so no section to head.
    return ("## Changes\n\n" + text) if text.startswith("### ") else text


def main(argv):
    try:
        text = changelog(argv[1:], sys.stdin.buffer)
    except Refused as refused:
        print(f"release-changelog: {refused}", file=sys.stderr)
        return 1
    sys.stdout.buffer.write((text + "\n").encode("utf-8"))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
