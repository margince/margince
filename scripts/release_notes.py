#!/usr/bin/env python3
"""Turn the squash-merge subjects since the last release into release notes.

Subjects follow conventional commits: `feat(scope): …`, `fix(scope): …`. A `!`
before the colon is breaking. Only what a user or operator can notice is
listed: breaking, feat, fix and perf. refactor, test, docs, ci, chore and the
dependency updates without a prefix are left to the compare view.

A release body holds at most 125,000 characters, and two weeks of merges here
list about 400 subjects. The caller passes the room its own text leaves. Over
it, fixes and speed-ups, then features, collapse into a count. Breaking changes
are never cut, since an operator reads them before an upgrade, so breaking
changes too long for the room refuse.

WHAT IT READS: subjects on stdin and its arguments. No git, no network, so its
own test can feed subjects and judge the answer. Standard library only, so a
workflow that checks out this one file can run it.

The compare link needs git refs. A reader may know the previous release by
another name, a dist version such as 2026.1.0 for a git revision, so an
optional fifth argument names it in the sentence; it defaults to the ref.

Usage: git log --first-parent --format=%s v1.0.0..v1.1.0 |
         python3 scripts/release_notes.py v1.0.0 v1.1.0 margince/margince [budget [previous-name]]
"""

import re
import sys

SUBJECT = re.compile(r"([a-z]+)(\(([^)]*)\))?(!)?: (.+)")


class Refused(Exception):
    """The notes cannot be written; the message says why and what to do."""


def notes(subjects, previous, tag, repo, budget=100000, previous_name=""):
    """The notes for these subjects, or Refused when the shortest form does not fit."""
    breaking, added, fixed, faster = [], [], [], []
    for subject in subjects:
        match = SUBJECT.fullmatch(subject)
        if not match:
            continue
        kind, scope, bang, text = match.group(1), match.group(3), match.group(4), match.group(5)
        line = "- " + (f"**{scope}:** " if scope else "") + text
        if bang:
            breaking.append(line)
        elif kind == "feat":
            added.append(line)
        elif kind == "fix":
            fixed.append(line)
        elif kind == "perf":
            faster.append(line)

    compare = f"https://github.com/{repo}/compare/{previous}...{tag}"
    sentence = f"Every change since {previous_name or previous}: {compare}"

    def render(sections):
        body = "".join(f"### {heading}\n\n" + "".join(l + "\n" for l in lines) + "\n"
                       for heading, lines in sections if lines)
        return body + sentence

    def collapsed(lines):
        return [f"- {len(lines)} more, listed in the compare view."] if lines else []

    for sections in (
        [("Breaking", breaking), ("Added", added), ("Fixed", fixed), ("Faster", faster)],
        [("Breaking", breaking), ("Added", added), ("Fixed", collapsed(fixed)), ("Faster", collapsed(faster))],
        [("Breaking", breaking), ("Added", collapsed(added)), ("Fixed", collapsed(fixed)), ("Faster", collapsed(faster))],
    ):
        text = render(sections)
        if len(text) <= budget:
            return text
    raise Refused(
        f"the shortest notes, every section but Breaking counted, need {len(text)} characters; the room is {budget}.\n"
        "  Shorten the draft's own text, or the breaking pull request titles."
    )


def read_subjects():
    """Every line on stdin, the last one with or without its line feed. A byte
    that is not UTF-8 passes through unchanged, as git wrote it."""
    return sys.stdin.buffer.read().decode("utf-8", "surrogateescape").split("\n")


def main(argv):
    previous, tag, repo = (argv + ["", "", ""])[1:4]
    if not previous or not tag or not repo:
        print("release-notes: pass the previous tag, this tag and owner/repo; subjects on stdin.", file=sys.stderr)
        return 1
    try:
        budget = int(argv[4]) if len(argv) > 4 and argv[4] else 100000
    except ValueError:
        print(f"release-notes: the budget {argv[4]!r} is not a whole number of characters.", file=sys.stderr)
        return 1
    previous_name = argv[5] if len(argv) > 5 else ""
    try:
        text = notes(read_subjects(), previous, tag, repo, budget, previous_name)
    except Refused as refused:
        print(f"release-notes: {refused}", file=sys.stderr)
        return 1
    sys.stdout.buffer.write((text + "\n").encode("utf-8", "surrogateescape"))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
