#!/usr/bin/env bash
# test-scheduled-report.sh — prove scheduled-report.sh files ONE issue per
# failing check, however long the tracker has grown.
#
# It runs beside the reporter in the scheduled lane for the same reason
# `test-reap-build-caches.sh` runs beside the reaper: the reporter can only be
# observed succeeding, and "it filed something" is not evidence it filed the
# right thing. A dedupe that reads a first page of open issues reports success
# on the day it starts duplicating — the tracker outgrows the page, the
# long-lived issue falls off the end of it, and a second one is filed under a
# title that already had one.
#
# `gh` is stubbed on PATH, so no case reaches the network or the tracker.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
stub_dir="$(mktemp -d)"
trap 'rm -rf "$stub_dir"' EXIT
failures=0

# The stub answers the open-issue read from OPEN_TITLES — one `number<TAB>title`
# per line — as the paginated API answers it: a stream of JSON arrays, newest
# first, one per page. Everything the reporter then does to the tracker is
# recorded rather than performed, so a case asserts on the action taken.
cat >"$stub_dir/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$1 ${2:-}" in
"api --paginate")
	# Paginate at 100 so a case can put its match beyond the first page, which
	# is the whole failure being tested.
	#
	# Counted with `n` rather than NR, and blank records skipped, because a
	# here-string always delivers ONE line: an empty tracker arrives as a single
	# blank record with NR == 1, and keyed on NR this emitted
	# `[{"number":,"title":""}]`. jq refused it and nothing said so — errexit is
	# suppressed inside a function called as `report ... || unreported=1`, so the
	# reporter read the empty output of a FAILED read as "no issue open" and filed
	# regardless. Every case here passing `""` was asserting over a parse error
	# rather than over an empty tracker.
	awk -F'\t' '
		$0 == "" { next }
		{ items[++n] = "{\"number\":" $1 ",\"title\":\"" $2 "\"}" }
		END {
			if (n == 0) { print "[]"; exit }
			for (i = 1; i <= n; i++) {
				page = page (page == "" ? "" : ",") items[i]
				if (i % 100 == 0) { print "[" page "]"; page = "" }
			}
			if (page != "") print "[" page "]"
		}' <<<"$OPEN_TITLES"
	;;
"issue comment") echo "comment $3" >>"$ACTION_LOG" ;;
"issue close") echo "close $3" >>"$ACTION_LOG" ;;
"issue create")
	for i in $(seq 1 $#); do
		if [ "${!i}" = "--title" ]; then j=$((i + 1)); echo "create ${!j}" >>"$ACTION_LOG"; fi
		# The body is recorded, not just the title: an arm that files the right
		# issue with the wrong contents is indistinguishable from a working one
		# unless something reads what it wrote.
		if [ "${!i}" = "--body" ] && [ -n "${BODY_LOG:-}" ]; then j=$((i + 1)); printf '%s' "${!j}" >>"$BODY_LOG"; fi
	done
	;;
*) echo "unexpected gh call: $*" >&2; exit 1 ;;
esac
STUB
chmod +x "$stub_dir/gh"
export PATH="$stub_dir:$PATH"

readonly GATE_TITLE="SonarCloud quality gate is not green on main"

