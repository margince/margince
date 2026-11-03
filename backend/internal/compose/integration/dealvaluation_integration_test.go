// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var valuationDate = time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)

type valuationFixture struct {
	e         *Env
	project   ids.ProjectID
	company   ids.CompanyID
	pipeline  ids.PipelineID
	open, won ids.StageID
}

func setupValuation(t *testing.T) valuationFixture {
	t.Helper()
	e := Setup(t)
	e.Deals.WithClock(func() time.Time { return valuationDate })
	pipeline, open, won := DealFixture(t, e)
	if _, err := e.Deals.UpdateStage(e.Admin(), open, deals.UpdateStageInput{WinProbability: new(60)}); err != nil {
		t.Fatal(err)
	}
	company := e.SeedCompany(t, "Valuation company", &e.Rep1)
	project := seedProject(e.Admin(), t, e, "Mixed currency delivery", company, &e.Rep1)
	return valuationFixture{e: e, project: project.ID, company: companyIDOf(company), pipeline: pipeline, open: open, won: won}
}

func (f valuationFixture) rate(t *testing.T, currency, rate string, date time.Time) {
	t.Helper()
	f.e.Deals.WithClock(func() time.Time { return date })
	if _, err := f.e.Deals.SetFxRate(f.e.Admin(), deals.SetFxRateInput{FromCurrency: currency, Rate: rate, EffectiveDate: date}); err != nil {
		t.Fatal(err)
	}
}

