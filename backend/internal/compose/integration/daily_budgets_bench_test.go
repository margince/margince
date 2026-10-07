// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The daily-use budgets (p95, per seat). PERF-1 and PERF-7 are declared beside
// their own benches; these are the screens only bench-daily measures.
const (
	Perf2Budget  = 150 * time.Millisecond
	Perf8Budget  = time.Second
	Perf9Budget  = 300 * time.Millisecond
	Perf10Budget = time.Second
)

// DailyVerdict is the recorded outcome of one flow for one seat; gen-perfdoc
// renders these exact strings, so a change here changes the published page.
type DailyVerdict string

const (
	DailyWithin    DailyVerdict = "within budget"
	DailyOverKnown DailyVerdict = "over budget"
	DailyOver      DailyVerdict = "over budget (unlisted)"
	DailyNoData    DailyVerdict = "no data"
	DailyNotGated  DailyVerdict = "not gated"
)

// KnownIssue ties a flow already over budget to the open issue tracking it.
type KnownIssue struct {
	Flow  string
	Issue int
}

// DailyKnownIssues lists flows already over budget, each tracked by an open
// issue; remove a row when its issue closes.
var DailyKnownIssues = []KnownIssue{
	{Flow: "worklist", Issue: 4912},
	{Flow: "palette_search_prefix", Issue: 7037},
}

// DailyResult is one flow measured for one seat, as the gate judges it.
type DailyResult struct {
	Flow, Seat string
	Verdict    DailyVerdict
	Issue      int
	Status5xx  int
	Status422  int
	// Allow422 marks a flow whose query_too_broad refusal is the known issue itself.
	Allow422 bool
}

const dailyScaleEnv = "MARGINCE_BENCH_DAILY_SCALE"

func knownIssueFor(flow string) int {
	for _, known := range DailyKnownIssues {
		if known.Flow == flow {
			return known.Issue
		}
	}
	return 0
}

// JudgeDaily returns the verdict for one flow's p95 and, when it is over budget
// and listed, the issue that tracks it. A p95 equal to the budget is over, as
// gen-perfdoc renders it.
func JudgeDaily(flow string, p95, budget time.Duration, samples int) (DailyVerdict, int) {
	switch {
	case samples == 0:
		return DailyNoData, 0
	case p95 < budget:
		return DailyWithin, 0
	}
	if issue := knownIssueFor(flow); issue != 0 {
		return DailyOverKnown, issue
	}
	return DailyOver, 0
}