# expect_actions <name> <expected-actions> <open-issues-tsv> <env assignment>...
#
# The general driver: run the reporter under a given set of lane results and
# assert on the comma-joined list of things it did to the tracker. Every case
# below is one of these, because "it filed something" is not evidence it filed
# the right thing — and now that a green check CLOSES an issue, "it did nothing"
# is a distinct and testable answer that used to be indistinguishable from a
# run that never reached the arm.
expect_actions() {
	local name="$1" want="$2" open="$3"
	shift 3
	local out status got
	export ACTION_LOG="$stub_dir/actions"
	: >"$ACTION_LOG"
	set +e
	out="$(env OPEN_TITLES="$open" GH_TOKEN=stub REPO=owner/repo \
		RUN_URL=https://example.test/run/1 "$@" \
		"$root/scripts/scheduled-report.sh" 2>&1)"
	status=$?
	set -e
	got="$(paste -sd, - <"$ACTION_LOG")"

	if [[ "$status" -ne 0 ]] || [[ "$got" != "$want" ]]; then
		echo "FAIL: $name"
		echo "  exit    want 0 got $status"
		echo "  actions want '$want' got '$got'"
		printf '  output: %s\n' "$out" | head -5
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

# expect <name> <expected-actions> <open-issues-tsv>
#
# Only the quality gate is reported failing, so the expected action is exactly
# one line naming what the reporter did about GATE_TITLE.
expect() {
	expect_actions "$1" "$2" "$3" GATE_RESULT=failure GATE_STATUS=ERROR
}

# expect_llm <name> <LLM_OUTCOME> <expected-title>
#
# The model lane splits one job result into two findings, and the split is the
# part worth testing: a lane that could not run must NEVER file "a use case is
# failing when driven by a real model". That issue sends somebody reading
# transcripts for a defect that does not exist, which is the exact failure this
# lane keeps re-learning — its first local run had a dead passport presenting
# itself as six scenarios of bad answers.
expect_llm() {
	local name="$1" outcome="$2" want_title="$3" out status got
	export ACTION_LOG="$stub_dir/actions"
	: >"$ACTION_LOG"
	set +e
	out="$(OPEN_TITLES="" GH_TOKEN=stub REPO=owner/repo RUN_URL=https://example.test/run/1 \
		LLM_RESULT=failure LLM_OUTCOME="$outcome" \
		"$root/scripts/scheduled-report.sh" 2>&1)"
	status=$?
	set -e
	got="$(paste -sd, - <"$ACTION_LOG")"

	if [[ "$status" -ne 0 ]] || [[ "$got" != "create $want_title" ]]; then
		echo "FAIL: $name"
		echo "  exit    want 0 got $status"
		echo "  actions want 'create $want_title' got '$got'"
		printf '  output: %s\n' "$out" | head -5
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

LLM_LANE_TITLE="the weekly model-driven use cases could not run"
LLM_CASE_TITLE="a use case is failing when driven by a real model"

expect_llm "a model lane that never drove a scenario is not reported as a bad answer" \
	"lane-failed" "$LLM_LANE_TITLE"
expect_llm "a scenario that drove and failed is reported as the use case failing" \
	"scenario-failed" "$LLM_CASE_TITLE"

# A tracker of `n` open issues, none of them the reported one, newest first.
noise() {
	local n="$1" i
	for ((i = n; i >= 1; i--)); do printf '%s\t%s\n' "$i" "an unrelated finding $i"; done
}

expect "an empty tracker gets the issue" \
	"create $GATE_TITLE" ""

expect "an open issue under the same title is commented, not re-filed" \
	"comment 40" \
	"$(printf '99\ta later finding\n40\t%s\n' "$GATE_TITLE")"

# The defect: 150 open issues, the match at the far end. A read capped at the
# first 100 files a duplicate here and calls it a clean run.
expect "an open issue past the first page is still found" \
	"comment 7" \
	"$(noise 150 | sed "s/^7\t.*/7\t$GATE_TITLE/")"

# Two issues already carry the title — the state the capped read produced. The
# repeat belongs on the one holding the triage, which is the older number.
expect "duplicates already filed: the oldest keeps the discussion" \
	"comment 12" \
	"$(printf '300\t%s\n12\t%s\n' "$GATE_TITLE" "$GATE_TITLE")"

# Exact titles only: a near miss is a different finding, and folding it into
# this one would hide it behind an issue nobody opened for it.
expect "a similar title is a different finding" \
	"create $GATE_TITLE" \
	"$(printf '55\tSonarCloud quality gate is not green on staging\n')"

# --- retraction: what a check that came back GREEN does -------------------------
#
# The other half of the reporter, and the half that did not exist. A finding was
# filed when a lane went red and never withdrawn when it went green, so the
# tracker reported a red main over a green tree for as long as it took somebody
# to close the issue by hand. margince/margince#5118 is the worked example:
# filed 05:35Z, fixed on main at 06:49Z, still open hours after.
#
# The cases below are written around the ONE direction this must not fail in.
# Filing a finding wrongly is loud — somebody reads the issue and says so.
# Retracting one wrongly is silent: the issue closes, the red stays, and nothing
# ever asks again. So a result that is not a pass must reach neither half.

expect_actions "a green check closes the issue it filed" \
	"close 40" \
	"$(printf '99\ta later finding\n40\t%s\n' "$GATE_TITLE")" \
	GATE_RESULT=success

# Nothing to retract is not a failure, and not a reason to say anything.
expect_actions "a green check with no open issue does nothing" \
	"" "" GATE_RESULT=success

# THE ONE THAT MATTERS. A skipped job is the ABSENCE of a verdict, not a passing
# one: the weekly perf lanes are skipped on the daily cron, and the daily lane's
# arms are unset entirely when main-health runs the same reporter. Reading any
# of those as green would close a finding that nothing re-examined.
expect_actions "a skipped check neither files nor closes" \
	"" "$(printf '40\t%s\n' "$GATE_TITLE")" \
	GATE_RESULT=skipped

expect_actions "a cancelled check neither files nor closes" \
	"" "$(printf '40\t%s\n' "$GATE_TITLE")" \
	GATE_RESULT=cancelled

# An unset result is how EVERY arm of the other workflow arrives. main-health and
# scheduled.yml run one reporter and each passes only its own lanes, so on any
# given run most arms see nothing at all.
expect_actions "an unset check neither files nor closes" \
	"" "$(printf '40\t%s\n' "$GATE_TITLE")" \
	GATE_STATUS=ERROR

# The same rule the filing side follows: the oldest open match is the one
# carrying the discussion, so it is the one that gets closed.
expect_actions "duplicates already filed: the oldest is the one closed" \
	"close 12" \
	"$(printf '300\t%s\n12\t%s\n' "$GATE_TITLE" "$GATE_TITLE")" \
	GATE_RESULT=success

# --- retraction where ONE job result feeds TWO titles ---------------------------
#
# Three lanes split their result into "could not run" and "the thing it measures
# is bad", and a mirrored else is wrong for them: the middle case — a lane that
# ran and measured something bad — is a FAILURE that nonetheless retracts "could
# not run". Getting this wrong leaves a permanent "could not complete" issue open
# on a lane that completes every week, which is the stale-issue defect in a new
# costume.
#
# Parameterised over the three, because each is its own pair of titles and a copy
# of one case would pass on the day another arm's spelling drifted.

# expect_split <lane> <result-var> <outcome-var> <measured-value> <ran-title> <bad-title>
expect_split() {
	local lane="$1" res="$2" out="$3" measured="$4" ran_title="$5" bad_title="$6"
	local open
	open="$(printf '10\t%s\n20\t%s\n' "$ran_title" "$bad_title")"

	# It ran and what it measured was bad: retract "could not run", report the bad
	# measurement. Both, in the order the reporter's arms appear.
	expect_actions "$lane/a measured failure retracts 'could not run' and files what it measured" \
		"close 10,comment 20" "$open" "$res=failure" "$out=$measured"

	# It ran and everything held: both titles are false, so both go.
	expect_actions "$lane/a green run retracts both of its findings" \
		"close 10,close 20" "$open" "$res=success"

	# It never ran: the honest report, and the bad-measurement title untouched
	# because nothing measured anything.
	expect_actions "$lane/a lane that never ran files only 'could not run'" \
		"comment 10" "$open" "$res=failure"
}

expect_split perf PERF_RESULT PERF_OUTCOME breach \
	"the weekly PERF-3/PERF-7 run could not complete" \
	"a PERF-3/PERF-7 budget is breaching on main"

expect_split mobile MOBILE_RESULT MOBILE_OUTCOME breach \
	"the weekly MOBILE-AC-2 run could not complete" \
	"PERF-1's perceived budget is breaching on main"

expect_split llm LLM_RESULT LLM_OUTCOME scenario-failed \
	"the weekly model-driven use cases could not run" \
	"a use case is failing when driven by a real model"

# --- main-health arms ----------------------------------------------------------
#
# The health check's whole value is the SUSPECT RANGE it carries: "main is red" was
# already knowable from any other pull request going red. So the assertion is not
# just that an issue is filed — it is that the range reaches the body. A reporter
# that filed the issue and dropped MAIN_SUSPECTS would look identical from the
# outside and be worth nothing.

readonly GATES_TITLE="main is red: the backend gate fails on the tip"
readonly INTEGRATION_TITLE="main is red: the integration lane fails on the tip"
readonly FRONTEND_TITLE="main is red: the frontend lane fails on the tip"
readonly UAT_TITLE="main is red: the screen-acceptance UAT fails on the tip"
readonly SONAR_TITLE="main's SonarCloud analysis was not published"
readonly SUSPECTS="- \`deadbeef\` Some Author — the commit that did it"

# What the cases below actually exercised, recorded as they run rather than
# listed by hand. The census at the end of this file reads it: a list kept
# beside the arms is a list that stops being true the day somebody adds one,
# which is exactly how MAIN_GATES_RESULT went uncovered while a comment above
# claimed every arm carried both of its cases.
covered_with_range=""
covered_no_range=""

# expect_health <name> <lane-result-variable> <expected-title> <suspects>
#
# The lane is a parameter rather than baked in, so an arm added to the reporter
# is covered by asking for it here instead of by copying this function. A copy
# would pass on the day the shared assertion below stopped holding.
expect_health() {
	local name="$1" lane="$2" title="$3" suspects="$4" out status got body
	local want="create $title"
	export ACTION_LOG="$stub_dir/actions"
	export BODY_LOG="$stub_dir/body"
	: >"$ACTION_LOG"
	: >"$BODY_LOG"
	set +e
	out="$(env OPEN_TITLES="" GH_TOKEN=stub REPO=owner/repo RUN_URL=https://example.test/run/1 \
		"$lane=failure" MAIN_SUSPECTS="$suspects" \
		"$root/scripts/scheduled-report.sh" 2>&1)"
	status=$?
	set -e
	got="$(paste -sd, - <"$ACTION_LOG")"
	body="$(cat "$BODY_LOG" 2>/dev/null || true)"

	if [[ "$status" -ne 0 ]] || [[ "$got" != "$want" ]]; then
		echo "FAIL: $name"
		echo "  exit    want 0 got $status"
		echo "  actions want '$want' got '$got'"
		printf '  output: %s\n' "$out" | head -5
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$suspects" ]] && ! grep -qF -- "deadbeef" <<<"$body"; then
		echo "FAIL: $name — the issue was filed but the suspect range never reached its body"
		failures=$((failures + 1))
		return
	fi
	# The degraded path asserts its own text, not merely that an issue exists.
	# Without this the no-range cases pass for free: an arm that dropped the
	# fallback would still file, and "a red lane with no range still files"
	# would go on reporting ok over a body that says nothing about the window.
	if [[ -z "$suspects" ]] && ! grep -qF -- "no suspect range was computed" <<<"$body"; then
		echo "FAIL: $name — filed without saying the suspect range is unknown"
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$suspects" ]]; then
		covered_with_range="$covered_with_range $lane"
	else
		covered_no_range="$covered_no_range $lane"
	fi
	echo "ok: $name"
}

expect_health "a red backend gate on main is filed with its suspect range" \
	MAIN_GATES_RESULT "$GATES_TITLE" "$SUSPECTS"

expect_health "a red backend gate with no range still files" \
	MAIN_GATES_RESULT "$GATES_TITLE" ""

expect_health "a red integration lane on main is filed with its suspect range" \
	MAIN_INTEGRATION_RESULT "$INTEGRATION_TITLE" "$SUSPECTS"

# No range computed is a degraded report, not a silent one: the issue still has to
# exist, because "main is red" is worth filing even when the window is unknown.
expect_health "a red integration lane with no range still files" \
	MAIN_INTEGRATION_RESULT "$INTEGRATION_TITLE" ""

# The frontend arm. It carries a second obligation the other two do not: while it
# is red main's SonarCloud analysis is deliberately not refreshed, so the issue is
# the only place a reader learns the quality gate's verdict has stopped moving.
expect_health "a red frontend lane on main is filed with its suspect range" \
	MAIN_FRONTEND_RESULT "$FRONTEND_TITLE" "$SUSPECTS"

# Every arm owes the degraded path a case of its own, not only the first one
# written: the fallback is per-arm text, so a missing one is missing for that arm
# alone and the other arms' cases go on passing over it. The census at the end of
# this file is what holds that, rather than this comment.
expect_health "a red frontend lane with no range still files" \
	MAIN_FRONTEND_RESULT "$FRONTEND_TITLE" ""

# The screen-acceptance arm. It is the only lane that answers whether the pages
# RENDER — the SPA lane above it passes over code that builds and never mounts —
# and on a pull request it is classifier-gated, so the tip is the one place it is
# asked unconditionally.
expect_health "a red uat lane on main is filed with its suspect range" \
	MAIN_UAT_RESULT "$UAT_TITLE" "$SUSPECTS"

expect_health "a red uat lane with no range still files" \
	MAIN_UAT_RESULT "$UAT_TITLE" ""

# The publisher, which is the arm that exists because its failure is invisible:
# a stale analysis reads exactly like a current one, so nothing but this issue
# would say the verdict stopped moving.
expect_health "a failed publish of main's analysis is filed with its suspect range" \
	MAIN_SONAR_RESULT "$SONAR_TITLE" "$SUSPECTS"

expect_health "a failed publish with no range still files" \
	MAIN_SONAR_RESULT "$SONAR_TITLE" ""

# --- merge-attest.yml -------------------------------------------------------------
#
# The push-time arm. It carries no suspect range and does not want one: it names
# the offending pull request exactly, which is the whole difference between this
# alarm and the two-hourly one above.

# The REASON is a parameter, not a constant, because the two findings do not
# share one: a commit no pull request names is reported before any verdict
# exists. Pinning the failing-verdict sentence onto both cases would test the
# no-pull-request arm against a reason the judge never emits for it.
expect_merge() {
	local name="$1" pr="$2" want_title="$3" must_say="$4" out status got body
	local why="${5:-pull request #2516 landed, and its required \`ci\` check then reported \`failure\`}"
	export ACTION_LOG="$stub_dir/actions"
	export BODY_LOG="$stub_dir/body"
	: >"$ACTION_LOG"
	: >"$BODY_LOG"
	set +e
	out="$(env OPEN_TITLES="" GH_TOKEN=stub REPO=owner/repo RUN_URL=https://example.test/run/1 \
		MERGE_VERDICT_RESULT=failure MERGE_VERDICT_PR="$pr" \
		MERGE_VERDICT_WHY="$why" \
		"$root/scripts/scheduled-report.sh" 2>&1)"
	status=$?
	set -e
	got="$(paste -sd, - <"$ACTION_LOG")"
	body="$(cat "$BODY_LOG" 2>/dev/null || true)"

	if [[ "$status" -ne 0 ]] || [[ "$got" != "create $want_title" ]]; then
		echo "FAIL: $name"
		echo "  exit    want 0 got $status"
		echo "  actions want 'create $want_title' got '$got'"
		printf '  output: %s\n' "$out" | head -5
		failures=$((failures + 1))
		return
	fi
	# The reason has to REACH the issue. An arm that files under the right title
	# and then explains nothing sends the reader back to the run log, which is
	# the trip this alarm exists to save.
	if ! grep -qF -- "$must_say" <<<"$body"; then
		echo "FAIL: $name — filed, but the body never said '$must_say'"
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

# One issue per offending pull request, so the number is IN the title. A standing
# title would collect every case under one issue, be closed once, and go stale —
# which is the dedupe above working exactly as designed against a subject it does
# not fit.
expect_merge "a merge over a red check is filed against its pull request" \
	2516 "A merge landed on main against a failing verdict (#2516)" \
	'its required `ci` check then reported `failure`'

# A commit with no pull request has no number to name, and the title must still
# be a title rather than one ending in an empty parenthesis.
# The OTHER finding, and it must not borrow the first one's title: a commit no
# pull request names is reported before any verdict exists, so "against a failing
# verdict" would describe a check that never ran.
expect_merge "a merge with no pull request is titled for what it found" \
	"" "A merge landed on main with no pull request behind it" \
	"no pull request naming it" \
	"abc1234 landed on main with no pull request naming it, so no review and no required check ever applied to it"

# --- the census -----------------------------------------------------------------
#
# Derived from the reporter, not from a list here. Every MAIN_*_RESULT arm it
# carries owes both cases, and the obligation is checked rather than asserted in a
# comment — a comment claiming full coverage is what stood over the uncovered
# MAIN_GATES_RESULT arm until a reviewer read both files side by side.
#
# The health arms only. The daily lane's arms (VULN_RESULT, GATE_RESULT, …) carry
# no suspect range and are covered by the `expect` cases above, so a rule about
# the fallback text does not apply to them.
#
# What is matched is the ARM — an `if [ "${…:-}" = "failure" ]` line at column
# zero — rather than a bare identifier anywhere in the file. A bare match reads
# prose: a single explanatory comment in the reporter naming an arm would invent
# a lane that does not exist, and the census would both demand cases for it and
# count it towards the no-arm check below, so a reporter carrying no real arm
# could satisfy that check on a comment alone. Nothing in the tree does this
# today; the anchored pattern is what keeps it that way.
#
# `[A-Z0-9_]`, and the digit is the point: a census that silently drops a subject
# reports the same "nothing missing" as one that checked it, so a future
# MAIN_SHARD2_RESULT would be exempt from the rule by spelling alone. The count
# below is the same argument one level up — a pattern that matches nothing reads
# as total coverage, which is the loudest way this file could lie.
#
# `|| true` so the empty case reaches the message below: grep exits 1 on no match
# and this script runs under `set -e`, so without it the suite dies at this line
# having printed thirteen `ok:` lines and no reason — a gate that fails without
# saying what it found is barely better than one that passes without looking.
lanes="$(grep -oE '^if \[\[ "\$\{MAIN_[A-Z0-9_]+_RESULT:-\}" = "failure" \]\]' \
	"$root/scripts/scheduled-report.sh" |
	grep -oE 'MAIN_[A-Z0-9_]+_RESULT' | sort -u || true)"
if [[ -z "$lanes" ]]; then
	echo "FAIL: the census found no MAIN_*_RESULT arm in the reporter — the pattern stopped matching, it did not stop mattering"
	failures=$((failures + 1))
fi
for lane in $lanes; do
	case " $covered_with_range " in
	*" $lane "*) ;;
	*)
		echo "FAIL: the reporter has a $lane arm that no case exercises with a suspect range"
		failures=$((failures + 1))
		;;
	esac
	case " $covered_no_range " in
	*" $lane "*) ;;
	*)
		echo "FAIL: the reporter has a $lane arm that no case exercises without a suspect range"
		failures=$((failures + 1))
		;;
	esac
