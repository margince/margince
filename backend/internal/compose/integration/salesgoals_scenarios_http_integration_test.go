// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Progress states where a month may finish from the forecast GET /forecast
// reads for the same month and seller. Its closed revenue is that forecast's
// won money.

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestProgressScenariosAreTheMonthsForecastAndClosedIsItsWonMoney(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	owner := signedInUser(t, e)
	now := clocktest.Now(t)
	publishEvenGoals(t, e, owner, now.Year())
	month := fmt.Sprintf("period=month&year=%d&month=%d&scope_kind=owner&scope_id=%s", now.Year(), int(now.Month()), owner)
	zone, err := time.LoadLocation(readProgress(t, e, month).Timezone)
	if err != nil {
		t.Fatal(err)
	}
	today := now.In(zone).Format(time.DateOnly)

	stages := apptest.DiscoverSeededPipeline(t, e)
	won := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Won this month", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": 300_000, "currency": "EUR", "expected_close_date": today,
	})
	mustCall(t, e, "POST", "/v1/deals/"+won+"/advance",
		AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}, http.StatusOK, nil)
	createdID(t, e, "/v1/deals", AnyMap{
		"name": "Open this month", "pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		"source": "manual", "amount_minor": 120_000, "currency": "EUR", "expected_close_date": today,
	})

	progress := readProgress(t, e, month)
	var forecast crmcontracts.ForecastReadings
	mustCall(t, e, "GET", "/v1/forecast?period=month&scope_kind=owner&scope_id="+owner, nil, http.StatusOK, &forecast)
	if forecast.Landing == nil || forecast.WonMinor < 300_000 || forecast.OpenMinor < 120_000 {
		t.Fatalf("the month's forecast = won %d, open %d, landing %+v; want the two deals in it",
			forecast.WonMinor, forecast.OpenMinor, forecast.Landing)
	}
	if progress.Closed.AmountMinor == nil || *progress.Closed.AmountMinor != forecast.WonMinor {
		t.Errorf("Progress closed %+v, the forecast won %d: one month, one seller, two answers", progress.Closed, forecast.WonMinor)
	}
	cases := progress.Scenarios
	if cases == nil {
		t.Fatalf("a running month against a set goal has no scenarios: %+v", progress)
	}
	if cases.Worst.AmountMinor != forecast.WonMinor || cases.Best.AmountMinor != forecast.WonMinor+forecast.OpenMinor {
		t.Errorf("worst %d and best %d, want the forecast's won %d and won plus open %d",
			cases.Worst.AmountMinor, cases.Best.AmountMinor, forecast.WonMinor, forecast.WonMinor+forecast.OpenMinor)
	}
	if cases.Expected.AmountMinor != forecast.Landing.AmountMinor || cases.Landing.Measure != forecast.Landing.Measure {
		t.Errorf("expected %d by %q, want the forecast landing %d by %q",
			cases.Expected.AmountMinor, cases.Landing.Measure, forecast.Landing.AmountMinor, forecast.Landing.Measure)
	}
	if want := cases.Expected.AmountMinor - monthlyGoal; cases.Expected.DeltaMinor != want {
		t.Errorf("the expected case stands %d from the goal, want %d", cases.Expected.DeltaMinor, want)
	}

	year := readProgress(t, e, fmt.Sprintf("period=year&year=%d&scope_kind=owner&scope_id=%s", now.Year(), owner))
	if year.Scenarios != nil {
		t.Errorf("a year has scenarios %+v, but the forecast has no year period", year.Scenarios)
	}
}

// publishEvenGoals publishes a goal of monthlyGoal for every month of year.
func publishEvenGoals(t *testing.T, e *apptest.AppEnv, owner string, year int) {
	t.Helper()
	plan := fmt.Sprintf("/v1/analytics/sales-goals/%s/%d", owner, year)
	total := yearlyGoal
	status, saved := planWrite(t, e, "PUT", plan+"/draft", 0,
		crmcontracts.SalesGoalDraftInput{Months: evenMonths(), DeclaredTotalMinor: &total, Reason: "Agreed in the yearly review"})
	if status != http.StatusOK {
		t.Fatalf("saving the draft → %d", status)
	}
	if status, _ := planWrite(t, e, "POST", plan+"/publish", saved.Version,
		crmcontracts.SalesGoalPublishInput{Reason: "Agreed", ConfirmAgreed: true}); status != http.StatusOK {
		t.Fatalf("publishing → %d", status)
	}
}
