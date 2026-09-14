#!/usr/bin/env bash
# Does this pull request say which issue it closes, or that it closes none?
#
# THE DEFECT IS NOT A MISSING REFERENCE, it is a reference nothing reads. A pull
# request body said in as many words "Closes the residual half of #548" and its
# metadata referenced nothing — GitHub parses a closing keyword only when the
# issue number follows it directly, so the prose closed nothing. The issue stayed
# open six days after its fix merged and was then re-queued as new work. The
# author did write it down; they wrote it somewhere nothing reads.
#
# So this reads the METADATA — GitHub's own closingIssuesReferences, the same
# list that actually closes an issue on merge — and never the prose. A check that
# grepped the body would have passed that pull request and reported the defect as
# absent.
#
# THE "CLOSES NONE" HALF IS WHAT MAKES IT ENFORCEABLE. Most pull requests close no
# issue and are right not to; without a way to say so, this would either nag them
# for ever or have to guess which ones meant it.
#
# IT IS NOT A GATE and will never be one on its own evidence. The mechanism works
# — the finding that seemed to show four pull requests ignoring it turned out to
# be false when its author checked, and the trailer had done exactly its job. What
# is patchy is the habit, and a hard block on a mechanism that mostly works buys
# friction on every pull request to fix one. Promote it if the warning is
# measurably ignored; that is a decision with evidence behind it rather than
# ahead of it.
#
# WHAT IT DELIBERATELY DOES NOT DO: decide whether a pull request fixes an issue
# it did not mention. That is the actual defect and it is not machine-decidable.
# The declaration is the affordance that makes a human answer it.
#
# Reads its evidence from the environment rather than fetching it, so every arm
# is drivable from a fixture — the same shape check-review-coverage.sh uses.
#
#   CLOSING_DECL_REFS  newline-separated issue numbers GitHub records this pull
#                      request as closing; empty when it records none
#   CLOSING_DECL_BODY  the pull request body, for the closes-none declaration
#
# Exit 0 when the pull request has declared one way or the other, 1 when it has
# not. The caller decides what a 1 costs; in this repository it costs a comment.
set -euo pipefail

# The token, matched at the start of a line so a sentence mentioning it in
# passing does not count as a declaration. Case-insensitive, because an author
# typing it from memory should not be refused over a capital.
readonly NONE_PATTERN='^[[:space:]]*closes:[[:space:]]*none[[:space:]]*$'

refs="${CLOSING_DECL_REFS-}"
body="${CLOSING_DECL_BODY-}"

if [ -n "${refs//[[:space:]]/}" ]; then
	echo "declared: closes $(printf '%s' "$refs" | tr '\n' ' ' | sed 's/ *$//')"
	exit 0
fi

# A here-string rather than a pipe: `grep -q` exits on its first match and the
# producer then fails with EPIPE, which pipefail reports as a failed pipeline —
# so a match would read as no-match, on some machines and not others.
if grep -qiE "$NONE_PATTERN" <<<"$body"; then
	echo "declared: closes no issue"
	exit 0
fi

cat >&2 <<'MSG'
This pull request declares neither an issue it closes nor that it closes none.

Add ONE of these to the body:

    Closes #1234
    Closes: none

The first must be exactly that shape. "Closes the residual half of #548" reads
fine and closes nothing: GitHub parses a closing keyword only when the issue
number follows it directly, so a sentence like that leaves the issue open after
the fix merges — which happened, and the issue was re-queued as new work six days
later.

This is a warning. Nothing is blocked.
MSG
exit 1