done

# The same wiring question, asked of the push-time alarm.
#
# MERGE_VERDICT_RESULT is fed by merge-attest.yml, not by main-health.yml, so
# the census above cannot see it — it is keyed on MAIN_*_RESULT and on that one
# workflow. An arm nothing asks about is an arm that can be added with its cases
# and still file nothing, which is the failure this census exists to catch.
#
# The limit, stated rather than implied: this covers the arms fed by a workflow
# job result. The daily lane's own arms (VULN_RESULT, GATE_RESULT, PERF_RESULT
# and the rest) are reached through scheduled.yml and are not checked here.
attest="$root/.github/workflows/merge-attest.yml"
if ! grep -qE "needs\.verdict\.result == 'failure'" "$attest"; then
	echo "FAIL: scheduled-report has a MERGE_VERDICT_RESULT arm, but merge-attest's report job does not run for a failing verdict — the arm is unreachable"
	failures=$((failures + 1))
else
	echo "ok: the push-time alarm's report job runs when the verdict fails"
fi
for key in MERGE_VERDICT_RESULT MERGE_VERDICT_PR MERGE_VERDICT_WHY; do
	if ! grep -qE "^ *${key}: \\$\{\{ needs\.verdict\." "$attest"; then
		echo "FAIL: merge-attest's report job does not pass $key to the reporter, so the arm reads an unset variable"
		failures=$((failures + 1))
	fi
