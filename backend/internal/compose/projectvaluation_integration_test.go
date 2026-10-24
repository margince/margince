// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestProjectCurrencyDerivationKeepsTheSamePricesAndCoverage(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	date := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	e.Deals.WithClock(func() time.Time { return date })
	if err := e.Deals.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	pipeline, err := e.Deals.DefaultPipeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var open ids.StageID
	for _, stage := range *pipeline.Stages {
		if stage.Semantic == "open" {
			open = ids.From[ids.StageKind](ids.UUID(stage.Id))
			break
		}
	}
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Delivery account", &e.Rep1))
	project, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: "Currency delivery", CompanyID: company, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	projectID := ids.From[ids.ProjectKind](ids.UUID(project.Id))
	if _, err := e.Deals.SetFxRate(ctx, deals.SetFxRateInput{FromCurrency: "USD", Rate: "0.5", EffectiveDate: date}); err != nil {
		t.Fatal(err)
	}
	for _, currency := range []string{"EUR", "USD", "GBP"} {
		if _, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
			Name: currency + " delivery", PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), StageID: open,
			AmountMinor: new(int64(10000)), Currency: &currency, CompanyID: &company, ProjectID: &projectID, Source: "manual",
		}); err != nil {
			t.Fatal(err)
		}
	}
	engine := newReportEngine(e.Pool)
	outcome, err := engine.Run(ctx, "projects-by-phase", reportRequest{})
	if err != nil || len(outcome.Rows) != 1 {
		t.Fatalf("report = %+v, %v", outcome, err)
	}
	detail, err := engine.Derive(ctx, "projects-by-phase", derivationQuery{
		GroupBy: outcome.GroupBy, Aggregates: outcome.Aggregates,
		AsOf: outcome.GeneratedAt, BaseCurrency: outcome.BaseCurrency,
	})
	if err != nil || len(detail.Rows) != 1 {
		t.Fatalf("drill-through = %+v, %v", detail, err)
	}
	for key, want := range map[string]string{measureOpenDealValue: "15000", measureOpenDealUnpriced: "1"} {
		if got := valuationJSON(t, outcome.Rows[0], key); got != want {
			t.Errorf("report %s = %s, want %s", key, got, want)
		}
		if got := valuationJSON(t, detail.Rows[0], key); got != want {
			t.Errorf("drill-through %s = %s, want %s", key, got, want)
		}
	}
	if _, err := engine.Run(ctx, "deals-by-stage", reportRequest{GroupBy: []string{fieldStageID}, Aggregates: []reportAggregate{{Fn: aggFnSum, Field: fieldAmountMinor, As: "native"}}}); err == nil {
		t.Error("native money was summed across currencies without its currency dimension")
	}
	perms := principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeOwn,
		Objects: map[string]principal.ObjectGrant{"deal": {Read: true}, "installation_settings": {Read: true}},
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	if _, err := engine.Run(rep, "deals-by-stage", reportRequest{Filters: map[string]any{fieldOwnerID: e.Rep3.String()}}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep measured a colleague's owner dial: %v", err)
	}
}

func valuationJSON(t *testing.T, row map[string]any, key string) string {
	t.Helper()
	wire, err := json.Marshal(row[key])
	if err != nil {
		t.Fatal(err)
	}
	return string(wire)
}

func TestProjectMoneyDistinguishesAnEmptyPopulationFromUnpricedAmounts(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	if err := e.Deals.SeedDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	pipeline, err := e.Deals.DefaultPipeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var open ids.StageID
	for _, stage := range *pipeline.Stages {
		if stage.Semantic == "open" {
			open = ids.From[ids.StageKind](ids.UUID(stage.Id))
			break
		}
	}
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Coverage account", &e.Rep1))
	var projectID ids.ProjectID
	for _, name := range []string{"Coverage delivery", "Empty delivery"} {
		row, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{Name: name, CompanyID: company, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if name == "Coverage delivery" {
			projectID = ids.From[ids.ProjectKind](ids.UUID(row.Id))
		}
	}
	engine := newReportEngine(e.Pool)
	for _, step := range []struct {
		name, currency, value, unpriced string
		amount                          *int64
	}{
		{name: "empty", value: "0", unpriced: "0"},
		{name: "amountless", value: "0", unpriced: "0"},
		{name: "unpriced beside empty", currency: "GBP", amount: new(int64(10000)), value: "null", unpriced: "1"},
		{name: "partial priced", currency: "EUR", amount: new(int64(15000)), value: "15000", unpriced: "1"},
	} {
		t.Run(step.name, func(t *testing.T) {
			if step.name != "empty" {
				var currency *string
				if step.amount != nil {
					currency = &step.currency
				}
				if _, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: step.name, PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), StageID: open, AmountMinor: step.amount, Currency: currency, CompanyID: &company, ProjectID: &projectID, Source: "manual"}); err != nil {
					t.Fatal(err)
				}
			}
			var totals deals.ProjectDealTotals
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				var err error
				totals, err = e.Deals.ProjectDealTotalsTx(ctx, tx, projectID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			header := map[string]any{measureOpenDealValue: totals.OpenMinor, measureWonDealValue: totals.WonMinor}
			outcome, err := engine.Run(ctx, "projects-by-phase", reportRequest{})
			if err != nil || len(outcome.Rows) != 1 {
				t.Fatalf("report = %+v, %v", outcome, err)
			}
			detail, err := engine.Derive(ctx, "projects-by-phase", derivationQuery{GroupBy: outcome.GroupBy, Aggregates: outcome.Aggregates, AsOf: outcome.GeneratedAt, BaseCurrency: outcome.BaseCurrency})
			if err != nil || len(detail.Rows) != 2 {
				t.Fatalf("derivation = %+v, %v", detail, err)
			}
			for key, want := range map[string]string{measureOpenDealValue: step.value, measureWonDealValue: "0"} {
				for label, row := range map[string]map[string]any{"header": header, "report": outcome.Rows[0]} {
					if got := valuationJSON(t, row, key); got != want {
						t.Errorf("%s %s = %s, want %s", label, key, got, want)
					}
				}
			}
			if got := valuationJSON(t, outcome.Rows[0], measureOpenDealUnpriced); got != step.unpriced {
				t.Errorf("coverage = %s, want %s", got, step.unpriced)
			}
			for _, row := range detail.Rows {
				want := "0"
				if row["id"] == projectID.String() {
					want = step.value
				}
				if got := valuationJSON(t, row, measureOpenDealValue); got != want {
					t.Errorf("detail %+v open = %s, want %s", row, got, want)
				}
				if got := valuationJSON(t, row, measureWonDealValue); got != "0" {
					t.Errorf("no-won detail = %s", got)
				}
			}
		})
	}
}
