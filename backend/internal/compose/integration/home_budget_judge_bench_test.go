// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"strings"
	"testing"
	"time"
)

func homeRow(row string, p95, budget time.Duration, samples int) DailyResult {
	return DailyResult{Flow: "home_warm", Row: row, P95: p95, Budget: budget, Samples: samples}
}

func TestAHomeReadUnderItsBudgetPasses(t *testing.T) {
	results := []DailyResult{homeRow("home_warm_brief", 249*time.Millisecond, homeWarmBudget, 30)}
	if failures := HomeBudgetFailures(results); len(failures) != 0 {
		t.Fatalf("a read under budget failed: %v", failures)
	}
}

func TestAHomeReadAtExactlyItsBudgetFails(t *testing.T) {
	results := []DailyResult{homeRow("home_warm_brief", homeWarmBudget, homeWarmBudget, 30)}
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
	slow := DailyResult{Flow: "home", Row: "home_worklist", P95: 2 * time.Second, Budget: homeWarmBudget, Samples: 30}
	if failures := HomeBudgetFailures([]DailyResult{slow}); len(failures) != 1 {
		t.Fatalf("a row DailyKnownIssues lists must still fail the Home check, got %v", failures)
	}
}

func TestAServerErrorOrARefusalFailsAHomeReadWithinBudget(t *testing.T) {
	errored := homeRow("home_warm_worklist", time.Millisecond, homeWarmBudget, 30)
	errored.Status5xx = 1
	refused := homeRow("home_warm_magic", time.Millisecond, homeWarmBudget, 30)
	refused.Status422 = 2
	failures := HomeBudgetFailures([]DailyResult{errored, refused})
	if len(failures) != 2 || !strings.Contains(failures[0], "server errors") || !strings.Contains(failures[1], "422") {
		t.Fatalf("a 5xx and a 422 must each fail, got %v", failures)
	}
}

func TestEveryRequiredHomeReadIsJudgedInBothPasses(t *testing.T) {
	calls := homeCalls(dailyInput{})
	measured := func(name string, samples int) dailyRow {
		return dailyRow{Measurement: BudgetMeasurement{Name: name}, Result: DailyResult{Row: name, P95: time.Millisecond, Budget: time.Second, Samples: samples}}
	}
	// The cold brief is missing entirely; the optional digest has nothing behind it.
	rows := []dailyRow{measured("home_cold_digest", 0), measured("home_warm_digest", 0)}
	for _, call := range calls {
		if call.Name != "digest" {
			rows = append(rows, measured(dailyRowName("home_warm", call), 30))
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