done
# Three wiring facts the judge cannot check for itself, because each is a
# property of the QUERY that feeds it rather than of the payload it receives. A
# judge fed the wrong data agrees with it.
# Matched on the QUERY STRING, not on the words: both of these appear in the
# comments explaining them, and a check that matched prose would pass with the
# code gone — which is the failure mode these exist to prevent, one level up.
if ! grep -qF 'gh api --paginate --slurp' "$attest"; then
	echo "FAIL: merge-attest reads check runs without --paginate --slurp. per_page caps at 100 and"
	echo "      filter=all counts every ATTEMPT, so a re-run pull request passes it — and the tail"
	echo "      that gets dropped is the OLDEST attempt, which is the one the judge reads."
	failures=$((failures + 1))
else
	echo "ok: the check-run query is paginated, so a re-run pull request does not truncate"
fi
if ! grep -qF 'check-runs?per_page=100&filter=all' "$attest"; then
	echo "FAIL: merge-attest asks for check runs without filter=all. The endpoint defaults to"
	echo "      filter=latest, which returns only the most recent attempt per check — and the judge"
	echo "      reads the OLDEST attempt precisely so a re-run after the merge cannot clear the"
	echo "      record of the merge it was absent for. The default makes that rule unenforceable."
	failures=$((failures + 1))