// DailyGate fails the run on every unlisted breach, every 5xx and every 422
// outside a listed flow. At the published scale (1) it also fails a listed flow
// now within budget for every measured seat, so the table cannot go stale.
func DailyGate(results []DailyResult, scale float64) error {
	var problems []string
	for _, r := range results {
		if r.Verdict == DailyOver {
			problems = append(problems, fmt.Sprintf("flow %s is over budget for %s and no open issue lists it", r.Flow, r.Seat))
		}
		if r.Status5xx > 0 {
			problems = append(problems, fmt.Sprintf("flow %s answered %d server errors for %s", r.Flow, r.Status5xx, r.Seat))
		}
		if r.Status422 > 0 && !r.Allow422 {
			problems = append(problems, fmt.Sprintf("flow %s was refused %d times with 422 for %s", r.Flow, r.Status422, r.Seat))
		}
	}
	if scale == 1 {
		problems = append(problems, staleKnownIssues(results)...)
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

// staleKnownIssues names each listed flow that was measured and came in within
// budget for every seat; a seat with no data or no gate says nothing either way.
func staleKnownIssues(results []DailyResult) []string {
	var stale []string
	for _, known := range DailyKnownIssues {
		measured, within := 0, 0
		for _, r := range results {
			if r.Flow != known.Flow || r.Verdict == DailyNoData || r.Verdict == DailyNotGated {
				continue
			}
			measured++
			if r.Verdict == DailyWithin {
				within++
			}
		}
		if measured > 0 && within == measured {
			stale = append(stale, fmt.Sprintf("flow %s is within budget for every seat: remove the row for #%d", known.Flow, known.Issue))
		}
	}
	return stale
}

// dailyScale reads the corpus multiplier; 1 is the published mid-market tier.
// A value that is not a positive number is an error, never a silent default.
func dailyScale() (float64, error) {
	raw := os.Getenv(dailyScaleEnv)
	if raw == "" {
		return 1, nil
	}
	scale, err := strconv.ParseFloat(raw, 64)
	if err != nil || scale <= 0 {
		return 0, fmt.Errorf("%s=%q: set a positive number (1 for the published tier, 0.05 for a development run)", dailyScaleEnv, raw)
	}
	return scale, nil
}

func TestADailyFlowWithinItsBudgetPasses(t *testing.T) {
	v, issue := JudgeDaily("lists", 90*time.Millisecond, Perf2Budget, 30)
	if v != DailyWithin || issue != 0 {
		t.Fatalf("got %q #%d, want within budget and no issue", v, issue)
	}
}

func TestAKnownSlowFlowIsRecordedAgainstItsIssueAndPasses(t *testing.T) {
	v, issue := JudgeDaily("worklist", 3*time.Second, Perf8Budget, 30)
	if v != DailyOverKnown || issue != 4912 {
		t.Fatalf("got %q #%d, want over budget against #4912", v, issue)
	}
	if err := DailyGate([]DailyResult{{Flow: "worklist", Seat: "rep", Verdict: v, Issue: issue}}, 1); err != nil {
		t.Fatalf("a listed over-budget flow must not fail the run: %v", err)
	}
}

func TestAnUnlistedSlowFlowFailsTheRunNamingTheFlow(t *testing.T) {
	v, _ := JudgeDaily("record_open", 400*time.Millisecond, 100*time.Millisecond, 30)
	err := DailyGate([]DailyResult{{Flow: "record_open", Seat: "manager", Verdict: v}}, 1)
	if v != DailyOver || err == nil || !strings.Contains(err.Error(), "record_open") {
		t.Fatalf("got %q, err %v; want an unlisted breach that fails and names the flow", v, err)
	}
}

func TestAFlowWithNoSamplesIsNoDataNotAPass(t *testing.T) {
	if v, _ := JudgeDaily("home_digest", 0, Perf8Budget, 0); v != DailyNoData {
		t.Fatalf("got %q, want no data", v)
	}
}

func TestAServerErrorFailsEvenWithinBudget(t *testing.T) {
	err := DailyGate([]DailyResult{{Flow: "contact_360", Seat: "rep", Verdict: DailyWithin, Status5xx: 1}}, 1)
	if err == nil || !strings.Contains(err.Error(), "contact_360") {
		t.Fatalf("got %v; a 5xx must fail the run and name the flow", err)
	}
}

func TestA422IsAllowedOnlyWhereListed(t *testing.T) {
	ok := DailyGate([]DailyResult{{Flow: "palette_search_prefix", Seat: "rep", Verdict: DailyOverKnown, Issue: 7037, Status422: 3, Allow422: true}}, 1)
	bad := DailyGate([]DailyResult{{Flow: "results_search", Seat: "rep", Verdict: DailyWithin, Status422: 1}}, 1)
	if ok != nil || bad == nil || !strings.Contains(bad.Error(), "results_search") {
		t.Fatalf("listed 422 err=%v (want nil); unlisted 422 err=%v (want an error naming the flow)", ok, bad)
	}
}

func TestAListedFlowBackUnderBudgetForEverySeatAsksForItsRowToGo(t *testing.T) {
	err := DailyGate([]DailyResult{
		{Flow: "worklist", Seat: "rep", Verdict: DailyWithin},
		{Flow: "worklist", Seat: "manager", Verdict: DailyWithin},
		{Flow: "worklist", Seat: "cold", Verdict: DailyNotGated},
	}, 1)
	want := "flow worklist is within budget for every seat: remove the row for #4912"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v; want %q", err, want)
	}
}

func TestAListedFlowStillOverForOneSeatKeepsItsRow(t *testing.T) {
	err := DailyGate([]DailyResult{
		{Flow: "worklist", Seat: "rep", Verdict: DailyWithin},
		{Flow: "worklist", Seat: "manager", Verdict: DailyOverKnown, Issue: 4912},
	}, 1)
	if err != nil {
		t.Fatalf("a flow still over for the manager must keep its row: %v", err)
	}
}

func TestAListedFlowWithNoMeasuredSeatKeepsItsRow(t *testing.T) {
	if err := DailyGate([]DailyResult{{Flow: "worklist", Seat: "rep", Verdict: DailyNoData}}, 1); err != nil {
		t.Fatalf("a flow nobody measured says nothing about its issue: %v", err)
	}
}

func TestADevelopmentScaleRunNeverAsksForARowToGo(t *testing.T) {
	err := DailyGate([]DailyResult{{Flow: "worklist", Seat: "rep", Verdict: DailyWithin}}, 0.05)
	if err != nil {
		t.Fatalf("a development-scale corpus cannot clear a known issue: %v", err)
	}
}

func TestTheDailyScaleDefaultsToThePublishedTier(t *testing.T) {
	t.Setenv(dailyScaleEnv, "")
	if scale, err := dailyScale(); err != nil || scale != 1 {
		t.Fatalf("got %v, %v; want 1 and no error", scale, err)
	}
	t.Setenv(dailyScaleEnv, "0.05")
	if scale, err := dailyScale(); err != nil || scale != 0.05 {
		t.Fatalf("got %v, %v; want 0.05 and no error", scale, err)
	}
}

func TestTheDailyScaleRefusesAValueThatSeedsNothing(t *testing.T) {
	for _, raw := range []string{"0", "-1", "half"} {
		t.Setenv(dailyScaleEnv, raw)
		if _, err := dailyScale(); err == nil || !strings.Contains(err.Error(), dailyScaleEnv) {
			t.Fatalf("%s=%q: got %v; want an error naming the variable", dailyScaleEnv, raw, err)
		}
	}
}