func (f valuationFixture) deal(t *testing.T, currency string, amount *int64, owner ids.UUID) ids.DealID {
	t.Helper()
	var currencyInput *string
	if amount != nil {
		currencyInput = &currency
	}
	row, err := f.e.Deals.CreateDeal(f.e.Admin(), deals.CreateDealInput{
		Name: "Price in " + currency, PipelineID: f.pipeline, StageID: f.open,
		AmountMinor: amount, Currency: currencyInput, CompanyID: &f.company, ProjectID: &f.project,
		OwnerID: new(ids.From[ids.UserKind](owner)), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.DealKind](ids.UUID(row.Id))
}

func (f valuationFixture) totals(ctx context.Context, t *testing.T) deals.ProjectDealTotals {
	t.Helper()
	var totals deals.ProjectDealTotals
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		var err error
		totals, err = f.e.Deals.ProjectDealTotalsTx(ctx, tx, f.project)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return totals
}

func (f valuationFixture) header(ctx context.Context, t *testing.T) crmcontracts.Project360Rollups {
	t.Helper()
	page, err := project360Service(f.e, valuationDate).Assemble(ctx, f.project)
	if err != nil || page.Rollups == nil {
		t.Fatalf("project header unavailable: %v", err)
	}
	return *page.Rollups
}

func valuationPlan(project ids.ProjectID) string {
	return fmt.Sprintf(`{"group_by":["stage_id"],"filters":{"project_id":%q},"aggregates":[{"fn":"count","as":"deals"},{"fn":"count","field":"amount_base_minor","as":"priced_deals"},{"fn":"sum","field":"amount_base_minor","as":"base_minor"},{"fn":"sum","field":"weighted_base_minor","as":"weighted_base_minor"}]}`, project.String())
}

func TestProjectAndBoardValueTheSameMixedCurrencyDealsAsAnalytics(t *testing.T) {
	f := setupValuation(t)
	f.rate(t, "USD", "0.5", valuationDate)
	f.rate(t, "JPY", "0.006", valuationDate)
	f.deal(t, "EUR", new(int64(10001)), f.e.Rep1)
	usd := f.deal(t, "USD", new(int64(20001)), f.e.Rep1)
	f.deal(t, "JPY", new(int64(50000)), f.e.Rep1)
	f.deal(t, "GBP", new(int64(70000)), f.e.Rep1)
	const want = int64(50002)
	totals := f.totals(f.e.Admin(), t)
	if totals.OpenMinor == nil || *totals.OpenMinor != want || totals.OpenUnpriced != 1 {
		t.Fatalf("project totals = %+v, want priced %d and one unpriced", totals, want)
	}
	header := f.header(f.e.Admin(), t)
	if header.OpenDealValue.AmountMinor == nil || *header.OpenDealValue.AmountMinor != want || header.OpenDealsUnpriced == nil || *header.OpenDealsUnpriced != 1 {
		t.Fatalf("header = %+v, want %d and one unpriced", header, want)
	}
	for _, key := range []string{"deals-by-stage", "pipeline-current"} {
		rows := runPrebuiltReport(f.e.Admin(), t, f.e, key, valuationPlan(f.project))
		if len(rows) != 1 || cell(rows[0], "base_minor") != fmt.Sprint(want) || cell(rows[0], "deals") != "4" || cell(rows[0], "priced_deals") != "3" || cell(rows[0], "weighted_base_minor") != "30002" {
			t.Fatalf("%s = %v, want one shared-currency row with three priced of four deals", key, rows)
		}
	}
	rows := runPrebuiltReport(f.e.Admin(), t, f.e, "projects-by-phase", "")
	if len(rows) != 1 || cell(rows[0], "open_deal_value_minor") != fmt.Sprint(want) || cell(rows[0], "open_deal_unpriced") != "1" {
		t.Fatalf("delivery report = %v, want %d and one unpriced", rows, want)
	}
	if _, err := f.e.Deals.UpdateDeal(f.e.Admin(), usd, deals.UpdateDealInput{ForecastCategory: new("commit")}); err != nil {
		t.Fatal(err)
	}
	cards, _, err := f.e.Deals.ListDeals(f.e.Admin(), deals.ListDealsInput{ProjectID: &f.project, ForecastCategory: new("commit")})
	if err != nil || len(cards) != 1 {
		t.Fatalf("forecast-filtered cards = %d, %v", len(cards), err)
	}
	filtered := strings.Replace(valuationPlan(f.project), `"project_id":`, `"forecast_category":"commit","project_id":`, 1)
	rows = runPrebuiltReport(f.e.Admin(), t, f.e, "deals-by-stage", filtered)
	if len(rows) != 1 || cell(rows[0], "deals") != "1" || cell(rows[0], "base_minor") != "10001" {
		t.Fatalf("forecast-filtered totals disagree with their cards: %v", rows)
	}
}

func TestAnUnpricedProjectAmountIsNullWhileAnAmountlessDealIsNotAnFxGap(t *testing.T) {
	f := setupValuation(t)
	f.deal(t, "GBP", new(int64(70000)), f.e.Rep1)
	f.deal(t, "JPY", nil, f.e.Rep1)
	totals := f.totals(f.e.Admin(), t)
	if totals.OpenMinor != nil || totals.OpenUnpriced != 1 {
		t.Fatalf("totals = %+v, want null and exactly one missing price", totals)
	}
	header := f.header(f.e.Admin(), t)
	if header.OpenDealValue.AmountMinor != nil || header.OpenDealsUnpriced == nil || *header.OpenDealsUnpriced != 1 {
		t.Fatalf("unpriced header became a measured zero: %+v", header)
	}
	rows := runPrebuiltReport(f.e.Admin(), t, f.e, "projects-by-phase", "")
	if len(rows) != 1 || rows[0]["open_deal_value_minor"] != nil || cell(rows[0], "open_deal_unpriced") != "1" {
		t.Fatalf("unpriced delivery report = %v", rows)
	}
	registry := compose.NewRegistry(f.e.Pool, compose.SendPath{})
	out, err := registry.Invoke(f.e.Admin(), "read_project_360", []byte(fmt.Sprintf(`{"project_id":%q}`, f.project.String())))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data struct {
			Rollups map[string]json.RawMessage `json:"rollups"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Rollups["open_deal_value_minor"]) != 0 || string(envelope.Data.Rollups["open_deals_unpriced"]) != "1" {
		t.Fatalf("agent projection lost absence or coverage: %s", out)
	}
}

func TestWonProjectAndCompanyValuesKeepTheActualCloseTimeFreeze(t *testing.T) {
	f := setupValuation(t)
	f.rate(t, "USD", "0.5", valuationDate)
	deal := f.deal(t, "USD", new(int64(20001)), f.e.Rep1)
	f.close(t, deal)
	f.rate(t, "USD", "0.9", valuationDate.AddDate(0, 0, 1))
	rowsByStage := runPrebuiltReport(f.e.Admin(), t, f.e, "deals-by-stage", valuationPlan(f.project))
	if len(rowsByStage) != 1 || cell(rowsByStage[0], "base_minor") != "10001" || cell(rowsByStage[0], "priced_deals") != "1" {
		t.Fatalf("the won board stage lost its frozen value: %v", rowsByStage)
	}
	totals := f.totals(f.e.Admin(), t)
	if totals.WonMinor == nil || *totals.WonMinor != 10001 || totals.WonUnpriced != 0 {
		t.Fatalf("frozen won total = %+v, want 10001", totals)
	}
	rows := runPrebuiltReport(f.e.Admin(), t, f.e, "projects-by-phase", "")
	if len(rows) != 1 || cell(rows[0], "won_deal_value_minor") != "10001" || cell(rows[0], "won_deal_unpriced") != "0" {
		t.Fatalf("won project report = %v", rows)
	}
	page, err := companySurfaceService(f.e).Assemble(f.e.Admin(), f.company)
	if err != nil || page.Deals == nil || page.Deals.WonLifetime.AmountMinor == nil || *page.Deals.WonLifetime.AmountMinor != 10001 {
		t.Fatalf("company won lifetime disagrees with project: %+v, %v", page.Deals, err)
	}
}

func TestMaskedDealAmountsRevealNeitherValueNorMissingPriceInProjectTotals(t *testing.T) {
	f := setupValuation(t)
	f.deal(t, "EUR", new(int64(10000)), f.e.Rep1)
	f.deal(t, "GBP", new(int64(70000)), f.e.Rep3)
	ctx := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, projectMaskedRepPerms)
	totals := f.totals(ctx, t)
	if totals.OpenMinor == nil || *totals.OpenMinor != 10000 || totals.OpenUnpriced != 0 {
		t.Fatalf("masked totals = %+v, want only visible money and no hidden price gap", totals)
	}
	header := f.header(ctx, t)
	if header.OpenDealsUnpriced == nil || *header.OpenDealsUnpriced != 0 {
		t.Fatalf("masked header disclosed an unpriced deal: %+v", header)
	}
	if got := f.totals(f.e.Admin(), t).OpenUnpriced; got != 1 {
		t.Fatalf("unmasked coverage = %d, want 1", got)
	}
	rows := runPrebuiltReport(ctx, t, f.e, "projects-by-phase", "")
	if len(rows) != 1 || cell(rows[0], "open_deal_value_minor") != "10000" || cell(rows[0], "open_deal_unpriced") != "0" {
		t.Fatalf("masked delivery report disclosed hidden money or coverage: %v", rows)
	}
}

func (f valuationFixture) close(t *testing.T, deal ids.DealID) {
	t.Helper()
	if _, err := f.e.Deals.AdvanceDeal(f.e.Admin(), deal, deals.AdvanceDealInput{ToStageID: f.won, WonWithoutContractReason: new("other"), WonWithoutContractDetail: new("Existing written agreement outside the system")}); err != nil {
		t.Fatal(err)
	}
}

func TestALegacyWonAmountWithoutItsFreezeStillUsesTheSharedValuation(t *testing.T) {
	f := setupValuation(t)
	f.rate(t, "USD", "0.5", valuationDate)
	deal := f.deal(t, "USD", new(int64(20001)), f.e.Rep1)
	f.close(t, deal)
	// A missing historical freeze cannot be created by today's close writer.
	f.e.WsExec(t, `UPDATE deal SET amount_minor_base = NULL WHERE id = $1`, deal)
	totals := f.totals(f.e.Admin(), t)
	if totals.WonMinor == nil || *totals.WonMinor != 10001 || totals.WonUnpriced != 0 {
		t.Fatalf("legacy won amount = %+v, want the rate-priced 10001", totals)
	}
	rows := runPrebuiltReport(f.e.Admin(), t, f.e, "projects-by-phase", "")
	if len(rows) != 1 || cell(rows[0], "won_deal_value_minor") != "10001" {
		t.Fatalf("legacy delivery report = %v, want 10001", rows)
	}
}

func TestLegacyUnpriceableWinsStayUnknownAcrossProjectAndCompanyTotals(t *testing.T) {
	f := setupValuation(t)
	f.rate(t, "USD", "0.5", valuationDate)
	deal := f.deal(t, "USD", new(int64(20001)), f.e.Rep1)
	f.close(t, deal)
	// A historical close without its freeze and rate cannot be created by the current writer.
	f.e.WsExec(t, `UPDATE deal SET amount_minor_base=NULL, currency='GBP' WHERE id=$1`, deal)
	if got := f.totals(f.e.Admin(), t); got.WonMinor != nil || got.WonUnpriced != 1 {
		t.Fatalf("unpriced project = %+v", got)
	}
	page, err := companySurfaceService(f.e).Assemble(f.e.Admin(), f.company)
	if err != nil || page.Deals == nil || page.Deals.WonLifetime.AmountMinor != nil {
		t.Fatalf("unpriced company = %+v, %v", page.Deals, err)
	}
	tree, err := compose.CompanyHierarchyRollup(f.e.Admin(), f.e.Pool, f.company.UUID, "self", compose.CompanyRollupPeriod{}, fixedClock(valuationDate.Add(time.Hour)))
	if err != nil || tree.ClosedWonMinor != nil {
		t.Fatalf("unpriced company hierarchy = %+v, %v", tree, err)
	}
}