else
	echo "ok: the check-run query asks for every attempt, not only the latest"
fi
if ! grep -qE "^ *--jq '\\[\\[\\.\\[\\] \\| \\{number, merged_at, head_sha: \\.head\\.sha\\}\\] \\| min_by\\(\\.number\\)\\]" "$attest"; then
	echo "FAIL: merge-attest does not narrow the commit's pull requests to one before reading a head."
	echo "      The endpoint can return several for one commit, and taking the head from a different"
	echo "      element than the judge reports on means describing pull request A while judging B's"
	echo "      checks against A's merge time."
	failures=$((failures + 1))
else
	echo "ok: one pull request is chosen once, and the head comes from that same element"
fi
if ! grep -qF "needs.verdict.outputs.why != ''" "$attest"; then
	echo "FAIL: merge-attest's report job runs on a failed verdict without requiring a REASON."
	echo "      The judge exits 2 with no reason when its input is missing, and a failed API step"
	echo "      leaves the same shape — either would file the default merge-accusing issue with"
	echo "      nothing behind it."
	failures=$((failures + 1))
else
	echo "ok: the report job requires a reason, not merely a failure"
fi

if ! grep -qF 'scripts/check-merge-verdict.sh' "$attest"; then
	echo "FAIL: merge-attest.yml never runs the judge, so its verdict job cannot fail for the reason the arm reports"
	failures=$((failures + 1))
