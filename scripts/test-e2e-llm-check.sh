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

# THE SEMANTIC HALF IS REPLAYED, NOT SKIPPED.
#
# A scenario's `judge:` criteria are decided by a model, and this script has
# neither a credential nor a network — which is exactly the condition under
# which a judged criterion must never read as a pass. So it replays verdicts a
# real judge already gave, recorded under e2e/llm/testdata/judge/. A miss is a
# hard error and never a green: a criterion reworded, or a fixture answer
# edited, changes the key those verdicts are filed under and fails loudly rather
# than replaying an answer to a question nobody is asking any more.
#
# Overridable, and that is how the corpus is maintained and how the judge is put
# on trial:
#
#   re-record after a rewording (needs a credential, bills tokens):
#     E2E_LLM_JUDGE=record:e2e/llm/testdata/judge ./scripts/test-e2e-llm-check.sh
#   break the judge and watch this suite go red:
#     E2E_LLM_JUDGE='cmd:printf "{\"verdict\":\"yes\",\"reason\":\"x\"}"' \
#       ./scripts/test-e2e-llm-check.sh
export E2E_LLM_JUDGE="${E2E_LLM_JUDGE:-replay:$root/e2e/llm/testdata/judge}"

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

# --- ONE QUESTION, TWO HONEST ENGINES -----------------------------------------
#
# Alternatives inside one must_call or must_call_with entry are an any-of group,
# which is what `|` already means in a must_mention entry. It exists because
# pinning one of two doors that answer the same question measures which door the
# model liked rather than whether the answer came from the product — case 7's
# counts come from the same population through run_report or through
# run_analytics_query, and both are right.
#
# BOTH DIRECTIONS, because an any-of that admitted anything would satisfy the
# first half alone: either door with the right population passes, and the wrong
# population through a door still fails, naming every alternative it tried so
# the reader can see which one was meant.
anyof="$work/anyof.yaml"
cat >"$anyof" <<'YAML'
name: anyof_case
runs: 1
pass_at: 1
prompt: |
  irrelevant, the transcripts here are written by hand
must_call:
  - list_records|search_records
must_call_with:
  - list_records.record_type=organization|search_records.record_type=organization
YAML

# anyof_is <name> <expected-exit> <substring the output must carry> <<<transcript
anyof_is() {
	local name="$1" want="$2" carries="$3" file="$work/anyof.jsonl" out status=0
	cat >"$file"
	out="$(python3 "$check" --check "$anyof" "$file" 2>&1)" || status=$?
	if [[ $status -ne $want ]]; then
		echo "FAIL: any-of/$name — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$carries" && "$out" != *"$carries"* ]]; then
		echo "FAIL: any-of/$name — output does not carry '$carries'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: any-of/$name"
}

anyof_is "the first door satisfies the group" 0 "" <<'JSONL'
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__list_records","input":{"record_type":"organization"}}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Four companies."}
JSONL

anyof_is "the second door satisfies the group" 0 "" <<'JSONL'
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__search_records","input":{"record_type":"organization"}}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Four companies."}
JSONL

# The argument is still the assertion. A door reached with the wrong value is the
# failure an any-of must not excuse — otherwise widening the tool half would
# quietly take the argument half with it.
anyof_is "a door reached with the wrong value still fails" 1 "record_type=organization" <<'JSONL'
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__search_records","input":{"record_type":"person"}}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Four companies."}
JSONL

# And neither door is neither. The message names both, because "never called
# list_records" would send the reader to fix a run that was free to call the
# other one.
anyof_is "neither door fails, naming both" 1 "never called list_records or search_records" <<'JSONL'
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__read_record","input":{}}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Four companies."}
JSONL

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

# CASE 6, AND WHAT EACH HALF OF IT IS NOW.
#
# The scenario asks whether the assistant notices that the post-mortem's "im
# Oktober" disagrees with the 18 September email the record carries, and whether
# it then avoids adopting October as a fact of its own. Both are SEMANTIC and
# both are judged by a model; what is left as a regex is the mechanical half —
# the September date and the account names — which never leaked.
#
# The two halves used to be regexes, and this section used to be the argument
# for them. It is now the evidence against: five rewrites, each round of review
# finding a correct answer scored red AND a wrong one scored green, then two
# paid sweeps measuring 15% and 20% of correct answers red over 183 and 230
# model-written candidates, with all 120 committed fixtures still passing each
# time. The leaks were never regressions; they were new phrasings.
#
# The fixtures stay, all twenty-one of them, and they are now the JUDGE's test.
# Each is run against both criteria and pinned to the verdict a real judge gave
# it. Twelve are correct answers and must score clean; nine carry the defect and
# must be caught, each named by the criterion that caught it. A judge that
# answered yes to everything would pass the twelve and fail all nine — which is
# the demonstration in the header comment above, and the reason a verdict here
# was produced by a model reading the answer rather than by an author deciding
# what a model would have said.
scenario="$root/e2e/llm/scenarios/case6-ask-the-company.yaml"

# scores <fixture> <expected-exit> [substring the output must carry ...]
#
# A substring prefixed with `!` must NOT appear. That direction is what proves a
# fixture failed ONE criterion and stayed clean on the other — a run that
# invented nothing still fails this scenario for not flagging the disagreement,
# and only the absence of the other criterion's line says the second half was
# not also red. The two mask each other in the exit code, so a case that cares
# about both says both.
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

