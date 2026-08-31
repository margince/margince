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
case_is "an error code containing 401 is not a refusal" 0 "" <<'JSONL'
{"type":"system","subtype":"init","tools":["mcp__margince__list_records"]}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"mcp__margince__list_records","input":{}}]}}
{"type":"result","subtype":"error","is_error":true,"result":"upstream returned HTTP 4011 after the tool call"}
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

if [[ $failures -ne 0 ]]; then
	echo "FAIL: $failures e2e-llm checker case(s) did not hold" >&2
	exit 1
fi
echo "OK: a refused run is named as one, and a genuinely bad answer is still a finding"