else
	echo "ok: the push-time alarm runs the judge it reports on"
fi

# --- the other half of the same invariant ---------------------------------------
#
# An arm in the reporter is only reachable if main-health both RUNS the report
# job for that lane and passes the lane's result in. Those are two lines in a
# different file, and the reporter's own test cannot see them — which is how a
# lane has been added with its arm, its env line and both cases, and still
# filed nothing: the report job's `if:` did not select it, so a failure of that
# lane alone skipped the job entirely and the arm was never reached.
#
# One invariant spelled on both sides of a wire is one item. So the census asks
# the workflow the same question it asks the reporter, and fails in the
# direction that was missed rather than only the one that was covered.
health="$root/.github/workflows/main-health.yml"
# The REPORT JOB's own block, not the whole file. Searching the file would let
# any other job's matching line stand in for the one that was removed — the
# gate would go on passing while the job it is about lost the wiring, which is
# the same "matched something, therefore fine" mistake it exists to catch.
#
# The slice runs from the job key at two-space indent to the next one; an empty
# slice means the job was renamed or removed, and that fails rather than
# silently checking nothing.
#
# The boundary matches every id GitHub Actions accepts — uppercase and digits
# included — because a boundary NARROWER than its subject is how this check
# would reacquire the hole it was written to close: a job the pattern does not
# recognise never ends the slice, so the slice swallows it and that job's wiring
# can satisfy the census on the report job's behalf.
report_job="$(awk '/^  report:/{inside=1} inside&&/^  [A-Za-z_][A-Za-z0-9_-]*:/&&!/^  report:/{exit} inside' "$health")"
if [[ -z "$report_job" ]]; then
	echo "FAIL: no 'report' job found in main-health.yml — the wiring checks below would pass by scanning nothing"
	failures=$((failures + 1))