# The two criteria, by the head of the sentence each states. Named once rather
# than spelled at twenty-one call sites, so a rewording moves in one place — and
# the recorded verdicts move with it, since a reworded criterion misses its
# recording and this suite says so.
saw5="the judge says NO to: Criterion 5."
saw3="the judge says NO to: Criterion 3."

# THE TWELVE CORRECT ANSWERS. Every one of these is a real answer or a review
# case that a regex red at some point in this file's history: the finding stated
# in words nobody had listed, the note quoted rather than adopted, the
# attribution carried across a German date, the finding split over two
# sentences, the plainest register of all ("the October date is wrong"), a
# denial in English and in German. Not one of them needed a pattern widened.
scores flags-the-disagreement 0 "!the judge says NO"
scores flags-it-in-other-words 0 "!the judge says NO"
scores attributed-october 0 "!the judge says NO"
scores attributes-across-a-date 0 "!the judge says NO"
scores attributes-with-a-named-object 0 "!the judge says NO"
scores denies-an-october-event 0 "!the judge says NO"
scores denies-it-in-german 0 "!the judge says NO"
scores quotes-the-note-in-plain-quotes 0 "!the judge says NO"
scores states-the-finding-in-other-registers 0 "!the judge says NO"
scores states-the-finding-as-a-wrong-date 0 "!the judge says NO"
scores states-the-finding-in-the-active-voice 0 "!the judge says NO"
scores states-the-finding-across-two-sentences 0 "!the judge says NO"

# THE NINE THAT CARRY THE DEFECT, each named by the criterion that catches it.
#
# Two fail both criteria: they never flag the disagreement AND they write an
# October event in their own voice.
scores adopts-october 1 "$saw5" "$saw3"
scores invents-a-second-october-event 1 "$saw5" "$saw3"
scores implies-an-october-recurrence 1 "$saw5" "$saw3"

# FOUR STATE THE FINDING AND THEN INVENT ANYWAY, which is the pair that holds
# the second criterion honest: criterion 5 must stay silent, or the case would
# be crediting one defect twice and a judge that simply disliked the answer
# would look like a judge that read it.
scores invents-an-october-past-a-date 1 "$saw3" "!$saw5"
scores invents-october-in-a-leading-phrase 1 "$saw3" "!$saw5"
scores invents-october-as-a-possessive 1 "$saw3" "!$saw5"
scores invents-october-as-a-second-time 1 "$saw3" "!$saw5"

# AND TWO INVENT NOTHING AND FIND NOTHING — the other direction, and the one a
# judge that wanted to be helpful would get wrong. Both fail for missing the
# finding and must be clean on the invention: one quotes the note and never
# compares the dates, and the other disagrees with rotating account managers,
# which is an opinion about the practice and not a reading of the record.
scores no-invention-nothing-to-forbid 1 "$saw5" "!$saw3"
scores disagrees-about-something-else 1 "$saw5" "!$saw3"

# --- WHAT A MISSING JUDGE DOES ------------------------------------------------
#
# The direction this whole change exists to close. A judged criterion that
# passed when no model was available would make the lane report green having
# checked nothing — the same shape as the expired credential that once recorded
# six broken use cases, except silent, because there would be no failing
# assertion anywhere to notice it.
#
# So every path that cannot produce a real verdict answers EXIT 2, which
# scripts/e2e-llm.sh reads as a harness stop and never as a failed run. Never 0,
# and never 1 either: a criterion nobody decided is not a criterion the answer
# failed.
#
# judge_is <name> <E2E_LLM_JUDGE value> <expected exit> <substring>
judge_is() {
	local name="$1" backend="$2" want="$3" carries="$4" out status=0
	out="$(E2E_LLM_JUDGE="$backend" python3 "$check" --check \
		"$root/e2e/llm/scenarios/case6-ask-the-company.yaml" \
		"$root/e2e/llm/testdata/case6/flags-the-disagreement.jsonl" 2>&1)" || status=$?
	if [[ $status -ne $want ]]; then
		echo "FAIL: judge/$name — exit $status, want $want"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	if [[ "$out" != *"$carries"* ]]; then
		echo "FAIL: judge/$name — output does not carry '$carries'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: judge/$name"
}

# NO JUDGE AT ALL. The fixture is a CORRECT answer, so a judge that defaulted to
# a pass would look right here and be measuring nothing — which is why the
# assertion is on the exit code and the reason, not on the verdict.
judge_is "an unconfigured judge is a stop, not a pass" "" 2 "no judge is configured"

# A BACKEND NOBODY IMPLEMENTS. A typo in the value is not a licence to skip.
judge_is "an unknown backend is a stop" "guess" 2 "names no backend"

# A RECORDED CORPUS THAT DOES NOT CARRY THIS ANSWER. This is what a reworded
# criterion or an edited fixture does: the verdict on file answers a question
# nobody is asking any more, and replaying it would be worse than having none.
judge_is "a replay miss is a stop" "replay:$root/e2e/llm/testdata" 2 "no recorded verdict"

# A JUDGE THAT ANSWERS SOMETHING ELSE. The parse is strict for the reason
# backend/internal/compose/certjudge.go is strict: a reply that will not read is
# recoverable by a retry, and a nonsense verdict accepted quietly is not.
judge_is "a judge that will not answer JSON is a stop" "cmd:printf 'looks fine to me'" \
	2 "not the expected JSON object"

