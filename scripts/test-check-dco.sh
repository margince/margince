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
	if [[ -n "$must_say" ]] && ! printf '%s' "$out" | grep -qF -- "$must_say"; then
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

if [[ "$failures" -ne 0 ]]; then
	echo "" >&2
	echo "$failures DCO gate case(s) failed" >&2
	exit 1
fi

echo "DCO gate: every case holds"