fi
# What this loop USED to ask of the report job's `if:` was whether it selects a
# failing lane. It no longer may: the job has to run whatever the lanes said, or
# the retraction half of the reporter is unreachable. So the per-lane question is
# now about the wiring that carries a result IN, and reachability moved below,
# where it is one question per workflow rather than one per lane.
needs_list="$(grep -oE '^    needs: \[[^]]*\]' <<<"$report_job" | sed -E 's/^    needs: \[//; s/\]$//')"
if [[ -z "$needs_list" ]]; then
	echo "FAIL: main-health's report job declares no 'needs:' list, so every needs.<job>.result it passes reads empty"
	failures=$((failures + 1))
fi
for lane in $lanes; do
	# MAIN_UAT_RESULT -> uat
	job="$(printf '%s' "$lane" | sed -E 's/^MAIN_(.+)_RESULT$/\1/' | tr '[:upper:]' '[:lower:]')"
	# A job absent from `needs:` is still legal to reference: needs.<job>.result
	# simply answers with an empty string, which the reporter reads as "no
	# verdict" and passes over in silence. The arm is reached, does nothing, and
	# nothing fails — which is the shape this census exists to refuse.
	case ",${needs_list// /}," in
	*",$job,"*) ;;
	*)
		echo "FAIL: $lane has a reporter arm, but '${job}' is not in main-health's report job 'needs:' — needs.${job}.result reads empty and the arm silently does nothing"
		failures=$((failures + 1))
		;;
	esac
	if ! grep -qE "^ *${lane}: \\\$\{\{ needs\.${job}\.result \}\}" <<<"$report_job"; then
		echo "FAIL: $lane has a reporter arm, but main-health never passes needs.${job}.result in as $lane"
		failures=$((failures + 1))
	fi
done