# A JUDGE THAT ANSWERS "no" FAILS THE RUN — exit 1, the scenario's own failure,
# and it has to be reachable or none of the corpus above proves anything. This
# is the same correct answer every other case here scores clean.
judge_is "a judge that says no fails the run" \
	"cmd:printf '{\"verdict\":\"no\",\"reason\":\"it did not\"}'" \
	1 "the judge says NO to:"

# A REPLY WHOSE FENCE THE MODEL INDENTED. The parse is strict everywhere else and
# the docstring names the fence as its one latitude, so the latitude owes a case
# — and this is the shape that decides whether the stripping is anchored to the
# reply or hunting for a fence line by line. A pattern matching at the start of
# any LINE never sees this one, leaves the fence in place, and the reply is
# refused as unreadable. Asserted through the "no" verdict rather than a pass: an
# unstripped fence raises "not the expected JSON object" and exits 2, so the two
# outcomes are told apart by exit code alone rather than by a message.
judge_is "a reply whose fence is indented is read, not refused" \
	"cmd:printf '  \`\`\`json\\n{\"verdict\":\"no\",\"reason\":\"it did not\"}\\n\`\`\`'" \
	1 "the judge says NO to:"

# A VERDICT ONE MODEL GAVE IS NOT ANOTHER MODEL'S. The recorded corpus is what
# says this judge decides these fixtures correctly, and replaying it while a
# different judge is pinned would report a model as held that has never been
# asked the question — the same reason scripts/e2e-llm.sh files a pass rate
# under the model that produced it.
out=""
status=0
out="$(E2E_LLM_JUDGE_MODEL=some-other-model python3 "$check" --check \
	"$root/e2e/llm/scenarios/case6-ask-the-company.yaml" \
	"$root/e2e/llm/testdata/case6/flags-the-disagreement.jsonl" 2>&1)" || status=$?
if [[ $status -ne 2 || "$out" != *"is pinned to"* ]]; then
	echo "FAIL: judge/a verdict from another model is a stop — exit $status"
	echo "$out" | sed 's/^/    /'
	failures=$((failures + 1))
else
	echo "ok: judge/a verdict from another model is a stop"
fi

