// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// strictHomeCohort is one row and seat the strict Home lane must find measured,
// and the budget it answers to.
type strictHomeCohort struct {
	requirement, row, seat string
	budget                 time.Duration
}

// strictHomeCohorts are Home's acceptance targets. They are stricter than the
// published PERF rows and never consult DailyKnownIssues.
var strictHomeCohorts = []strictHomeCohort{
	{"R02", "worklist", "rep", 750 * time.Millisecond},
	{"R02", "worklist", "manager", 750 * time.Millisecond},
	{"R02", "home_worklist", "rep", 750 * time.Millisecond},
	{"R02", "home_worklist", "manager", 750 * time.Millisecond},
	{"R04", "app_shell_me", "rep", 150 * time.Millisecond},
	{"R04", "app_shell_me", "manager", 150 * time.Millisecond},
	{"R04", "morning_load_cheap_route", "team", 150 * time.Millisecond},
	{"R05", "home_brief", "rep", 200 * time.Millisecond},
	{"R05", "home_handled", "rep", 200 * time.Millisecond},
	{"R05", "home_handled", "manager", 200 * time.Millisecond},
	{"R05", "home_magic", "rep", 200 * time.Millisecond},
	{"R05", "home_magic", "manager", 200 * time.Millisecond},
	{"R06", "admin_rail_ai_usage", "admin", 100 * time.Millisecond},
	{"R06", "admin_rail_embed_reindex_identities", "admin", 100 * time.Millisecond},
}

// JudgeStrictHome judges every strict cohort against the run's results. A
// cohort passes only when it was measured, every round's p95 is under its
// budget, and no answer was a server error, a refusal or missing a source.
func JudgeStrictHome(results []DailyResult) []StrictVerdict {
	verdicts := make([]StrictVerdict, 0, len(strictHomeCohorts))
	for _, c := range strictHomeCohorts {
		v := StrictVerdict{Requirement: c.requirement, Name: c.row, Seat: c.seat, BudgetMs: ms(c.budget)}
		at := slices.IndexFunc(results, func(r DailyResult) bool { return r.Row == c.row && r.Seat == c.seat })
		if at < 0 {
			v.Failure = "not measured"
			verdicts = append(verdicts, v)
			continue
		}
		r := results[at]
		worst := max(r.P95, r.FirstP95)
		v.P95Ms, v.Samples = ms(worst), r.Samples
		v.Failure = strictFailure(r, worst, c.budget)
		v.Pass = v.Failure == ""
		verdicts = append(verdicts, v)
	}
	return verdicts
}

// strictFailure names the first reason a measured row fails, or "" when it passes.
func strictFailure(r DailyResult, worst, budget time.Duration) string {
	switch {
	case r.Verdict == DailyNoData || r.Samples == 0:
		return "no data"
	case r.Status5xx > 0:
		return fmt.Sprintf("%d server errors", r.Status5xx)
	case r.Status422 > 0:
		return fmt.Sprintf("%d refused (422)", r.Status422)
	case r.Incomplete > 0:
		return fmt.Sprintf("%d answers named an unavailable source", r.Incomplete)
	case worst >= budget:
		if r.FirstP95 > r.P95 {
			return fmt.Sprintf("p95 %d ms on the first measurement, over %d ms", worst.Milliseconds(), budget.Milliseconds())
		}
		return fmt.Sprintf("p95 %d ms, over %d ms", worst.Milliseconds(), budget.Milliseconds())
	}
	return ""
}

