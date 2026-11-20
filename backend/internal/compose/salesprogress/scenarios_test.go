// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scenarioCase is a running March for one seller against a set goal of
// 100000, read by a seat that may read the forecast.
type scenarioCase struct {
	ctx   context.Context
	frame progressFrame
	out   crmcontracts.SalesProgress
}

func runningMarch(t *testing.T, objects map[string]principal.ObjectGrant) scenarioCase {
	t.Helper()
	ctx := principal.WithActor(t.Context(), principal.Principal{
		Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: objects},
	})
	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	month := 3
	seller := openapi_types.UUID(ids.NewV7())
	goal := int64(100_000)
	return scenarioCase{
		ctx: ctx,
		frame: progressFrame{
			period: progressPeriod{kind: "month", year: 2026, month: &month}, now: start.AddDate(0, 0, 19),
			horizon: crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 1, 0)},
		},
		out: crmcontracts.SalesProgress{
			Scope: crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindOwner, Id: &seller},
			State: crmcontracts.SalesProgressStateCurrent, Closed: okAmount(60_000),
			Goal: crmcontracts.SalesProgressGoal{Status: crmcontracts.SalesProgressGoalStatusSet, AmountMinor: &goal},
		},
	}
}

var forecastReader = map[string]principal.ObjectGrant{forecastObject: {Read: true}}

// forecastOf answers 60000 won, 30000 Likely and 70000 open, and counts the reads.
func forecastOf(asked *int, err error) ForecastReader {
	return func(context.Context, pgx.Tx, time.Time, time.Time, forecasting.Scope) (Forecast, error) {
		*asked++
		readings := forecasting.Readings{WonMinor: 60_000, EvidenceMinor: 30_000, OpenMinor: 70_000}
		return Forecast{Readings: readings, Measure: forecasting.MeasureLikely}, err
	}
}

func TestARunningMonthStatesEachCaseAgainstItsGoal(t *testing.T) {
	c := runningMarch(t, forecastReader)
	asked := 0
	if err := (Reading{Forecast: forecastOf(&asked, nil)}).fillScenarios(c.ctx, nil, c.frame, &c.out); err != nil {
		t.Fatal(err)
	}
	cases := c.out.Scenarios
	if cases == nil {
		t.Fatal("a running month against a set goal has no scenarios")
	}
	for name, got := range map[string][2]int64{
		"worst":    {cases.Worst.AmountMinor, cases.Worst.DeltaMinor},
		"expected": {cases.Expected.AmountMinor, cases.Expected.DeltaMinor},
		"best":     {cases.Best.AmountMinor, cases.Best.DeltaMinor},
	} {
		want := map[string][2]int64{"worst": {60_000, -40_000}, "expected": {90_000, -10_000}, "best": {130_000, 30_000}}[name]
		if got != want {
			t.Errorf("the %s case = %v, want amount and delta %v", name, got, want)
		}
	}
}

// Each of these readings has no case to state, and asks no forecast for one.
func TestAReadingWithNoCaseToStateAsksNoForecast(t *testing.T) {
	for name, change := range map[string]func(*scenarioCase){
		"a year":                 func(c *scenarioCase) { c.frame.period = progressPeriod{kind: "year", year: 2026} },
		"an ended month":         func(c *scenarioCase) { c.out.State = crmcontracts.SalesProgressStateEnded },
		"an incomplete goal":     func(c *scenarioCase) { c.out.Goal.Status = crmcontracts.SalesProgressGoalStatusIncomplete },
		"a masked closed amount": func(c *scenarioCase) { c.out.Closed.Status = crmcontracts.SalesProgressAmountStatusWithheld },
		"a partly priced amount": func(c *scenarioCase) { c.out.Closed.Status = crmcontracts.SalesProgressAmountStatusPartial },
		"masked open deals in a month not started": func(c *scenarioCase) {
			c.out.State, c.out.Closed.Status = crmcontracts.SalesProgressStateNotStarted, crmcontracts.SalesProgressAmountStatusNotStarted
			c.out.Planned.Status = crmcontracts.SalesProgressAmountStatusPartial
		},
		"a seat that may not read the forecast": func(c *scenarioCase) {
			c.ctx = principal.WithActor(c.ctx, principal.Principal{Type: principal.PrincipalHuman})
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := runningMarch(t, forecastReader)
			change(&c)
			asked := 0
			if err := (Reading{Forecast: forecastOf(&asked, nil)}).fillScenarios(c.ctx, nil, c.frame, &c.out); err != nil {
				t.Fatal(err)
			}
			if c.out.Scenarios != nil || asked != 0 {
				t.Errorf("scenarios %+v after %d forecast read(s), want none", c.out.Scenarios, asked)
			}
		})
	}
}

// A month not started whose open deals are all visible still has cases.
func TestAMonthNotStartedWithVisibleOpenDealsHasCases(t *testing.T) {
	c := runningMarch(t, forecastReader)
	c.out.State, c.out.Closed.Status = crmcontracts.SalesProgressStateNotStarted, crmcontracts.SalesProgressAmountStatusNotStarted
	c.out.Planned.Status = crmcontracts.SalesProgressAmountStatusOk
	asked := 0
	if err := (Reading{Forecast: forecastOf(&asked, nil)}).fillScenarios(c.ctx, nil, c.frame, &c.out); err != nil {
		t.Fatal(err)
	}
	if c.out.Scenarios == nil {
		t.Fatal("a month not started with every open deal visible has no scenarios")
	}
}

// While individual performance is off the forecast refuses one colleague's
// total. Progress then states no case and still answers; any other failure is
// an error.
func TestARefusedForecastLeavesTheCasesOutAndAFailedOneFails(t *testing.T) {
	c := runningMarch(t, forecastReader)
	asked := 0
	refused := auth.IndividualPerformanceOffError{}
	if err := (Reading{Forecast: forecastOf(&asked, refused)}).fillScenarios(c.ctx, nil, c.frame, &c.out); err != nil || c.out.Scenarios != nil {
		t.Errorf("a refused forecast answers %v with scenarios %+v, want no error and none", err, c.out.Scenarios)
	}
	broken := errors.New("the forecast read failed")
	if err := (Reading{Forecast: forecastOf(&asked, broken)}).fillScenarios(c.ctx, nil, c.frame, &c.out); !errors.Is(err, broken) {
		t.Errorf("a failed forecast answers %v, want the failure", err)
	}
}