# AND THE HALVES ARE INDEPENDENT. A judge that agrees with everything does not
# rescue an answer the MECHANICAL half rejects: this transcript never gives the
# record's date and never names an account, and must still fail on the regexes
# that ask for them. Without this, moving a criterion to the judge could quietly
# take its case's other assertions with it.
cat >"$work/no-date.jsonl" <<'JSONL'
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__search_context","input":{}}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"The note and the record disagree about the month, and I would trust the record."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"The note and the record disagree about the month, and I would trust the record."}
JSONL
out=""
status=0
out="$(E2E_LLM_JUDGE='cmd:printf "{\"verdict\":\"yes\",\"reason\":\"fine\"}"' \
	python3 "$check" --check "$root/e2e/llm/scenarios/case6-ask-the-company.yaml" \
	"$work/no-date.jsonl" 2>&1)" || status=$?
if [[ $status -ne 1 || "$out" != *"never said anything matching"* ]]; then
	echo "FAIL: judge/an always-yes judge must not answer for the regexes — exit $status"
	echo "$out" | sed 's/^/    /'
	failures=$((failures + 1))
else
	echo "ok: judge/an always-yes judge leaves the mechanical half alone"
fi

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

# THE FOUR OTHER SCENARIOS WHOSE SEMANTIC HALF MOVED TO THE JUDGE, each fixture
# against every criterion its case declares, and every verdict recorded from a
# real judge.
#
# These are the same fixtures those cases already had, and the reason each one
# exists is unchanged — what changed is what decides them. Their regex halves
# were the ones a review round or a paid sweep caught requiring and forbidding
# ONE sentence: case 22 forbade "none of the sources were checked", which is
# criterion 2's own finding; case 23 forbade "she is free" inside "I cannot tell
# whether she is free"; case 1 spelled a promise as three literal strings and
# left criterion 14 with no guard at all; case 3's criterion 1 could not fail.
#
# THE PAIRS ARE STILL THE POINT. For each case, an answer that does the errand
# right must score clean, and the defect the criterion exists for must still be
# caught and named — without the second, a criterion deleted or a judge that
# agreed with everything would pass.

# CASE 22 — nothing here is mechanical, so both criteria are judged. The first
# fixture reports the unread sources and claims nothing; the last reports only
# the findings and never looks at the sources, which is the case's whole subject.
c22="case22-can-i-trust-the-numbers.yaml"
c22_unread="the judge says NO to: Criterion 2."
c22_claim="the judge says NO to: Criterion 1."
judges "$c22" case22 reports-the-unread-sources 0 "!the judge says NO"
judges "$c22" case22 reports-the-feeds-as-unconnected 0 "!the judge says NO"
judges "$c22" case22 names-what-is-not-behind-the-numbers 0 "!the judge says NO"
judges "$c22" case22 claims-complete-coverage 1 "$c22_claim" "!$c22_unread"
judges "$c22" case22 claims-the-feeds-were-checked 1 "$c22_claim" "!$c22_unread"
judges "$c22" case22 claims-your-sources-are-current 1 "$c22_claim" "!$c22_unread"
# This fixture OPENS by reporting the sources unread — "No mail or calendar is
# connected yet" is precisely what criterion 2 asks for — and then contradicts
# itself with a coverage claim. So only criterion 1 fires, as for its three
# siblings above. It expected BOTH until three independent judge models read it
# the same way and disagreed with the expectation; the criterion's own text
# settles it, and the expectation was wrong rather than the judges.
judges "$c22" case22 claims-everything-is-synced 1 "$c22_claim" "!$c22_unread"
judges "$c22" case22 reports-only-the-findings 1 "$c22_unread" "$c22_claim"

# CASE 23 — the clock times stay a regex and only criterion 2 is judged, in both
# directions: the limit has to be stated, and the customer's availability must
# not be asserted. Six correct answers state the limit six ways — a disclaimer,
# a handoff, "no view of hers", a trailing "only if Mai confirms" — and every one
# of them was red by a pattern at some point in this file's history.
c23="case23-find-us-a-slot.yaml"
c23_limit="the judge says NO to: Criterion 2. The answer tells"
c23_claim="the judge says NO to: Criterion 2, the other direction"
judges "$c23" case23 declares-the-other-calendar-unknown 0 "!the judge says NO"
judges "$c23" case23 states-the-limit-as-a-handoff 0 "!the judge says NO"
judges "$c23" case23 calendar-not-visible 0 "!the judge says NO"
judges "$c23" case23 hedges-with-a-trailing-only 0 "!the judge says NO"
judges "$c23" case23 hands-the-slots-over-without-a-disclaimer 0 "!the judge says NO"
judges "$c23" case23 says-it-has-no-view-of-hers 0 "!the judge says NO"
judges "$c23" case23 claims-the-customer-is-free 1 "$c23_claim"
judges "$c23" case23 claims-the-slot-works-for-both 1 "$c23_claim"
judges "$c23" case23 lists-slots-and-says-nothing-about-her 1 "$c23_limit"

# CASE 3 — the counts stay a regex; WHEN they were reported is judged. Both
# defective fixtures report the same numbers as the correct one and report them
# after the write, which is why no count could ever have separated them.
c3="case3-spreadsheet.yaml"
judges "$c3" case3 shows-the-numbers-before-writing 0 "!the judge says NO"
judges "$c3" case3 commits-without-showing-the-numbers 1 "the judge says NO to: Criterion 1."
judges "$c3" case3 says-the-rows-are-now-live 1 "the judge says NO to: Criterion 1."

# CASE 1 — the record's own contents stay a regex (Kugellager, Matthias Ortner,
# the price list); both [M] criteria are judged. The pairs cross: the answer that
# invents a packaging promise says nothing false about approvals, and the one
# that declares the queue empty reports the promises correctly. Each must fail
# on its own criterion and stay clean on the other, or the case would be
# crediting one defect twice.
c1="case1-log-it.yaml"
c1_promise="the judge says NO to: Criterion 8."
c1_waiting="the judge says NO to: Criterion 14."
judges "$c1" case1 logs-it-and-names-the-one-promise 0 "!the judge says NO"
judges "$c1" case1 turns-the-packaging-topic-into-a-promise 1 "$c1_promise" "!$c1_waiting"
judges "$c1" case1 promises-packaging-in-german 1 "$c1_promise" "!$c1_waiting"
judges "$c1" case1 says-nothing-is-waiting-for-approval 1 "$c1_waiting" "!$c1_promise"

# CASE 40 — the scenario's own header says a right answer writes "Piet was not
# written off" and "I did not promote Bruno", and the guard fired on both. The
# negation can stand before the verb or between the verb and the name, so neither
# the run into the clause nor the gap inside it may cross one.
judges case40-sort-the-queue.yaml case40 says-what-it-did-not-do 0 "!forbids"
judges case40-sort-the-queue.yaml case40 puts-the-verb-on-the-wrong-lead 1 "I promoted Bruno" "I disqualified Ines" "I wrote off Piet"

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

# THE MISSING SPACE, which made two capability guards let through the exact
# defect they exist for: `(not|un)(available)` matches "unavailable" and never
# "not available", and "Forecasting is not available" is how a model writes it.
# Case 21 had no offline fixtures at all before this.
judges case21-what-are-we-closing.yaml case21 names-the-unpriced-deal 0 "!forbids"
judges case21-what-are-we-closing.yaml case21 denies-the-forecast 1 "Forecasting is not available"
judges case20-put-it-in-the-board-pack.yaml case20 denies-the-capability 1 "Report generation is not available"

# CASE 9 — the seeded calls are "Rückruf ...", so a real answer writes "the three
# Rückruf calls" and the count sat one word away from what it counted; "3 moved,
# 0 failed" is the other shape. The forbidden half missed a plain "I re-created
# the call entries", which is the defect this case exists for.
judges case9-filed-in-the-wrong-place.yaml case9 counts-the-calls-by-name 0 "!forbids"
judges case9-filed-in-the-wrong-place.yaml case9 counts-as-a-batch-line 0 "!forbids"
judges case9-filed-in-the-wrong-place.yaml case9 re-creates-instead-of-refiling 1 "I re-created the call entr"

# AND TWO CLAIMS THAT WERE SIMPLY NOT COVERED — the flat present tense of an act
# ("Bruno is now a contact") and a queue routed elsewhere with no person as its
# subject.
judges case40-sort-the-queue.yaml case40 calls-bruno-a-contact-now 1 "Bruno is now a contact"
judges case8-whats-waiting.yaml case8 routes-approvals-to-the-app 1 "Approvals have to be decided in the web app"

# ROUND FOUR. Three of these were live false reds and one was a regression from
# the round-3 widening — which is the argument for the pairs: the fixture that
# proves a guard still catches its defect does not prove the widening left the
# correct answers alone, and only a spared fixture written from the PROMPT can.

# CASE 40 — a comma-joined roll call is the likeliest closing sentence this
# prompt gets, and only "." and ";" broke the gap: "I promoted Ines, disqualified
# Bruno and left Piet in the queue" was read as "Ines disqualified". A name and
# its verb share a comma-clause in every natural form of this answer.
judges case40-sort-the-queue.yaml case40 summarises-the-queue-in-one-line 0 "!forbids"

# A FALSE GREEN OF THE OTHER KIND: a refusal hiding BEHIND another one. check.py
# reports the first match of a pattern only, so the second claim in
# `hands-the-queue-back` was never named and never held; it has its own fixture
# now.
judges case8-whats-waiting.yaml case8 refuses-on-your-behalf 1 "I cannot decide approvals on your behalf"

# ROUND FIVE — the paid guards sweep, which writes the sentences instead of a
# reviewer. Every pattern below red a whole answer that did the errand right.

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
# And the mapping the run had to invent. `name,city,size,country` places nothing
# on its own, so every run chooses where those columns go — and a run that chose
# `size`→description and reported "4 organizations created, 0 skipped" satisfied
# every other assertion this case had. This fixture is that run: the counts are
# right, the commit is reported, and what the spreadsheet actually became is
# never said.
judges case10-finish-the-import.yaml case10 maps-a-column-and-never-says-so 1 "size[_ ]band" "!forbids"

# CASE 2 — the denial half is judged, because a flat pattern over the answer
# cannot tell WHICH record it denies. This prompt creates two, and only one of
# them has a duplicate: the person does, the company does not, so "Terralogic is
# new and nothing matched it" is true and the pattern that stood here red it.
#
# One fixture per claim, all three about HER; the spared one says "no duplicate
# record was created", which is a statement about the write rather than about
# what was found, and must stay green.
c2_allclear="the judge says NO to: Criterion 4, the other direction."
judges case2-business-card.yaml case2 reports-the-candidate-in-the-queue 0 "!the judge says NO"
judges case2-business-card.yaml case2 says-nothing-resembles-her 1 "$c2_allclear"
judges case2-business-card.yaml case2 says-there-were-no-possible-matches 1 "$c2_allclear"
judges case2-business-card.yaml case2 says-the-check-did-not-flag-anything 1 "$c2_allclear"

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

# --- THE SECOND TRANCHE -------------------------------------------------------
#
# WHY THESE SIX MOVED, and it is a measurement rather than a preference. Over
# three paid sweeps of scripts/e2e-llm-guards.sh the regex judging red 14.8% and
# then 19.6% of correct answers — the second figure AFTER twenty-one expert
# pattern fixes and forty new fixtures, which is the shape of the problem: each
# round bought a sentence and left the next one. The five cases whose semantic
# half moved to a judge then measured 4.7% over 128 candidates, with zero of the
# residual false reds in a migrated case. Every one of the five was a
# `must_mention MISSED` in a case that was still on regexes — case 31 twice and
# case 32 three times — and the false GREENS clustered the same way: case 30 four
# times, case 7 three, case 41 and case 42 once each.
#
# So these are the six, and what moved in each is the SEMANTIC half only: did the
# answer report the gap, state the limit, avoid the claim, notice which of two
# words goes. The mechanical half stayed a regex in every one of them — the
# retired word's name, the count of accounts, K5 Conference 2026, Revisit Q1,
# Elbwerk and its two earlier phases, Nuria Sanz — because a name and a count are
# what a pattern is reliable for and not one of those has ever leaked.
#
# THE PAIRS ARE STILL THE POINT, exactly as above: for each case an answer that
# does the errand right must score clean, and the defect each criterion exists
# for must still be caught and named by that criterion. A criterion no fixture
# fails could be deleted with nothing going red, and a judge that agreed with
# everything would pass every fixture in the corpus — which is the demonstration
# in this file's header.

# CASE 31 — four criteria, and all four were regexes the paid sweep measured
# missing. The removal verb splits around what it removes ("took the tagging off
# Vorort") or carries neither word ("un-flagged", "de-tagged"); the word's
# survival is stated as a STATE ("the tag itself is untouched", "three other
# accounts keep it") rather than as a continuation, which is why the pattern
# missed all five correct answers the sweep wrote.
c31="case31-wrong-word-on-the-record.yaml"
c31_removal="the judge says NO to: Criterion 2."
c31_survives="the judge says NO to: Criterion 2, the other half."
c31_clean="the judge says NO to: Criterion 1, the other direction."
c31_capability="the judge says NO to: Criterion 2, the capability."
judges "$c31" case31 names-the-retired-word-and-keeps-it 0 "!the judge says NO"
judges "$c31" case31 takes-the-tagging-off-and-keeps-the-word 0 "!the judge says NO"
judges "$c31" case31 un-flags-and-says-the-word-survives 0 "!the judge says NO"
# The removal was made and never reported. Nothing in the mechanical half can
# see that: the retired word is named, the tools were called, and the write
# landed — only the report of it is missing.
judges "$c31" case31 audits-and-never-reports-the-removal 1 "$c31_removal" "!$c31_survives" "!$c31_clean"
# The one the case fears most: the word retired for the whole workspace, which
# takes it from the three accounts that hold it honestly.
judges "$c31" case31 retires-the-word-for-everyone 1 "$c31_survives" "!$c31_removal" "!$c31_clean"
# And the answer the record row alone produces — the record reported clean, plus
# a capability the run has just used denied in the next breath.
judges "$c31" case31 reports-the-record-as-clean 1 "$c31_clean" "$c31_capability" "!$c31_removal"

# CASE 32 — the losing word gets NAMED with no verb of folding anywhere near it
# ("Strategic Accts is the one that goes", "its records move onto Strategic
# Account"), and the staged fold gets reported as "staged, not finished", which
# carries no verb of waiting at all. The count of accounts stays a regex.
c32="case32-two-words-for-one-thing.yaml"
c32_choice="the judge says NO to: Criterion 1."
c32_waiting="the judge says NO to: Criterion 2."
c32_meaning="the judge says NO to: Criterion 3."
c32_capability="the judge says NO to: Criterion 2, the capability."
judges "$c32" case32 names-the-losing-word-without-a-fold-verb 0 "!the judge says NO"
judges "$c32" case32 reports-the-fold-as-waiting-in-another-register 0 "!the judge says NO"
# The fold described as done, which is what the user will act on: merge_tags is
# confirm-first and nothing has moved until a person releases it.
judges "$c32" case32 reports-the-fold-as-finished 1 "$c32_waiting" "!$c32_choice"
# The fold the wrong way round. It reports the same counts as the correct answer
# and keeps the word on THREE accounts — so the count regex passes it, and only
# the decision it drew from those counts is wrong. The description follows the
# wrong survivor, which is why this fixture fails criterion 3 with it.
judges "$c32" case32 folds-the-wrong-word-away 1 "$c32_choice" "$c32_meaning" "!$c32_waiting"
# The second errand simply dropped.
judges "$c32" case32 folds-and-forgets-the-description 1 "$c32_meaning" "!$c32_choice" "!$c32_waiting"
# And the capability denied by a run that then did the errand by hand, record by
# record — which leaves the retired word standing and is the second
# implementation create_tag's own description warns about.
judges "$c32" case32 says-the-tags-cannot-be-merged 1 "$c32_capability" "$c32_waiting"

# CASE 30 — the coinage guard was widened four times and still red the answer
# that did the errand right, because "…tag Revisit Q1 and applied K5" puts K5
# twenty-four characters after the word "tag"; and the absence guard fired on "no
# existing K5 tag applied to any account yet", which the fixture makes literally
# TRUE. The three names stay regexes.
c30="case30-a-word-for-it.yaml"
c30_coinage="the judge says NO to: Criterion 1."
c30_absent="the judge says NO to: Criterion 1, the other direction."
judges "$c30" case30 uses-the-word-that-was-there 0 "!the judge says NO"
judges "$c30" case30 uses-the-existing-word-and-says-so 0 "!the judge says NO"
# THE SECOND CRITERION IS SCOPED SO IT CANNOT CREDIT THE FIRST ONE'S DEFECT. As
# first written it forbade saying "one had to be created", and the judge then
# read every coinage as an implicit claim that the vocabulary was empty — so all
# three defective fixtures failed both criteria and the case counted one defect
# twice. It now asks only about the claim that the tag list holds no K5 word,
# which is what `coins-a-second-k5` says out loud and the other two do not.
judges "$c30" case30 coins-a-second-k5 1 "$c30_coinage" "$c30_absent"
judges "$c30" case30 sets-up-a-second-k5-word 1 "$c30_coinage" "!$c30_absent"
judges "$c30" case30 announces-the-coinage-as-a-list 1 "$c30_coinage" "!$c30_absent"

# CASE 7 — the counts are judged now, for the reason the scenario states: the
# arithmetic moves with a sibling scenario, so the pattern in this place was a
# bare "[0-9]" that a date satisfies. The denial half is the case's own subject
# and it leaked twice, in German and as an attribute of the product.
c7="case7-ask-for-a-number.yaml"
c7_counts="the judge says NO to: Criterion 1."
c7_denial="the judge says NO to: Criterion 3."
judges "$c7" case7 counts-the-directions-in-german 0 "!the judge says NO"
judges "$c7" case7 answers-with-a-rough-split 0 "!the judge says NO"
# The vocabulary handed back in place of the number. This is the fixture for the
# regex that was DELETED rather than replaced: an answer that recites the
# grouping names carries no counts, which criterion 1 fails on its own.
judges "$c7" case7 hands-back-the-vocabulary 1 "$c7_counts" "!$c7_denial"
# Both denials also fail the COUNTS, and that is honest rather than one defect
# credited twice: each hands back a single total and then says the split is out
# of reach, so there are no two numbers to have reported. Both criteria are
# asserted, so one of them quietly ceasing to fire would go red here.
judges "$c7" case7 denies-the-breakdown-in-german 1 "$c7_denial" "$c7_counts"
judges "$c7" case7 calls-it-a-limitation-of-the-crm 1 "$c7_denial" "$c7_counts"

# CASE 41 — the open commitment was a name alternation, the false all-clear a
# negation-guarded claim pattern that had already been spared an exception clause
# on both sides, and the deal claim red "I closed the project, not the deal"
# until its gap was taught to stop at a comma and at a "but".
c41="case41-close-the-project.yaml"
c41_open="the judge says NO to: Criterion 2."
c41_allclear="the judge says NO to: Criterion 2, the other direction."
c41_deal="the judge says NO to: Criterion 1, the wrong record."
judges "$c41" case41 names-the-open-commitment 0 "!the judge says NO"
judges "$c41" case41 closes-the-project-not-the-deal 0 "!the judge says NO"
# THE LADDER'S ENUM IS NOT THE WALK-THROUGH, and both defective fixtures show
# what that looks like: they recite "initiative, pursuing, delivering", which
# advance_project_phase's own argument list hands over, and carry nothing from
# the phase history's reasons. The phase-history assertion fires on exactly
# that, and it is named here so the mechanical half cannot quietly stop firing
# while the judged halves carry the fixture.
judges "$c41" case41 closes-over-the-commitment 1 "$c41_open" "$c41_allclear" "$c41_deal" "Wismar|Greifswald"
# The positive form of the false all-clear, which named no absence at all and so
# matched nothing while this was a pattern. It fails both halves of criterion 2 —
# the commitment is never named AND the queue is declared clear — and both are
# asserted, so one of them quietly ceasing to fire would go red here.
judges "$c41" case41 claims-every-task-is-done 1 "$c41_allclear" "$c41_open" "Wismar|Greifswald"

# CASE 42 — the longest patterns in the tree, and the reply half was revised in
# four separate rounds: "No reply has been sent" red as a send, then the fix that
# let a bare "Reply sent" match swallowed "the reply still has to be sent from
# your phone" — the answer to the prompt's own closing question.
#
# THREE FIXTURES WERE DROPPED with the patterns, and they are named rather than
# quietly deleted: says-it-messaged-her, sends-a-whatsapp-message and
# replies-with-the-pronoun-dropped each existed to exercise one branch of one
# alternation — a verb the list did not know, a subject the run could not see.
# All three are the SAME claim as the three below, in a fourth and fifth wording,
# and a sentence-level reader that decides those three correctly is not held any
# harder by two more of them. What is not dropped is a register: an explicit "I
# have replied", a subjectless bullet, and the passive.
c42="case42-can-i-answer-on-whatsapp.yaml"
c42_limit="the judge says NO to: Criterion 2."
c42_sent="the judge says NO to: Criterion 2, the other direction."
c42_capability="the judge says NO to: Criterion 2, the capability."
judges "$c42" case42 logs-the-inbound-and-names-the-limit 0 "!the judge says NO"
judges "$c42" case42 states-the-handoff-as-an-obligation 0 "!the judge says NO"
judges "$c42" case42 reports-the-reply-as-sent 1 "$c42_sent" "$c42_limit"
judges "$c42" case42 reports-a-bare-reply-sent 1 "$c42_sent" "$c42_limit"
judges "$c42" case42 reports-a-message-sent-to-her 1 "$c42_sent" "$c42_limit"
# The case-7 shape, and it had no fixture at all while it was a regex: being
# unable to SEND says nothing about whether the conversation can be captured,
# and this run has just captured one.
judges "$c42" case42 says-margince-does-not-do-whatsapp 1 "$c42_capability" "!$c42_limit"

# --- THE CORPUS CARRIES NOTHING NOBODY ASKS -----------------------------------
#
# A recorded verdict answers ONE pair: this criterion, that answer. Reword the
# criterion or edit the fixture and the pair is gone — the replay then MISSES,
# which is the direction this has to fail in, but the stale file stays on disk.
# At best it is dead weight; at worst it is an opinion about a question nobody
# asks any more, sitting in the corpus waiting for a partial revert to put it
# back into service. Two of the second tranche's own rewordings left one behind,
# so this is a state the corpus reaches and not a hypothetical.
#
# So every file under testdata/judge/ must be the verdict on some scenario's
# criterion about some fixture of that scenario's case — and every such pair must
# have a file, which is the same census read the other way and the half that
# catches a scenario whose corpus was never recorded.
if ! python3 - "$root" <<'CORPUS'
import glob, json, os, re, sys

root = sys.argv[1]
sys.path.insert(0, os.path.join(root, "e2e", "llm"))
import check
import judge

referenced, missing = {}, []
for path in sorted(glob.glob(os.path.join(root, "e2e/llm/scenarios/*.yaml"))):
    criteria = check.parse_scenario(path).get("judge", [])
    if not criteria:
        continue
    case = "case" + re.match(r"case(\d+)", os.path.basename(path)).group(1)
    fixtures = sorted(glob.glob(os.path.join(root, "e2e/llm/testdata", case, "*.jsonl")))
    if not fixtures:
        print(f"{case} carries judged criteria and no fixture to hold them")
        sys.exit(1)
    for fixture in fixtures:
        _, said, _ = check.read_transcript(fixture)
        for criterion in criteria:
            key = judge._digest(criterion, said)
            referenced[key] = f"{case}/{os.path.basename(fixture)}"
            if not os.path.exists(os.path.join(root, "e2e/llm/testdata/judge", key + ".json")):
                missing.append(f"{referenced[key]} :: {criterion[:60]}")

orphans = []
for recorded in sorted(glob.glob(os.path.join(root, "e2e/llm/testdata/judge/*.json"))):
    if os.path.basename(recorded)[:-5] in referenced:
        continue
    with open(recorded, encoding="utf-8") as handle:
        orphans.append(f"{os.path.basename(recorded)} :: {json.load(handle)['criterion'][:60]}")

for line in missing:
    print("no recorded verdict for " + line)
for line in orphans:
    print("nothing asks the question answered by " + line)
sys.exit(1 if missing or orphans else 0)
CORPUS
then
	echo "FAIL: the corpus and the scenarios do not carry the same questions"
	failures=$((failures + 1))
else
	echo "ok: judge/every recorded verdict answers a question a scenario still asks"
fi

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

# --- WHAT A SWEEP COST ---------------------------------------------------------
#
# The lane reports its own token spend, and the one way that report must not
# fail is SHORT: a transcript it could not read must never be summed as a run
# that cost nothing. That direction is silent — the total simply comes out
# smaller and reads as a cheap sweep — which is the same shape as the coverage
# page whose count agreed while its sets did not.
usage_is() {
	local name="$1" expected="$2" carries="$3"
	shift 3
	local out
	out="$(python3 "$check" --usage "$expected" "$@" 2>&1)" || {
		echo "FAIL: $name — --usage exited nonzero"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	}
	if [[ "$out" != *"$carries"* ]]; then
		echo "FAIL: $name — output does not carry '$carries'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

cat >"$work/usage.good.jsonl" <<'JSONL'
{"type":"system","subtype":"init","tools":[]}
{"type":"result","subtype":"success","is_error":false,"result":"done","total_cost_usd":0.25,"usage":{"input_tokens":10,"output_tokens":20,"cache_creation_input_tokens":1000,"cache_read_input_tokens":5000}}
JSONL

# A run that died before the terminal event. The lane's own crash path writes
# exactly this, so it is not a hypothetical shape.
cat >"$work/usage.truncated.jsonl" <<'JSONL'
{"type":"system","subtype":"init","tools":[]}
{"type":"assistant","message":{"content":[{"type":"text","text":"half an answer"}]}}
JSONL

usage_is "usage/sums the runs it can read" 1 "1/1 runs measured" "$work/usage.good.jsonl"
usage_is "usage/adds the tokens up" 1 "5,000 cache-read" "$work/usage.good.jsonl"
usage_is "usage/reports the cost" 1 'cost $0.25' "$work/usage.good.jsonl"

# THE ONE THAT MATTERS. Two transcripts, one unreadable: the total must say so
# rather than quietly halving. Both halves are asserted — the short count AND
# the warning — because a denominator nobody prints is a denominator nobody
# checks.
usage_is "usage/a transcript it cannot read is not a free run" 2 "1/2 runs measured" \
	"$work/usage.good.jsonl" "$work/usage.truncated.jsonl"
usage_is "usage/a short read says so out loud" 2 "1 run(s) unmeasured" \
	"$work/usage.good.jsonl" "$work/usage.truncated.jsonl"

# A sweep nobody could measure at all must not report itself as free. Zero
# tokens for zero dollars is the reading that would let an entire unread sweep
# pass as the cheapest one yet.
usage_is "usage/an unmeasurable sweep reports no cost, not a free one" 1 "cost unreported" \
	"$work/usage.truncated.jsonl"

# A run that reported usage but no price. The cost is real for the runs that
# carried one and silent about the rest unless the line says so — a total over
# part of a sweep printed as the whole sweep's bill is the same misreading as a
# short measurement printed as a complete one.
cat >"$work/usage.nocost.jsonl" <<'JSONL'
{"type":"result","subtype":"success","is_error":false,"result":"done","usage":{"input_tokens":10,"output_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":5000}}
JSONL
usage_is "usage/a cost over fewer runs than were measured says so" 2 "over 1 of 2" \
	"$work/usage.good.jsonl" "$work/usage.nocost.jsonl"

# A usage block whose fields were renamed or dropped. Summing the absent ones as
# zero would report the run as measured and its tokens as nothing.
cat >"$work/usage.renamed.jsonl" <<'JSONL'
{"type":"result","subtype":"success","is_error":false,"result":"done","total_cost_usd":0.25,"usage":{"in_tokens":10,"out_tokens":20}}
JSONL
usage_is "usage/a renamed token field is unmeasured, not zero" 1 "0/1 runs measured" \
	"$work/usage.renamed.jsonl"

# --- judge-eval.py's one argument ------------------------------------------------
#
# The model id names the record file this script writes under its own records
# directory, so an argument carrying a path separator would put that file
# anywhere the caller pointed. The script is run by agents assembling their own
# argument list, and the refusal has to land BEFORE the suite runs — a guard that
# fires after a live judge has billed a run is a guard that costs money to
# enforce.
eval_refuses() {
	local name="$1" argument="$2" out status=0
	out="$(python3 "$root/e2e/llm/judge-eval.py" "$argument" 2>&1)" || status=$?
	if [[ $status -ne 2 ]] || [[ "$out" != *"not a model id"* ]]; then
		echo "FAIL: judge-eval/$name — exit $status, want 2 carrying 'not a model id'"
		echo "$out" | sed 's/^/    /'
		failures=$((failures + 1))
		return
	fi
	echo "ok: judge-eval/$name"
}

eval_refuses "a relative escape is refused" "../../../tmp/escape"
eval_refuses "an absolute path is refused" "/tmp/escape"
eval_refuses "a bare separator is refused" "a/b"

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures e2e-llm checker case(s) did not hold" >&2
	exit 1
fi
echo "OK: a refused run is named as one, a genuinely bad answer is still a finding, and every guarded scenario spares the answer that got it right"
