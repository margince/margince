// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func homeRow(row string, p95, budget time.Duration, samples int) DailyResult {
	flow := homeWarmFlow.Name
	if strings.HasPrefix(row, homeColdFlow.Name) {
		flow = homeColdFlow.Name
	}
	return DailyResult{Flow: flow, Row: row, P95: p95, Budget: budget, Samples: samples}
}

func TestAHomeReadUnderItsBudgetPasses(t *testing.T) {
	results := []DailyResult{homeRow("home_warm_brief", 249*time.Millisecond, homeWarmBudget, dailySamples)}
	if failures := HomeBudgetFailures(results); len(failures) != 0 {
		t.Fatalf("a read under budget failed: %v", failures)
	}
}

func TestAHomeReadAtExactlyItsBudgetFails(t *testing.T) {
	results := []DailyResult{homeRow("home_warm_brief", homeWarmBudget, homeWarmBudget, dailySamples)}
	if failures := HomeBudgetFailures(results); len(failures) != 1 || !strings.Contains(failures[0], "at or over") {
		t.Fatalf("a p95 equal to the budget must fail, got %v", failures)
	}
}

func TestAColdHomeReadOverFiveHundredMillisecondsFails(t *testing.T) {
	results := []DailyResult{homeRow("home_cold_magic", 501*time.Millisecond, homeColdBudget, homeColdRounds)}
	if failures := HomeBudgetFailures(results); len(failures) != 1 || !strings.Contains(failures[0], "home_cold_magic") {
		t.Fatalf("a cold read over 500 ms must fail naming its row, got %v", failures)
	}
}

func TestAHomeReadWithNoSamplesFailsRatherThanPasses(t *testing.T) {
	results := []DailyResult{homeRow("home_warm_handled", 0, homeWarmBudget, 0)}
	if failures := HomeBudgetFailures(results); len(failures) != 1 || !strings.Contains(failures[0], "no samples") {
		t.Fatalf("a read with no samples must fail, got %v", failures)
	}
	if failures := HomeBudgetFailures(nil); len(failures) != 1 {
		t.Fatalf("a run that measured nothing must fail, got %v", failures)
	}
}

func TestAKnownIssueNeverExcusesASlowHomeRead(t *testing.T) {
	// The daily bench's own row for the Home worklist, which DailyKnownIssues may list.
	slow := DailyResult{Flow: homeWarmFlow.Name, Row: "home_worklist", P95: 2 * time.Second, Budget: homeWarmBudget, Samples: dailySamples}
	if failures := HomeBudgetFailures([]DailyResult{slow}); len(failures) != 1 {
		t.Fatalf("a row DailyKnownIssues lists must still fail the Home check, got %v", failures)
	}
}

func TestAServerErrorOrARefusalFailsAHomeReadWithinBudget(t *testing.T) {
	errored := homeRow("home_warm_worklist", time.Millisecond, homeWarmBudget, dailySamples)
	errored.Status5xx = 1
	refused := homeRow("home_warm_magic", time.Millisecond, homeWarmBudget, dailySamples)
	refused.Status422 = 2
	failures := HomeBudgetFailures([]DailyResult{errored, refused})
	if len(failures) != 2 || !strings.Contains(failures[0], "server errors") || !strings.Contains(failures[1], "422") {
		t.Fatalf("a 5xx and a 422 must each fail, got %v", failures)
	}
}

func TestEveryRequiredHomeReadIsJudgedInBothPasses(t *testing.T) {
	calls := homeCalls(dailyInput{})
	measured := func(name string, samples int) dailyRow {
		result := homeRow(name, time.Millisecond, time.Second, samples)
		return dailyRow{Measurement: BudgetMeasurement{Name: name}, Result: result}
	}
	// The cold brief is missing entirely; the optional digest has nothing behind it.
	rows := []dailyRow{measured("home_cold_digest", 0), measured("home_warm_digest", 0)}
	for _, call := range calls {
		if call.Name != "digest" {
			rows = append(rows, measured(dailyRowName("home_warm", call), dailySamples))
			if call.Name != "brief" {
				rows = append(rows, measured(dailyRowName("home_cold", call), homeColdRounds))
			}
		}
	}
	required := homeRequired(rows, calls)
	if want := 2 * (len(calls) - 1); len(required) != want {
		t.Fatalf("judged %d rows, want %d: every required read in both passes", len(required), want)
	}
	failures := HomeBudgetFailures(required)
	if len(failures) != 1 || !strings.Contains(failures[0], "home_cold_brief: no samples") {
		t.Fatalf("a required read with no row must fail as unsampled, got %v", failures)
	}
}

