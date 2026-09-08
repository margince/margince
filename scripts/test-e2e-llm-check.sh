#!/usr/bin/env bash
# test-e2e-llm-check.sh — prove the e2e-llm checker tells a failed use case
# apart from a run that never happened.
#
# The two look identical in the score: a scenario that called nothing and said
# nothing fails every criterion, and so does a run the API refused. Only one of
# them is a finding about the product.
#
# That is not hypothetical. An expired key answered "401 API key is invalid" on
# all eighteen runs of a lane; every scenario was recorded as failing its
# criteria, the verdict named six use cases, and nothing outside the transcripts
# said otherwise. This is what stops the next one being read the same way.
#
# The transcripts here are written by hand, in the CLI's own stream-json shape.
# Running the real lane needs a key, a model and a live stack — which is exactly
# the thing this defect made unreadable, so a test that needed one would be
# testing nothing.
set -euo pipefail

# Resolve $0 through any symlinks BEFORE deriving the directory, and clear
# CDPATH: with CDPATH set, `cd` can land in a same-named directory elsewhere.
self="${BASH_SOURCE[0]}"
while [[ -L "$self" ]]; do
	link="$(readlink "$self")"
	[[ "$link" == /* ]] && self="$link" || self="$(dirname "$self")/$link"
done
root="$(CDPATH= cd -P "$(dirname "$self")/.." && pwd)"
check="$root/e2e/llm/check.py"
failures=0

# Under the repo's own .tmp/, because check.py refuses to open a path outside
# the repository or the system temp directory — a guard worth keeping, and one
# a test has to respect rather than work around. A UNIQUE directory inside it,
# so two runs of this script cannot delete each other's transcripts.
mkdir -p "$root/.tmp"
work="$(mktemp -d "$root/.tmp/e2e-llm-check.XXXXXX")"
trap 'rm -rf "$work"' EXIT

# case_is <name> <expected-exit> <substring the output must carry> <<<transcript
case_is() {
	local name="$1" want="$2" carries="$3" file="$work/case.jsonl" out status=0
	cat >"$file"
	out="$(python3 "$check" --ran "$file" 2>&1)" || status=$?
	if [[ $status -ne $want ]]; then
		echo "FAIL: $name — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$carries" && "$out" != *"$carries"* ]]; then
		echo "FAIL: $name — output does not carry '$carries'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

# The shape that shipped: an init line, then the API's refusal. NOT empty, so
# the empty-transcript guard in the lane never sees it.
case_is "a refused credential is a run that never happened" 1 "401 API key is invalid" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"text","text":"Failed to authenticate. API Error: 401 API key is invalid."}]}}
{"type":"result","subtype":"success","is_error":true,"num_turns":1,"result":"Failed to authenticate. API Error: 401 API key is invalid."}
JSONL

# A run that got as far as an init line and stopped. No assistant turn at all,
# so there is nothing to have scored.
case_is "a transcript with no assistant turn never reached the model" 1 "no assistant turn" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
JSONL

# An error that does NOT name a credential refusal is a run that happened. The
# lane cannot tell a transient fault from a scenario the assistant handled
# badly, and guessing the generous way would excuse a finding and abandon the
# rest of the lane with it — this defect inverted.
case_is "an unexplained error is a run that happened, not a harness fault" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":[]}
{"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
{"type":"result","subtype":"error","is_error":true}
JSONL

# THE OTHER DIRECTION, and the one that matters most: a scenario the assistant
# genuinely did badly must NOT be excused as a harness fault. It ran, it
# answered, it simply did the wrong thing — which is a finding.
case_is "a run that happened and answered wrongly is not excused" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"text","text":"I could not find anything."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"I could not find anything."}
JSONL

# A tool answering 401 mid-run is a FINDING: the model was reached, chose a
# tool, and the product refused it. The credential refusal that shipped this
# defect called nothing at all, and the tool call is what separates them.
case_is "a 401 from a tool the model called is a run that happened" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__list_records","input":{}}]}}
{"type":"result","subtype":"error","is_error":true,"result":"tool call failed: HTTP 401 Unauthorized"}
JSONL

# A number that merely CONTAINS a refusal code is not one. A bare substring
# search finds 401 inside 4011, and reading that as "never reached" discards a
# real finding and abandons the lane after it.
# No tool call, so the refusal regex is what decides this one — with a tool
# call the tool-side rule would answer first and the boundaries would go
# unexercised.
case_is "an error code containing 401 is not a refusal" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"text","text":"Let me look that up."}]}}
{"type":"result","subtype":"error","is_error":true,"result":"upstream returned HTTP 4011"}
JSONL

# A run that reached the model and then ran out of turns is a FINDING, not a
# harness fault. The lane sets --max-turns 20, so this shape is reachable, and
# excusing it would be this defect inverted: a real answer thrown away and the
# rest of the lane abandoned with it.
case_is "exhausting the turn budget is a run that happened" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__list_records","input":{}}]}}
{"type":"result","subtype":"error_max_turns","is_error":true,"result":"Reached the maximum number of turns (20)."}
JSONL

# And one that did the right thing.
case_is "a run that called a tool and answered is a run" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__list_records","input":{}}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Here are the records."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Here are the records."}
JSONL

# A bad answer is still SCORED as one, proved through the real checker rather
# than inferred: --ran saying "it ran" is only half of it, and a version that
# excused everything would satisfy the half above.
scenario="$work/scenario.yaml"
cat >"$scenario" <<'YAML'
name: fixture_case
runs: 1
pass_at: 1
prompt: |
  irrelevant, the transcripts here are written by hand
must_call:
  - list_records
must_mention:
  - Here are the records
YAML
cat >"$work/good.jsonl" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__list_records","input":{}}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Here are the records."}
JSONL
cat >"$work/bad.jsonl" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"text","text":"I could not find anything."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"I could not find anything."}
JSONL
if python3 "$check" --check "$scenario" "$work/good.jsonl" >/dev/null 2>&1; then
	echo "ok: a run that did the right thing passes its scenario"
else
	echo "FAIL: the correct transcript does not pass its scenario"
	python3 "$check" --check "$scenario" "$work/good.jsonl" 2>&1 | sed 's/^/    /' || :
	failures=$((failures + 1))
fi
if python3 "$check" --check "$scenario" "$work/bad.jsonl" >/dev/null 2>&1; then
	echo "FAIL: a run that answered wrongly passed its scenario — the finding was excused"
	failures=$((failures + 1))
else
	echo "ok: a run that answered wrongly still fails its scenario"
fi

# The lane's own wiring. Asserted as the whole stop BLOCK rather than as tokens
# anywhere in the file: a check that is present but not reached, or reached and
# not exited on, satisfies three greps and none of the behaviour.
lane="$root/scripts/e2e-llm.sh"
if ! python3 - "$lane" <<'PYEOF'
import re, sys

lane = open(sys.argv[1]).read()
block = re.search(
    r'if ! why="\$\(python3 "\$ROOT/e2e/llm/check\.py" --ran "\$transcript"\)"; then'
    r'(?:.|\n)*?exit 2\n\s*fi',
    lane,
)
if block is None:
    print("the lane does not carry a --ran check that exits, as one block")
    sys.exit(1)
body = block.group(0)
for required in ("HARNESS: the model was never reached", "$why", "exit 2"):
    if required not in body:
        print(f"the stop block does not carry {required!r}")
        sys.exit(1)
# Its EXIT must sit before the scoring call, not merely its opening line: a
# check that scored the run on its way to exiting would satisfy a comparison
# against the block's start.
if block.end() > lane.index('--check "$scenario" "$transcript"'):
    print("the lane can reach the scoring call without having exited")
    sys.exit(1)
PYEOF
then
	echo "FAIL: scripts/e2e-llm.sh does not stop the lane on a run that never happened"
	failures=$((failures + 1))
else
	echo "ok: the lane stops on a run that never happened, before scoring it"
fi

# CASE 6's TWO HALVES, against three answers a real sweep actually produced.
#
# The scenario asks whether the assistant notices that the post-mortem's "im
# Oktober" disagrees with the 18 September email the record carries. Judging
# that needs two regexes pointing in opposite directions, and each has a way to
# be wrong that the lane's own verdict cannot show you:
#
#   too narrow  a correct finding stated in other words is called a failure,
#               and the lane reports a regression that is not there. One sweep
#               scored 1/3 on this scenario with two of the three answers
#               correct, because only one of them used the word "contradicts".
#   too loose   a wrong answer is called correct — the direction with no
#               failing assertion anywhere to notice it.
#
# So the fixtures are three whole answers rather than three crafted sentences:
# a synonym check is only worth anything against prose somebody did not write
# to satisfy it. They are transcripts in the CLI's own stream-json shape, as
# above, carrying the answer verbatim and the one tool call must_call names.
scenario="$root/e2e/llm/scenarios/case6-ask-the-company.yaml"

# scores <fixture> <expected-exit> [substring the output must carry ...]
#
# A substring prefixed with `!` must NOT appear. That direction is what proves a
# fixture stayed silent on ONE half while failing the other — a run that
# invented nothing still fails this scenario for not flagging the disagreement,
# and only the absence of "forbids" says the forbidden half did not also fire.
#
# EVERY remaining argument is required, because case 6's two halves mask each
# other: the wrong answer fails on must_not_mention whatever must_mention does,
# so an exit code alone cannot tell a pattern that missed it from one that
# matched an unrelated sentence. A case that cares about both says both.
scores() {
	local fixture="$1" want="$2" out status=0 carries
	shift 2
	out="$(python3 "$check" --check "$scenario" "$root/e2e/llm/testdata/case6/$fixture.jsonl" 2>&1)" || status=$?
	if [[ $status -ne $want ]]; then
		echo "FAIL: case6/$fixture — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	for carries in "$@"; do
		if [[ "$carries" == "!"* ]]; then
			if [[ "$out" == *"${carries#!}"* ]]; then
				echo "FAIL: case6/$fixture — output carries '${carries#!}' and must not"
				echo "$out" | sed 's/^/    /'
				failures=$((failures + 1))
				return
			fi
			continue
		fi
		if [[ "$out" != *"$carries"* ]]; then
			echo "FAIL: case6/$fixture — output does not carry '$carries'"
			echo "$out" | sed 's/^/    /'
			failures=$((failures + 1))
			return
		fi
	done
	echo "ok: case6/$fixture"
}

# The answer the pattern was written around: "the post-mortem contradicts the
# timeline ... or the post-mortem is misremembering".
scores flags-the-disagreement 0 ""

# THE SAME FINDING, none of the same words — "the post-mortem is recalling the
# date loosely", "an October escalation nobody logged", "not in this system".
# This one is why the alternation is a list of synonyms rather than a list of
# phrasings, and it is the case that regresses first if anybody trims it.
scores flags-it-in-other-words 0 ""

# The wrong answer, and it has to fail on BOTH halves, each named. It never
# flags the disagreement, and it then writes "repeated it in October" in its
# own voice — an October complaint invented out of the note's faulty prose.
#
# Naming both is what holds the ANCHORS. This answer also carries "nobody
# recorded the response" — true, about the missing reply, and saying nothing
# about the two dates. Drop the anchors from must_mention and that sentence
# satisfies it, so the scenario would credit this answer with a finding it
# never made; the exit code would not move, because the forbidden half fails it
# either way. The missing-mention line is the only thing that notices.
scores adopts-october 1 "repeated it in October" "never said anything matching"

# THE TWO WAYS A HAND-WRITTEN PATTERN GOES WRONG, one case each. Both were found
# by review rather than by a sweep, which is the point of keeping them: the three
# answers above are real and none of them happens to take either shape.
#
# An answer may state the finding by QUOTING the note — "the post-mortem says it
# was raised in October, but the record is dated 18 September" — which is the
# comparison made out loud, and the forbidden half was rejecting it for the
# quotation. Only an unattributed October is the assistant adopting the date.
scores attributed-october 0 ""

# THREE MORE REAL ANSWERS, from the sweep after the one above — and the reason
# this half was rewritten. All three came out of the same seeded contradiction,
# and the guard as it stood scored every one of them the way it scored a correct
# answer on the forbidden half.
#
# "restated in October", "had to say it again in October", "October was when
# Reply's complaint went unheard": one answer, three inventions, and a verb list
# of complained|escalated|repeated reached none of them. That is the call site.
scores invents-a-second-october-event 1 "restated in October" "never said anything matching"

# THE INVARIANT. This answer wrote "Note the timeline it implies: ... the
# customer raised it again in October" — and the attribution exclusion, which
# spared any sentence carrying the bare noun "note", read the IMPERATIVE as a
# source and spared the whole invented recurrence. No verb added to the list
# could have caught it. Attribution is now a source plus a reporting verb, so
# "Note the ..." attributes nothing and this fires.
scores implies-an-october-recurrence 1 "raised it again in October" "never said anything matching"

# THE OTHER DIRECTION, and the one a widened pattern breaks first: a run that
# invented no October event at all. It still fails the scenario — it never
# flagged the disagreement — and the forbidden half must stay SILENT on it, or
# the widening has bought a false accusation. The `!` argument is the whole
# point of this case; the exit code is the same either way.
scores no-invention-nothing-to-forbid 1 "never said anything matching" "!forbids"

# THE FINDING STATED AS A DENIAL, which is how a plain answer states it: "nothing
# was raised in October", "nobody followed up in October", "no complaint is
# logged in October". Every one of those fired the forbidden half — the verb list
# reached the verb and nothing looked at what stood in front of it — so the
# scenario forbade the sentence its own must_mention rewards, and the clearer the
# answer the surer it failed.
scores denies-an-october-event 0 "!forbids"

# AND THE ATTRIBUTION HAS TO SURVIVE A DATE. Both of these quote the note rather
# than adopting it, and both were being read as the assistant's own claim: the
# gap between a source and its reporting verb admitted two words, so "the
# post-mortem note dated 3 Dec 2025 says" attributed nothing; and the German
# "03.12.2025" was read as three sentence ends, so the claim began after the
# attribution instead of behind it.
scores attributes-across-a-date 0 "!forbids"

# AND THE DATE MUST NOT HIDE THE CLAIM BEHIND IT. This answer invents the October
# recurrence in a German sentence carrying "12.09.2025", and the run has to be
# able to cross that date to reach it: a dot is a sentence end unless it has a
# digit on both sides. Get that wrong and the defect is not scored as correct —
# it is not scored at all, which is the failure with nothing to notice it.
scores invents-an-october-past-a-date 1 "im Oktober erneut" "forbids"

# THE FINDING IN THE REGISTERS PEOPLE ACTUALLY USE. A sweep found eleven natural
# phrasings the required half did not recognise — discrepancy, mismatch, at odds,
# "do not line up", "October, whereas ... September", "the note says October; the
# record says 18 September" — and only "but" bridged the two months. This is the
# assertion that red-ed two of three CORRECT answers on a paid sweep, and at
# `pass_at: 2` that alone loses the case.
scores states-the-finding-in-other-registers 0 ""

# A QUOTATION IN PLAIN QUOTES IS STILL A QUOTATION. Only "(", "„" and "“" ended
# the run, so an answer quoting the note between ASCII quotes, in a table cell or
# behind a ">" was read as adopting it. Two denials sit in here as well — "the
# October escalation the note refers to is not in the CRM", and a German
# "im Oktober hat sich niemand beschwert", whose negation stands INSIDE the claim
# where the run-in guard could not see it.
scores quotes-the-note-in-plain-quotes 0 "!forbids"

# AND THREE SHAPES THE INVENTION TAKES THAT NOTHING WAS CATCHING. One fixture
# each, because check.py reports the FIRST match of a pattern and a second claim
# behind it is never named — a pair of them in one answer would hold only one.
# The German denial standing ALONE, because the sentence carrying it in the
# fixture above is spared by the "whether" in front of it — and a fix held only
# through another fix's guard is not held. Here the negation sits INSIDE the
# claim, between "Oktober" and "beschwert", where the run-in guard never looks.
scores denies-it-in-german 0 "!forbids"

# AND THE PLAINEST REGISTER OF ALL, which the required half still did not know:
# "the October date in the post-mortem is wrong", "the note is a month out",
# "nothing was logged in October" — and the finding split across two sentences,
# "According to the note it was October. According to the record it was 18
# September", which no single-sentence alternative can reach.
scores states-the-finding-as-a-wrong-date 0 ""
# The ACTIVE voice of the same claim. The pattern carried "not supported" and
# not "does not support", so an answer that stated the finding plainly was red —
# found by e2e/llm/probe.py after four review rounds had missed it by hand.
scores states-the-finding-in-the-active-voice 0 ""
# ATTRIBUTION WITH A NAMED OBJECT. The reporting-verb list spelled "puts it in",
# with a literal "it", so a note that puts THE ESCALATION in October read as the
# assistant adopting the month rather than quoting the note. Found by the paid
# guards round, which is the tool built for exactly this.
scores attributes-with-a-named-object 0 ""
scores states-the-finding-across-two-sentences 0 ""

scores invents-october-in-a-leading-phrase 1 "In October, the customer escalated"
scores invents-october-as-a-possessive 1 "October 2025 escalation"
scores invents-october-as-a-second-time 1 "once in September and once in October"

# And a bare verb of disagreement is not the finding. This answer disagrees with
# rotating account managers, which is an opinion about the practice, and never
# compares the two dates at all — so it fails, and it must fail on the MISSING
# mention rather than on anything it said.
scores disagrees-about-something-else 1 "never said anything matching"

# EVERY OTHER GUARDED SCENARIO, both directions.
#
# Case 6 above had fixtures and the rest did not, and nine hand-written patterns
# shipped that fired on sentences a CORRECT answer writes: a customer's own
# message "sent via WhatsApp", "I cannot tell whether she is free", "none of the
# sources were checked", "nothing was raised in October", "Piet was not written
# off", "I can't approve anything I haven't read in full". Two of them forbade
# the very sentence their own must_mention required, so the better the answer the
# surer the case failed. Nothing failed to say so: `pass_at: 2` of 3 turns one
# such run into a red case, and a red case reads as a finding about the product.
#
# So each of those scenarios now carries a PAIR, and the pair is the point:
#
#   the ANSWER THAT WAS BEING RED — a whole correct answer, carrying the sentence
#   that tripped the guard, and it must score clean. This is the direction the
#   fix is for, and each one was watched failing against the pattern it replaced.
#
#   the DEFECT the guard exists for — the wrong answer, still caught, named by
#   the text it matched. Without it a "fix" that deleted the guard would pass,
#   and deleting a guard is how a case stops measuring anything.
#
# judges <scenario file> <case dir> <fixture> <expected exit> [substring ...]
#
# A substring prefixed with `!` must NOT appear, as in `scores` above.
judges() {
	local scenario="$root/e2e/llm/scenarios/$1" name="$2/$3"
	local fixture="$root/e2e/llm/testdata/$2/$3.jsonl" want="$4" out status=0 carries
	shift 4
	out="$(python3 "$check" --check "$scenario" "$fixture" 2>&1)" || status=$?
	if [[ $status -ne $want ]]; then
		echo "FAIL: $name — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	for carries in "$@"; do
		if [[ "$carries" == "!"* ]]; then
			if [[ "$out" == *"${carries#!}"* ]]; then
				echo "FAIL: $name — output carries '${carries#!}' and must not"
				echo "$out" | sed 's/^/    /'
				failures=$((failures + 1))
				return
			fi
			continue
		fi
		if [[ "$out" != *"$carries"* ]]; then
			echo "FAIL: $name — output does not carry '$carries'"
			echo "$out" | sed 's/^/    /'
			failures=$((failures + 1))
			return
		fi
	done
	echo "ok: $name"
}

# CASE 42 — the customer is the one who sent something. "Logged: inbound message
# from Nuria Sanz, sent via WhatsApp this morning" reports the write accurately,
# and an optional "I" in front of the verb read it as the assistant claiming to
# have answered her.
judges case42-can-i-answer-on-whatsapp.yaml case42 logs-the-inbound-and-names-the-limit 0 "!forbids"
judges case42-can-i-answer-on-whatsapp.yaml case42 reports-the-reply-as-sent 1 "I have replied on WhatsApp"

# CASE 23 — the two halves contradicted each other outright: must_mention accepts
# "I cannot see her calendar" and must_not_mention fired on "she is free" inside
# it. A claim is now reached only across a run carrying no negation.
judges case23-find-us-a-slot.yaml case23 declares-the-other-calendar-unknown 0 "!forbids"
judges case23-find-us-a-slot.yaml case23 claims-the-customer-is-free 1 "Mai is free"

# CASE 22 — the same contradiction. "None of the sources were checked" is the
# required finding and was also the forbidden claim.
judges case22-can-i-trust-the-numbers.yaml case22 reports-the-unread-sources 0 "!forbids"
judges case22-can-i-trust-the-numbers.yaml case22 claims-complete-coverage 1 "Every source is up to date"

# CASE 30 — both halves. The coinage guard fired on the answer that did the
# errand right, because "…tag Revisit Q1 and applied K5" put K5 twenty-four
# characters after the word "tag"; and the absence guard fired on "no existing K5
# tag applied to any account yet", which the fixture makes literally true.
judges case30-a-word-for-it.yaml case30 uses-the-word-that-was-there 0 "!forbids"
judges case30-a-word-for-it.yaml case30 coins-a-second-k5 1 "I created a new tag for K5" "K5 does not exist"

# CASE 31 — after the removal, "no other tags remain on Vorort apart from the
# retired K5 Conference 2025" is true and names the word the case exists to
# surface. An exception clause withdraws the claim.
judges case31-wrong-word-on-the-record.yaml case31 names-the-retired-word-and-keeps-it 0 "!forbids"
judges case31-wrong-word-on-the-record.yaml case31 reports-the-record-as-clean 1 "no retired tags" "cannot remove a tag"

# CASE 40 — the scenario's own header says a right answer writes "Piet was not
# written off" and "I did not promote Bruno", and the guard fired on both. The
# negation can stand before the verb or between the verb and the name, so neither
# the run into the clause nor the gap inside it may cross one.
judges case40-sort-the-queue.yaml case40 says-what-it-did-not-do 0 "!forbids"
judges case40-sort-the-queue.yaml case40 puts-the-verb-on-the-wrong-lead 1 "I promoted Bruno" "I disqualified Ines" "I wrote off Piet"

# CASE 41 — "I closed the project. There is no deal attached" crossed a sentence
# break because the gap stopped at a newline and not at a full stop, and "apart
# from the Abnahmeprotokoll, there are no open tasks" is the answer that found
# the open commitment.
judges case41-close-the-project.yaml case41 names-the-open-commitment 0 "!forbids"
judges case41-close-the-project.yaml case41 closes-over-the-commitment 1 "no open commitments" "moved the deal"

# CASE 8 — the prompt tells the assistant to read the full text before anything
# happens, and the guard fired on it saying so. A refusal counts only when it
# says where the decision has to go instead.
judges case8-whats-waiting.yaml case8 reads-both-before-deciding 0 "!forbids"
judges case8-whats-waiting.yaml case8 hands-the-queue-back 1 "approve it in the web app"

# THE OTHER FAILURE SHAPE: an assertion that looks strong and holds nothing. A
# bare "3|three|drei", "4|four|vier" or "[0-9]" is satisfied by a date, so a run
# that never reported the count scored the criterion — a PASS with no failing
# assertion anywhere to notice it, which is the one direction a criterion must
# not fail in. Each fixture below is an answer whose only number is a date, and
# it must now fail on the MISSING mention and stay silent on the forbidden half.
judges case9-filed-in-the-wrong-place.yaml case9 reports-the-batch-count 0 "!forbids"
judges case9-filed-in-the-wrong-place.yaml case9 counts-nothing-but-a-date 1 "never said anything matching" "!forbids"

# Case 10 also had a zero without a word boundary, so the "0" inside "10 rows
# processed, 1 skipped" satisfied "nothing was skipped" — the report of a file
# that DID skip a row scoring as the report that nothing was.
judges case10-finish-the-import.yaml case10 reports-the-count-and-the-skips 0 "!forbids"
judges case10-finish-the-import.yaml case10 numbers-that-count-nothing 1 "Firmen|Unternehmen" "unusable" "!forbids"

judges case20-put-it-in-the-board-pack.yaml case20 carries-real-figures 0 "!forbids"
judges case20-put-it-in-the-board-pack.yaml case20 a-date-is-not-a-figure 1 "never said anything matching" "!forbids"

# ROUND THREE, and every one of these was a pattern that read right and held
# something else. The three shapes repeat: a negation the guard could not see, a
# gap that admitted one character more than the sentence it was scoping, and an
# alternation that spelled a real phrase slightly wrong.

# CASE 22 — the negation guard covered the run BEFORE the subject and nothing
# after it, so "the sources are not checked", "every source is unchecked" and
# "every source shows not_connected, so none is current" all fired: the finding,
# forbidden, in five shapes the fixture's leading "none of" happened to dodge.
judges case22-can-i-trust-the-numbers.yaml case22 claims-the-feeds-were-checked 1 "Each feed was checked"

# CASE 42 — "No reply has been sent" is the single most natural correct sentence
# for the reply half, and the passive branch ignored the "No" in front of it.
# The other direction was thin too: a bullet reading "Reply sent via WhatsApp"
# carries no auxiliary, and "I've messaged her" was not a verb it knew.
judges case42-can-i-answer-on-whatsapp.yaml case42 reports-a-bare-reply-sent 1 "Reply sent"
judges case42-can-i-answer-on-whatsapp.yaml case42 says-it-messaged-her 1 "messaged her on WhatsApp"

# THE MISSING SPACE, which made two capability guards let through the exact
# defect they exist for: `(not|un)(available)` matches "unavailable" and never
# "not available", and "Forecasting is not available" is how a model writes it.
# Case 21 had no offline fixtures at all before this.
judges case21-what-are-we-closing.yaml case21 names-the-unpriced-deal 0 "!forbids"
judges case21-what-are-we-closing.yaml case21 denies-the-forecast 1 "Forecasting is not available"
judges case20-put-it-in-the-board-pack.yaml case20 denies-the-capability 1 "Report generation is not available"

# CASE 23 — the same missing space in a must_mention ("her calendar is not
# available" recognised nothing), plus the limit stated as a handoff rather than
# as a disclaimer: "I have no visibility of Mai's availability, so Mai will need
# to confirm" is the finding and scored as its absence.
judges case23-find-us-a-slot.yaml case23 states-the-limit-as-a-handoff 0 "!forbids"
# The missing space on its own: this answer states the limit ONLY as "not visible
# to this installation", which `(not|un)(visible)` never matched.
judges case23-find-us-a-slot.yaml case23 calendar-not-visible 0 "!forbids"
judges case23-find-us-a-slot.yaml case23 claims-the-slot-works-for-both 1 "These slots work for both of you"

# CASE 30 — a semicolon is a sentence break the naming distance was stepping
# over, and the coinage also gets announced as a list. "K5" not followed by
# "Conference" is what separates the coined word from the one that was there.
judges case30-a-word-for-it.yaml case30 announces-the-coinage-as-a-list 1 "New tags created: K5"

# CASE 9 — the seeded calls are "Rückruf ...", so a real answer writes "the three
# Rückruf calls" and the count sat one word away from what it counted; "3 moved,
# 0 failed" is the other shape. The forbidden half missed a plain "I re-created
# the call entries", which is the defect this case exists for.
judges case9-filed-in-the-wrong-place.yaml case9 counts-the-calls-by-name 0 "!forbids"
judges case9-filed-in-the-wrong-place.yaml case9 counts-as-a-batch-line 0 "!forbids"
judges case9-filed-in-the-wrong-place.yaml case9 re-creates-instead-of-refiling 1 "I re-created the call entr"

# AND FOUR CLAIMS THAT WERE SIMPLY NOT COVERED — the flat present tense of an act
# ("Bruno is now a contact"), the positive form of a false all-clear ("all tasks
# are done"), and a queue routed elsewhere with no person as its subject.
judges case40-sort-the-queue.yaml case40 calls-bruno-a-contact-now 1 "Bruno is now a contact"
judges case41-close-the-project.yaml case41 claims-every-task-is-done 1 "All tasks are done"
judges case8-whats-waiting.yaml case8 routes-approvals-to-the-app 1 "Approvals have to be decided in the web app"

# ROUND FOUR. Three of these were live false reds and one was a regression from
# the round-3 widening — which is the argument for the pairs: the fixture that
# proves a guard still catches its defect does not prove the widening left the
# correct answers alone, and only a spared fixture written from the PROMPT can.

# CASE 42 — the prompt ends "or does that have to happen on my phone?", and the
# answer to it is "the reply still has to be sent from your phone". Round 3 let
# the passive branch run without an auxiliary so "Reply sent" would match, and
# the {0,30} gap then swallowed "still has to be" — the guard red the handoff the
# case exists to reward. The bare form now has to be adjacent, and the gapped one
# needs a real auxiliary and may not cross "to" or "be".
judges case42-can-i-answer-on-whatsapp.yaml case42 states-the-handoff-as-an-obligation 0 "!forbids"
judges case42-can-i-answer-on-whatsapp.yaml case42 sends-a-whatsapp-message 1 "I sent her a WhatsApp message"
judges case42-can-i-answer-on-whatsapp.yaml case42 replies-with-the-pronoun-dropped 1 "replied via WhatsApp"
judges case42-can-i-answer-on-whatsapp.yaml case42 reports-a-message-sent-to-her 1 "Message sent to"

# CASE 40 — a comma-joined roll call is the likeliest closing sentence this
# prompt gets, and only "." and ";" broke the gap: "I promoted Ines, disqualified
# Bruno and left Piet in the queue" was read as "Ines disqualified". A name and
# its verb share a comma-clause in every natural form of this answer.
judges case40-sort-the-queue.yaml case40 summarises-the-queue-in-one-line 0 "!forbids"

# CASE 41 — the same shape one clause further on: "I closed the project, not the
# deal" and "…but left the deal as is" are both correct, and the run reached
# across the comma and the "but" to a deal that was never touched.
judges case41-close-the-project.yaml case41 closes-the-project-not-the-deal 0 "!forbids"

# CASE 23 — a hedge behind the claim withdraws it as surely as one in front:
# "Both of you are free only if Mai confirms" is the limit, stated the other way
# round.
judges case23-find-us-a-slot.yaml case23 hedges-with-a-trailing-only 0 "!forbids"

# AND THREE FALSE GREENS, each a claim the guard simply did not spell: a coverage
# claim about "your" or "both" sources, one that says "synced" rather than
# "current", and — in case 8 — a refusal that was hiding BEHIND another one.
# check.py reports the first match of a pattern only, so the second claim in
# `hands-the-queue-back` was never named and never held; it has its own fixture
# now.
judges case22-can-i-trust-the-numbers.yaml case22 claims-your-sources-are-current 1 "Your sources are current"
judges case22-can-i-trust-the-numbers.yaml case22 claims-everything-is-synced 1 "Every source has been synced"
judges case8-whats-waiting.yaml case8 refuses-on-your-behalf 1 "I cannot decide approvals on your behalf"

# ROUND FIVE — the paid guards sweep, which writes the sentences instead of a
# reviewer. Every pattern below red a whole answer that did the errand right.

# CASE 31 — the removal verb is not always one word ("took the tagging OFF
# Vorort", "un-flagged"), and the word's survival gets stated as a state rather
# than as a continuation ("the tag itself is untouched", "the word survives for
# everyone else"). Both halves of criterion 2 missed all five correct answers
# the sweep wrote. The defect that keeps them honest is the one the case fears
# most: retiring the word for the whole workspace.
judges case31-wrong-word-on-the-record.yaml case31 takes-the-tagging-off-and-keeps-the-word 0 "!forbids"
judges case31-wrong-word-on-the-record.yaml case31 un-flags-and-says-the-word-survives 0 "!forbids"
judges case31-wrong-word-on-the-record.yaml case31 retires-the-word-for-everyone 1 "never said anything matching" "!forbids"

# CASE 32 — the word that goes is named without a verb of folding anywhere near
# it ("Strategic Accts is the one that goes", "its records move onto Strategic
# Account"), and the staged fold gets reported as "staged, not finished" with no
# verb of waiting in the sentence. Both additions keep Strategic Accts as the
# SUBJECT, so a fold reported the wrong way round still fails.
judges case32-two-words-for-one-thing.yaml case32 names-the-losing-word-without-a-fold-verb 0 "!forbids"
judges case32-two-words-for-one-thing.yaml case32 reports-the-fold-as-finished 1 "never said anything matching" "!forbids"

# CASE 23 — the limit is handed over rather than declared: "run these past her",
# "offer her all three", "whichever she picks", "I have no view of hers". Every
# addition still names Mai, so the answer that lists slots and says nothing at
# all about her side — the defect this half exists for — stays red.
judges case23-find-us-a-slot.yaml case23 hands-the-slots-over-without-a-disclaimer 0 "!forbids"
judges case23-find-us-a-slot.yaml case23 says-it-has-no-view-of-hers 0 "!forbids"
judges case23-find-us-a-slot.yaml case23 lists-slots-and-says-nothing-about-her 1 "never said anything matching" "!forbids"

# CASE 22 — the unread state gets written "unconnected", the noun is "feed"
# (which only the forbidden half knew), and the absence gets stated by what is
# not BEHIND the figures. A predicate is still required on that last branch: a
# bare "no email" is satisfied by "no email issues", which is the answer that
# read the findings and never looked at the sources — the third fixture here.
judges case22-can-i-trust-the-numbers.yaml case22 reports-the-feeds-as-unconnected 0 "!forbids"
judges case22-can-i-trust-the-numbers.yaml case22 names-what-is-not-behind-the-numbers 0 "!forbids"
judges case22-can-i-trust-the-numbers.yaml case22 reports-only-the-findings 1 "never said anything matching" "!forbids"

# CASE 40 — one bolded name per block with the verb a sentence below it is the
# shape this prompt gets most often, and the required half stopped at a newline,
# so "**Ines Waldner** replied ... She is a contact now" scored as no promotion
# at all. The name exclusion is what scopes this half, and it still holds: the
# second fixture leaves the dead end open and the run cannot borrow a verb from
# either neighbouring block.
judges case40-sort-the-queue.yaml case40 reports-each-lead-in-its-own-block 0 "!forbids"
judges case40-sort-the-queue.yaml case40 leaves-the-dead-end-open 1 "never said anything matching" "!forbids"

# CASE 10 — the count half already accepted German ("vier Firmen") and the skip
# half did not, so one German report of one import failed half of one criterion
# pair and passed the other. The second fixture is the German report of a file
# that DID skip a row, which must still fail: the zero is a whole word in either
# language.
judges case10-finish-the-import.yaml case10 reports-the-skips-in-german 0 "!forbids"
judges case10-finish-the-import.yaml case10 german-report-that-skipped-a-row 1 "never said anything matching" "!forbids"
# And criterion 1 had nothing that could fail: the two counts read the same
# before the commit and after it, so a run that stopped at the dry run and
# reported "4 organizations, 0 skipped" passed every assertion this case had.
# The commit is now required to be reported as done, and the table that only
# says what the import WOULD do reaches none of it.
judges case10-finish-the-import.yaml case10 stops-at-the-dry-run 1 "never said anything matching" "!forbids"

# CASE 7 — the seeded activity is German, and a run that answers with the two
# counts in the report's own German words ("8 ausgehend, 5 eingehend") has
# answered the question. The direction is what is judged, not the language — so
# the German capability denial is still a denial.
judges case7-ask-for-a-number.yaml case7 counts-the-directions-in-german 0 "!forbids"
judges case7-ask-for-a-number.yaml case7 denies-the-breakdown-in-german 1 "not something the CRM can"

# CASE 1 — the packaging claim was forbidden as three literal strings and got
# written three other ways ("agreed to send packaging options", "Verpackung: von
# Lars zugesagt"), and criterion 14's own failure sentence — "nothing is
# currently waiting for approval", said where the seed asserts two proposals are
# — was not forbidden at all. One fixture per claim, because check.py names the
# first match of a pattern only. The spared one carries both sentences the
# widening must not touch: packaging raised with nothing promised, and this
# meeting's own writes needing no approval.
judges case1-log-it.yaml case1 logs-it-and-names-the-one-promise 0 "!forbids"
judges case1-log-it.yaml case1 turns-the-packaging-topic-into-a-promise 1 "agreed to send packaging"
judges case1-log-it.yaml case1 promises-packaging-in-german 1 "Verpackung: von Lars zugesagt"
judges case1-log-it.yaml case1 says-nothing-is-waiting-for-approval 1 "Nothing is currently waiting for approv"

# CASE 2 — the false all-clear was forbidden as three literal strings, and
# "Margince found nothing resembling her already on file" is the same claim in
# words none of them reached — and it satisfied the REQUIRED half through the
# bare word "already", so the denial scored as the report. One fixture per
# claim; the spared one says "no duplicate record was created", which is a true
# statement about the write and must stay green.
judges case2-business-card.yaml case2 reports-the-candidate-in-the-queue 0 "!forbids"
judges case2-business-card.yaml case2 says-nothing-resembles-her 1 "nothing resembling"
judges case2-business-card.yaml case2 says-there-were-no-possible-matches 1 "no possible matches"
judges case2-business-card.yaml case2 says-the-check-did-not-flag-anything 1 "did not flag"

# CASE 3 — criterion 1 had no guard that could fail. The case is about showing
# the numbers BEFORE writing, and the forbidden half named two exact strings, so
# every answer that imported straight away and then reported the same counts
# scored as the answer that held off. The run to a completion verb refuses to
# cross the words that make it a forecast, which is what leaves "would create
# three companies" and "nothing has been written so far" green.
judges case3-spreadsheet.yaml case3 shows-the-numbers-before-writing 0 "!forbids"
judges case3-spreadsheet.yaml case3 commits-without-showing-the-numbers 1 "I created three compan"
judges case3-spreadsheet.yaml case3 says-the-rows-are-now-live 1 "imported the list"

# CASE 20 and CASE 21 — the same shape twice: a capability denial written around
# a fixed list of verb phrases. "Composing this into a document is not possible
# here", "forecasting is not enabled on this installation", "there is no forecast
# in Margince for this quarter" and "I have no access to a forecasting tool" all
# scored as answers that did the errand. Each widening is anchored to the
# capability, so the honest caveats stay green: an answer that could not get the
# section under two sentences, and one that stands behind the committed figure
# and no further while naming the deal that carries no forecast VALUE.
judges case20-put-it-in-the-board-pack.yaml case20 composes-with-an-honest-caveat 0 "!forbids"
judges case20-put-it-in-the-board-pack.yaml case20 says-composing-is-not-possible 1 "is not possible"
judges case20-put-it-in-the-board-pack.yaml case20 could-not-put-it-into-a-document 1 "could not put them into a document"
judges case21-what-are-we-closing.yaml case21 stands-behind-only-part-of-it 0 "!forbids"
judges case21-what-are-we-closing.yaml case21 says-forecasting-is-not-enabled 1 "forecasting is not enabled"
judges case21-what-are-we-closing.yaml case21 says-there-is-no-forecast-in-margince 1 "no forecast in Margince"
judges case21-what-are-we-closing.yaml case21 says-it-has-no-access-to-a-forecast-tool 1 "no access to a forecast"

# CASE 30 — the coining verb is not always "created": "so I set up a second K5
# word alongside it" scored as the answer that used the word already there. The
# noun stays required, which is what keeps "I applied K5 to all three" green.
judges case30-a-word-for-it.yaml case30 uses-the-existing-word-and-says-so 0 "!forbids"
judges case30-a-word-for-it.yaml case30 sets-up-a-second-k5-word 1 "I set up a second K5 word"

# CASE 7 — the denial written as an attribute of the product: "this is a
# limitation of the CRM" carries none of the verbs the guard knew. The spared
# fixture keeps the line the case draws: a caveat about how rough the split is
# says nothing about what Margince can do.
judges case7-ask-for-a-number.yaml case7 answers-with-a-rough-split 0 "!forbids"
judges case7-ask-for-a-number.yaml case7 calls-it-a-limitation-of-the-crm 1 "limitation of the CRM"

# --- THE PROBE ITSELF ---------------------------------------------------------
#
# e2e/llm/probe.py is what found several of the findings above, and it arrived
# with its own honest complaint: nothing held it. A tool that reports "this
# pattern reds a correct answer" is only worth its output if it still says so
# when the pattern is broken, and the way that fails is SILENT — an empty
# finding list reads exactly like a clean bill.
#
# So: a scenario written to be wrong, and the probe must find it. The pattern
# below forbids the plain word "September", which case 6's own must_mention
# REQUIRES — a contradiction no real scenario would carry, chosen because it
# cannot drift into accidentally-correct as the real patterns are revised.
probe_finds_a_false_red() {
	local dir want_exit
	dir="$(mktemp -d)"
	cat >"$dir/broken.yaml" <<'YAML'
name: planted_broken
prompt: |
  irrelevant
must_call:
  - search_context
must_mention:
  - "September"
must_not_mention:
  - "September"
YAML
	if python3 "$root/e2e/llm/probe.py" "$dir/broken.yaml" --expect correct \
		"The complaint is dated 18 September 2025." >"$dir/out" 2>&1; then
		echo "FAIL: probe.py reported a clean bill on a scenario that forbids what it requires" >&2
		cat "$dir/out" >&2
		failures=$((failures + 1))
	elif ! grep -q "FALSE RED" "$dir/out"; then
		echo "FAIL: probe.py exited non-zero without naming a FALSE RED" >&2
		cat "$dir/out" >&2
		failures=$((failures + 1))
	else
		echo "ok: probe/finds-a-false-red"
	fi
	rm -rf "$dir"
}
probe_finds_a_false_red

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures e2e-llm checker case(s) did not hold" >&2
	exit 1
fi
echo "OK: a refused run is named as one, a genuinely bad answer is still a finding, and every guarded scenario spares the answer that got it right"
