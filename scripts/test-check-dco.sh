#!/usr/bin/env bash
# test-check-dco.sh — prove the DCO gate still catches, and that an attestation
# excuses exactly one commit.
#
# The gate can only fail by being too permissive, and a permissive DCO gate
# reports the same "all commits are signed off" as a strict one. Remediation is
# where that risk actually lives: the whole point of the trailer is to let a
# commit stop failing, so a sloppy match turns the gate into a rubber stamp —
# a prefix match excuses a second commit sharing the first characters, an
# unanchored one excuses whatever a body happens to quote, and either reads as a
# clean history.
#
# Every case runs against a throwaway repository built here, because the gate's
# subject is COMMIT MESSAGES and this repository's own history is not a fixture
# anybody may edit. `git commit -s` is never used: the trailers are written out,
# so a case proves what it says rather than what the local git config supplies.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gate="$root/scripts/check-dco.sh"
failures=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

repo="$work/repo"
git init -q "$repo"
git -C "$repo" config user.name "Test Author"
git -C "$repo" config user.email "test@example.com"
git -C "$repo" config commit.gpgsign false

# commit_with <message> — one commit whose message is exactly what is passed,
# and whose sha is echoed for the caller to attest against.
commit_with() {
	local message="$1" n
	n="$(git -C "$repo" rev-list --count HEAD 2>/dev/null || echo 0)"
	echo "$n" >"$repo/file-$n"
	git -C "$repo" add -A
	git -C "$repo" commit -q --no-verify -m "$message"
	git -C "$repo" rev-parse HEAD
}