func TestAHomeRowShortOfItsPlannedSamplesFails(t *testing.T) {
	short := []DailyResult{
		homeRow("home_cold_magic", time.Millisecond, homeColdBudget, homeColdRounds-1),
		homeRow("home_warm_magic", time.Millisecond, homeWarmBudget, dailySamples-1),
	}
	failures := HomeBudgetFailures(short)
	if len(failures) != 2 || !strings.Contains(failures[0], fmt.Sprintf("want %d", homeColdRounds)) ||
		!strings.Contains(failures[1], fmt.Sprintf("want %d", dailySamples)) {
		t.Fatalf("a row short of its pass's samples must fail, got %v", failures)
	}
}

func TestARowFromNeitherHomePassFails(t *testing.T) {
	stray := DailyResult{Flow: "home", Row: "home_brief", P95: time.Millisecond, Budget: homeWarmBudget, Samples: dailySamples}
	if failures := HomeBudgetFailures([]DailyResult{stray}); len(failures) != 1 {
		t.Fatalf("a row neither pass measured must fail, got %v", failures)
	}
}

func TestAnEmptyHomeReadsListFails(t *testing.T) {
	timed := callNames(homeCalls(dailyInput{}))
	for _, source := range []string{
		"export const HOME_READS = [] as const;",
		"export const HOME_READS = [ ] as const;",
	} {
		if gaps := homeReadsGaps([]byte(source), timed); len(gaps) != 1 || !strings.Contains(gaps[0], "empty") {
			t.Errorf("%s: want one gap naming it empty, got %v", source, gaps)
		}
	}
	if gaps := homeReadsGaps([]byte(`export const HOME_READS = ["brief", "magic"] as const;`), timed); len(gaps) != 0 {
		t.Errorf("reads homeCalls times reported gaps: %v", gaps)
	}
	if gaps := homeReadsGaps([]byte(`export const HOME_READS = ["brief", "deals"] as const;`), timed); len(gaps) != 1 {
		t.Errorf("a read homeCalls does not time must be named, got %v", gaps)
	}
}

func TestTheWarmUpWaitsOnlyForStatementsStillTryingCustomPlans(t *testing.T) {
	on := func(pid uint32, name string) preparedStatement { return preparedStatement{pid: pid, name: name} }
	before := map[preparedStatement]planCount{
		on(7, "trying"):    {custom: 2},
		on(7, "weighing"):  {custom: 4},
		on(7, "custom"):    {custom: 5},
		on(7, "forced"):    {generic: 1},
		on(7, "idle"):      {custom: 1},
		on(7, "elsewhere"): {custom: 5},
		on(8, "elsewhere"): {custom: 4},
	}
	after := map[preparedStatement]planCount{
		on(7, "trying"):    {custom: 3},
		on(7, "weighing"):  {custom: 5},
		on(7, "custom"):    {custom: 6},
		on(7, "forced"):    {generic: 2},
		on(7, "idle"):      {custom: 1},
		on(7, "new"):       {custom: 1},
		on(7, "elsewhere"): {custom: 6},
		on(8, "elsewhere"): {custom: 4},
	}
	var named []string
	for _, s := range inCustomPlanTrials(before, after) {
		named = append(named, fmt.Sprintf("%d/%s", s.pid, s.name))
	}
	slices.Sort(named)
	// The fifth custom plan still trying, and a statement whose SQL ran only on
	// the other connection, are both still in their trials.
	if want := []string{"7/new", "7/trying", "7/weighing", "8/elsewhere"}; !slices.Equal(named, want) {
		t.Fatalf("statements in their custom-plan trials = %v, want %v", named, want)
	}
}