// strictHomeSummary is the lane's one-line answer for the run log.
func strictHomeSummary(verdicts []StrictVerdict) string {
	var failed []string
	for _, v := range verdicts {
		if !v.Pass {
			failed = append(failed, v.Name+" "+v.Seat+": "+v.Failure)
		}
	}
	if len(failed) == 0 {
		return fmt.Sprintf("strict Home acceptance PASS on %d cohorts (exploratory: %d samples each)", len(verdicts), dailySamples)
	}
	return fmt.Sprintf("strict Home acceptance FAIL on %d of %d cohorts: %s", len(failed), len(verdicts), strings.Join(failed, "; "))
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// unavailableSources counts the sources a 200 answer says it could not read.
// Such an answer is fast because it is partial, so the strict lane fails it.
func unavailableSources(body []byte) int {
	var answer struct {
		SourcesUnavailable []json.RawMessage `json:"sources_unavailable"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return 0
	}
	return len(answer.SourcesUnavailable)
}

func strictCohortResults() []DailyResult {
	var results []DailyResult
	for _, c := range strictHomeCohorts {
		results = append(results, DailyResult{Row: c.row, Seat: c.seat, Verdict: DailyWithin, Samples: dailySamples, P95: c.budget / 2, Budget: c.budget})
	}
	return results
}

func TestEveryStrictCohortUnderBudgetPasses(t *testing.T) {
	for _, v := range JudgeStrictHome(strictCohortResults()) {
		if !v.Pass {
			t.Errorf("%s %s failed with %q; a measured row at half its budget passes", v.Name, v.Seat, v.Failure)
		}
	}
}

func TestAKnownIssueDoesNotPassTheStrictLane(t *testing.T) {
	results := strictCohortResults()
	results[0].Verdict, results[0].Issue, results[0].P95 = DailyOverKnown, 4912, 2900*time.Millisecond
	got := JudgeStrictHome(results)[0]
	if got.Pass || got.Failure != "p95 2900 ms, over 750 ms" {
		t.Fatalf("got pass=%v %q; a row its known issue excuses still fails the strict lane", got.Pass, got.Failure)
	}
}

func TestASecondMeasurementCannotEraseAStrictBreach(t *testing.T) {
	results := strictCohortResults()
	results[0].P95, results[0].FirstP95 = 400*time.Millisecond, 900*time.Millisecond
	got := JudgeStrictHome(results)[0]
	if got.Pass || got.P95Ms != 900 || !strings.Contains(got.Failure, "first measurement") {
		t.Fatalf("got pass=%v p95=%v %q; the breached first round decides", got.Pass, got.P95Ms, got.Failure)
	}
}

func TestAMissingOrEmptyCohortFailsTheStrictLane(t *testing.T) {
	results := strictCohortResults()
	results = slices.DeleteFunc(results, func(r DailyResult) bool { return r.Row == "home_brief" })
	results[0].Verdict, results[0].Samples = DailyNoData, 0
	verdicts := JudgeStrictHome(results)
	brief := verdicts[slices.IndexFunc(verdicts, func(v StrictVerdict) bool { return v.Name == "home_brief" })]
	if verdicts[0].Pass || verdicts[0].Failure != "no data" || brief.Pass || brief.Failure != "not measured" {
		t.Fatalf("no data %+v, missing %+v; neither may pass", verdicts[0], brief)
	}
}

func TestAFastPartialAnswerFailsTheStrictLane(t *testing.T) {
	for name, mutate := range map[string]func(*DailyResult){
		"incomplete": func(r *DailyResult) { r.Incomplete = 3 },
		"5xx":        func(r *DailyResult) { r.Status5xx = 1 },
		"422":        func(r *DailyResult) { r.Status422 = 1 },
	} {
		results := strictCohortResults()
		mutate(&results[0])
		if got := JudgeStrictHome(results)[0]; got.Pass {
			t.Errorf("%s: a fast answer that is not a whole answer passed", name)
		}
	}
}

func TestUnavailableSourcesReadsTheWorklistsOwnField(t *testing.T) {
	for body, want := range map[string]int{
		`{"items":[],"sources_unavailable":["approval","task"]}`: 2,
		`{"items":[],"sources_unavailable":[]}`:                  0,
		`{"items":[]}`:                                           0,
		`[]`:                                                     0,
	} {
		if got := unavailableSources([]byte(body)); got != want {
			t.Errorf("unavailableSources(%s) = %d, want %d", body, got, want)
		}
	}
}