# case_is <name> <base> <head> <expected-exit> [substring the output must carry]
#
# A refusal case asserts the REFUSAL SENTENCE, never the bare sha: the line that
# ACCEPTS an attestation names that same sha, so a sha-only assertion is
# satisfied by either verdict. One case proved it — with the attester's own
# sign-off requirement deleted, it stayed green on an exit code it was owed for
# an unrelated reason.
case_is() {
	local name="$1" base="$2" head="$3" want="$4" must_say="${5:-}"
	local out status=0
	out="$(cd "$repo" && "$gate" "$base" "$head" 2>&1)" || status=$?
	if [[ "$status" -ne "$want" ]]; then
		echo "FAIL: $name — expected exit $want, got $status" >&2
		printf '%s\n' "$out" >&2
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$must_say" ]] && ! grep -qF -- "$must_say" <<<"$out"; then
		echo "FAIL: $name — exited $want but never named '$must_say'" >&2
		printf '%s\n' "$out" >&2
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

signed="Signed-off-by: Test Author <test@example.com>"
attest="DCO-Remediation-Commit: I, Test Author <test@example.com>, hereby add my Signed-off-by to commit:"

base="$(commit_with "root

$signed")"

# The happy path first, so a suite that somehow signs everything by accident is
# not the thing proving the failure cases.
signed_head="$(commit_with "a signed change

$signed")"
case_is "a signed commit passes" "$base" "$signed_head" 0 "are signed off"

unsigned="$(commit_with "an unsigned change")"
case_is "an unsigned commit fails, and is named" "$base" "$unsigned" 1 "$unsigned"

# The attestation arrives LATER than the commit it covers, which is the only
# order remediation can happen in — and therefore the order the gate has to
# read the range in.
attested_head="$(commit_with "attest the one above

$attest $unsigned

$signed")"
case_is "an attested commit passes" "$base" "$attested_head" 0 "attested by its author"

# Everything below is a way the match could be too generous. Each was written as
# the naive version first and rejected here.
other="$(commit_with "another unsigned change")"
case_is "an attestation covers only the commit it names" "$base" "$other" 1 "$other"

abbreviated="$(commit_with "attest by abbreviation

${attest} ${other:0:12}

$signed")"
case_is "an abbreviated sha attests nothing" "$base" "$abbreviated" 1 "$other"

# The line ENDS at the sha, so the trailing anchor cannot be what rejects this
# and the leading one is the only thing left holding it. Written the other way
# first — with prose after the sha — the case passed against a gate whose `^`
# had been deleted, which is a case asserting nothing.
quoted="$(commit_with "a body that merely quotes the words

In review somebody asked for: $attest $other

$signed")"
case_is "an attestation must start its own line" "$base" "$quoted" 1 "$other"

# Author-only, which is the whole worth of the certification: if anybody can
# attest to anybody's commit, the trailer certifies that somebody typed a sha.
# Each case below is one way the attestation could be somebody else's.
#
# Every one runs on a branch of its own, off a base of its own, holding exactly
# the target and the attestation under test. Sharing a mainline is what made the
# first draft of these cases meaningless: an unsigned commit left behind by an
# earlier case failed every range after it, so a case could report the verdict
# it expected while proving nothing about the property in its own name.
stranger_name="Other Person"
stranger_mail="other@example.com"
stranger_attest="DCO-Remediation-Commit: I, $stranger_name <$stranger_mail>, hereby add my Signed-off-by to commit:"

# case_branch <name> — a fresh branch at $base, and the unsigned commit each of
# these cases attests to (or fails to). Echoes that commit's sha.
case_branch() {
	git -C "$repo" checkout -q -b "$1" "$base"
	commit_with "an unsigned change to attest to"
}

target="$(case_branch stranger)"
git -C "$repo" -c user.name="$stranger_name" -c user.email="$stranger_mail" \
	commit -q --no-verify --allow-empty -m "a stranger attests to it

$stranger_attest $target

Signed-off-by: $stranger_name <$stranger_mail>"
case_is "a stranger cannot attest to somebody else's commit" "$base" "$(git -C "$repo" rev-parse HEAD)" 1 "commit $target missing"

# The claimed identity and the commit carrying it must be the same person, or
# the sentence and the signature disagree about who is speaking — and the
# sentence is the half a human reads.
target="$(case_branch misclaimed)"
head_sha="$(commit_with "claim to be somebody else

$stranger_attest $target

$signed")"
case_is "a claimed identity must match the attesting author" "$base" "$head_sha" 1 "commit $target missing"

# An unsigned commit cannot lend a signature it does not have, so a run of
# unsigned commits cannot certify itself.
target="$(case_branch unsigned-attest)"
head_sha="$(commit_with "attest without signing off

$attest $target")"
case_is "an unsigned commit cannot carry an attestation" "$base" "$head_sha" 1 "commit $target missing"

# Somebody can only attest to what they already wrote. Two branches off one
# base, merged: the attestation is in range and is the target's author's own,
# and it still does not descend from the commit it names.
target="$(case_branch aside-target)"
git -C "$repo" checkout -q -b aside-attest "$base"
commit_with "attest from a branch the target is not on

$attest $target

$signed" >/dev/null
git -C "$repo" merge -q --no-ff --no-verify -m "Merge the attestation branch" aside-target
case_is "an attestation must descend from what it attests to" "$base" "$(git -C "$repo" rev-parse HEAD)" 1 "commit $target missing"

# A merge commit needs no sign-off, but one that HAS a sign-off is a commit of
# its author's like any other — and the guidance promises an attestation may
# live in any later signed-off commit of theirs. The collection pass therefore
# reads merges even though the judging pass skips them; before that split this
# case was red.
target="$(case_branch merge-attester)"
git -C "$repo" checkout -q -b merge-attester-side "$target"
commit_with "a signed change to merge back

$signed" >/dev/null
git -C "$repo" checkout -q merge-attester
git -C "$repo" merge -q --no-ff --no-verify -m "Merge, and attest while doing it

$attest $target

$signed" merge-attester-side
case_is "a signed merge commit can attest" "$base" "$(git -C "$repo" rev-parse HEAD)" 0 "attested by its author"

# The same commit, attested properly by its own author, passes — so the four
# refusals above are about identity and order, not about the gate having quietly
# stopped accepting attestations at all.
target="$(case_branch proper)"
head_sha="$(commit_with "the author attests to their own commit

$attest $target

$signed")"
case_is "the author's own attestation is accepted" "$base" "$head_sha" 0 "attested by its author"

# A merge commit carries no sign-off and never can — GitHub synthesizes one for
# every pull_request run — so the gate skips merges. Proven rather than assumed:
# without --no-merges this case is red.
#
# It runs on a branch of its own, off a base of its own. Sharing the mainline
# would put the unsigned commits above in range, and the case would then be red
# for a reason that has nothing to do with merges — which is how it was first
# written, and it looked exactly like a gate that rejects merge commits.
git -C "$repo" checkout -q -b merge-case "$base"
merge_base="$(commit_with "the merge case starts here

$signed")"
git -C "$repo" checkout -q -b merge-side "$merge_base"
commit_with "a signed change on a side branch

$signed" >/dev/null
git -C "$repo" checkout -q merge-case
git -C "$repo" merge -q --no-ff --no-verify -m "Merge the side branch" merge-side
merged="$(git -C "$repo" rev-parse HEAD)"
case_is "a merge commit needs no sign-off" "$merge_base" "$merged" 0 "are signed off"

# THE RANGE ITSELF. Every case above hands the gate a range with something in
# it, so none of them can tell "all these commits are signed" from "there were
# no commits". That distinction is the one a gate must never lose: it reads a
# smaller history, prints the same clean pass, and nothing fails to say so.
#
# Each of these was a green run before the range was checked.

case_is "an empty range is refused, not passed" "$merged" "$merged" 1 "holds no commits"

# A base that RESOLVES and is not behind the head is the dangerous shape, and it
# is the one `git rev-list` answers silently: not an error, just nothing. The
# advisory main-health lane asserted this and the required ci.yml job did not,
# so the protection lived where it could not block a merge.
git -C "$repo" checkout -q -b sideways "$base"
sideways="$(commit_with "a commit on a branch of its own

$signed")"
case_is "a base that is not an ancestor is refused" "$sideways" "$merged" 1 "not an ancestor"

# And the success line carries its own DENOMINATOR, which is what makes a run
# that read nothing unable to look like one that read everything.
git -C "$repo" checkout -q merge-case
case_is "the pass says how many commits it read" "$merge_base" "$merged" 0 "all 1 of 2 commit(s)"
case_is "and how many of those needed no trailer" "$merge_base" "$merged" 0 "1 merge commit(s) need none"

if [[ "$failures" -ne 0 ]]; then
	echo "" >&2
	echo "$failures DCO gate case(s) failed" >&2
	exit 1
fi

echo "DCO gate: every case holds"
