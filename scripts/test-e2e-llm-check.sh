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
# a test has to respect rather than work around.
work="$root/.tmp/e2e-llm-check-test"
rm -rf "$work"
mkdir -p "$work"
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

# An error with no message still says the run did not happen, rather than
# reading as a silent success.
case_is "an error with no message is still not a run" 1 "reported an error" <<'JSONL'
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

# And one that did the right thing.
case_is "a run that called a tool and answered is a run" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__list_records","input":{}}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Here are the records."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"Here are the records."}
JSONL

# The lane's own wiring: the check must be REACHED, and its failure must stop
# the lane rather than being scored. Asserted on the script's text, because
# running the lane needs a key and a stack.
lane="$root/scripts/e2e-llm.sh"
for required in "check.py\" --ran" "HARNESS: the model was never reached" "exit 2"; do
	if ! grep -q -- "$required" "$lane"; then
		echo "FAIL: scripts/e2e-llm.sh does not carry '$required', so a refused run is scored as a failed scenario"
		failures=$((failures + 1))
	else
		echo "ok: the lane carries $required"
	fi
done

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures e2e-llm checker case(s) did not hold" >&2
	exit 1
fi
echo "OK: a refused run is named as one, and a genuinely bad answer is still a finding"