# --- every finding that can be FILED can be RETRACTED ---------------------------
#
# Derived from the reporter rather than listed here, because a list kept beside
# the arms stops being true the day somebody adds one. A `report` with a literal
# title makes a STANDING claim about a lane's state, and a standing claim nothing
# withdraws is the defect this whole change is about: an arm added without its
# `resolve` is a one-line omission whose only symptom is an issue nobody closes.
#
# The exemption is the CODE SHAPE, not a skip-list. `report "$merge_title"` names
# one past merge and there is no state a later run could retract, so `[^"$]` is
# what excuses it — a future computed title is exempt automatically, and a future
# literal one is not.
reporter="$root/scripts/scheduled-report.sh"
reported="$(grep -oE '^  report "[^"$]+"' "$reporter" | sed -E 's/^  report "//; s/"$//' | sort -u)"
resolved="$(grep -oE '^  resolve "[^"$]+"' "$reporter" | sed -E 's/^  resolve "//; s/"$//' | sort -u)"
# A pattern that has stopped matching reads as total coverage, which is the
# loudest way this file could lie.
if [[ -z "$reported" ]]; then
	echo "FAIL: the census found no literal report title in the reporter — the pattern stopped matching, it did not stop mattering"
	failures=$((failures + 1))
fi
while IFS= read -r title; do
	[[ -z "$title" ]] && continue
	if ! grep -qxF "$title" <<<"$resolved"; then
		echo "FAIL: the reporter files \"$title\" and never retracts it — a green run leaves that issue open over a fixed tree"
		failures=$((failures + 1))
	fi
done <<<"$reported"
# And the other direction, because a retraction for a title nothing files is dead
# code that reads exactly like coverage: the check above would count it and be
# satisfied by it.
while IFS= read -r title; do
	[[ -z "$title" ]] && continue
	if ! grep -qxF "$title" <<<"$reported"; then
		echo "FAIL: the reporter retracts \"$title\" but no arm files it — either the title drifted or the arm is gone"
		failures=$((failures + 1))
	fi
done <<<"$resolved"

# --- the report job is REACHED on a green run -----------------------------------
#
# One question per workflow, because it is one fact about the job: it must run
# whatever its lanes said. Gate it on a failure again and retraction breaks in
# the silent direction — green run, skipped job, issue left open over a fixed
# tree, and no assertion anywhere fails. That is how the defect this change
# fixes went unnoticed for as long as it did.
for wf in "$health" "$root/.github/workflows/scheduled.yml"; do
	wf_name="$(basename "$wf")"
	wf_job="$(awk '/^  report:/{inside=1} inside&&/^  [A-Za-z_][A-Za-z0-9_-]*:/&&!/^  report:/{exit} inside' "$wf")"
	if [[ -z "$wf_job" ]]; then
		echo "FAIL: no 'report' job found in $wf_name — the checks below would pass by scanning nothing"
		failures=$((failures + 1))
		continue
	fi
	# The `if:` block alone: from its key to the next key at the same indent. The
	# comments explaining it sit ABOVE the key and are deliberately out of scope —
	# they name the very clause being forbidden here, so a check that read prose
	# would fail on the comment explaining why the code is right.
	cond="$(awk '/^    if:/{inside=1; print; next} inside&&/^    [a-z]/{exit} inside' <<<"$wf_job")"
	if [[ -z "$cond" ]]; then
		echo "FAIL: $wf_name's report job has no 'if:' — it either never runs, or runs on a cancelled workflow carrying results no lane produced"
		failures=$((failures + 1))
		continue
	fi
	if grep -qE 'needs[.[]' <<<"$cond"; then
		echo "FAIL: $wf_name's report job gates its 'if:' on a lane result."
		echo "      The reporter RETRACTS a finding whose check came back green, and this job is"
		echo "      skipped on exactly the run that would do it. Green run, skipped job, issue left"
		echo "      open over a fixed tree, and nothing fails."
		failures=$((failures + 1))
		continue
	fi
	if ! grep -qF '!cancelled()' <<<"$cond"; then
		echo "FAIL: $wf_name's report job does not guard on '!cancelled()', so a cancelled run reaches the reporter carrying lane results nothing produced"
		failures=$((failures + 1))
		continue
	fi
	echo "ok: $wf_name's report job is reached whatever its lanes said"
done

if [[ "$failures" -ne 0 ]]; then
	echo "FAIL: $failures case(s)" >&2
	exit 1
fi
echo "OK: scheduled-report files one issue per check"
