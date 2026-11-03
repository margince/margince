// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dimensionFixture is two customers in two markets and two products of two
// kinds. Its three won deals are one split over both products, one of the
// suite alone, and one with no allocation at all.
type dimensionFixture struct {
	suite, onboarding       ids.UUID
	manufacturing, software ids.UUID
}

func seedDimensionDeals(t *testing.T, e *forecastEnv, start time.Time) dimensionFixture {
	t.Helper()
	var f dimensionFixture
	f.manufacturing = e.seedID(t, `INSERT INTO company (id, display_name, industry, source, captured_by) VALUES ($1, 'Linden Manufacturing', 'manufacturing', 'manual', 'human:x')`)
	f.software = e.seedID(t, `INSERT INTO company (id, display_name, industry, source, captured_by) VALUES ($1, 'Tern Digital', 'software', 'manual', 'human:x')`)
	for company, country := range map[ids.UUID]string{f.manufacturing: "DE", f.software: "NL"} {
		if _, err := e.owner.Exec(t.Context(), `INSERT INTO company_address_country (company_id, countries, checked_at) VALUES ($1, $2, now())`, company, []string{country}); err != nil {
			t.Fatalf("seeding the address country: %v", err)
		}
	}
	f.suite = e.seedID(t, `INSERT INTO product (id, name, unit_price_minor, currency, source, captured_by, component_kind) VALUES ($1, 'Suite', 100000, 'EUR', 'manual', 'human:x', 'software')`)
	f.onboarding = e.seedID(t, `INSERT INTO product (id, name, unit_price_minor, currency, source, captured_by, component_kind) VALUES ($1, 'Onboarding', 100000, 'EUR', 'manual', 'human:x', 'implementation')`)
	closed := start.Add(time.Hour)
	won := func(name string, amount int64, company ids.UUID) ids.UUID {
		return e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, company_id, amount_minor, currency, status, closed_at, fx_rate_to_base, fx_rate_date, expected_close_date, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'EUR', 'won', $8, 1, current_date, current_date, 'manual', 'human:x')`,
			name, e.pipeline, e.stages[pipelineTestStage], e.Rep1, company, amount, closed)
	}
	split := won("Split", 10_000, f.manufacturing)
	suiteOnly := won("Suite only", 5_000, f.software)
	won("Unallocated", 4_000, f.manufacturing)
	for _, row := range []struct {
		deal, product ids.UUID
		amount        int64
		kind          string
	}{{split, f.suite, 7_000, "software"}, {split, f.onboarding, 3_000, "implementation"}, {suiteOnly, f.suite, 5_000, "software"}} {
		if _, err := e.owner.Exec(t.Context(), `INSERT INTO deal_product (deal_id, product_id, source, amount_minor, currency, component_kind) VALUES ($1, $2, 'offer', $3, 'EUR', $4)`, row.deal, row.product, row.amount, row.kind); err != nil {
			t.Fatalf("seeding the allocation: %v", err)
		}
	}
	return f
}

func dimensionEvaluation(t *testing.T, e *forecastEnv, start time.Time, narrow func(*crmcontracts.ReportingSelection)) crmcontracts.ReportingEvaluation {
	t.Helper()
	ctx := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeAll)
	service := newReportingService(e.Pool, func() time.Time { return start.AddDate(0, 0, 2) })
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 0, 3)}
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{reportingBookingsWon, reportingBookingsMix, reportingOpenPipeline}, Blocks: []crmcontracts.ReportingBlockKind{"breakdown", "metric_reading"}}
	narrow(&selection)
	result, err := service.Evaluate(ctx, selection)
	if err != nil {
		t.Fatalf("evaluating %+v: %v", selection, err)
	}
	return result
}

// Sales won follows the product, the market and the value basis the
// selection names; the commercial mix groups the same money by kind.
func TestSalesWonFollowsTheProductMarketAndValueBasis(t *testing.T) {
	e := setupForecast(t)
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	f := seedDimensionDeals(t, e, start)

	whole := dimensionEvaluation(t, e, start, func(*crmcontracts.ReportingSelection) {})
	if got := sectionsValue(t, whole, reportingBookingsWon); got != 19_000 {
		t.Fatalf("sales won over everything is %v, want 19000", got)
	}
	mix, labels := sectionsBreakdown(t, whole, reportingBookingsMix)
	for key, want := range map[string]float64{"kind:software": 12_000, "kind:implementation": 3_000, "kind:unallocated": 4_000} {
		if mix[key] != want {
			t.Fatalf("the mix holds %v under %s, want %v (%v)", mix[key], key, want, mix)
		}
	}
	if labels["kind:unallocated"] != "Unallocated" {
		t.Fatalf("the unallocated group is labelled %q", labels["kind:unallocated"])
	}

	suite := openapi_types.UUID(f.suite)
	product := dimensionEvaluation(t, e, start, func(s *crmcontracts.ReportingSelection) { s.ProductId = &suite })
	if got := sectionsValue(t, product, reportingBookingsWon); got != 12_000 {
		t.Fatalf("suite bookings are %v, want 12000", got)
	}
	if product.Context.ProductLabel == nil || *product.Context.ProductLabel != "Suite" {
		t.Fatalf("the context names the product %v, want Suite", product.Context.ProductLabel)
	}
	if mix, _ := sectionsBreakdown(t, product, reportingBookingsMix); mix["kind:software"] != 12_000 || mix["kind:unallocated"] != 0 {
		t.Fatalf("the suite mix is %v, want 12000 of software and no remainder", mix)
	}

	industry := "manufacturing"
	byIndustry := dimensionEvaluation(t, e, start, func(s *crmcontracts.ReportingSelection) { s.Industry = &industry })
	if got := sectionsValue(t, byIndustry, reportingBookingsWon); got != 14_000 {
		t.Fatalf("manufacturing bookings are %v, want 14000", got)
	}
	country := "NL"
	byCountry := dimensionEvaluation(t, e, start, func(s *crmcontracts.ReportingSelection) { s.Country = &country })
	if got := sectionsValue(t, byCountry, reportingBookingsWon); got != 5_000 {
		t.Fatalf("NL bookings are %v, want 5000", got)
	}

	basis := crmcontracts.ReportingValueBasisSoftware
	bySoftware := dimensionEvaluation(t, e, start, func(s *crmcontracts.ReportingSelection) { s.ValueBasis = &basis })
	if got := sectionsValue(t, bySoftware, reportingBookingsWon); got != 12_000 {
		t.Fatalf("software value is %v, want 12000", got)
	}
	if open := sectionsReading(t, bySoftware, reportingOpenPipeline); open.Coverage.Status != "unsupported" {
		t.Fatalf("open pipeline under a value basis is %+v, want unsupported", open.Coverage)
	}
	if open := sectionsReading(t, product, reportingOpenPipeline); open.Coverage.Status == "unsupported" {
		t.Fatalf("open pipeline refused a product it can follow: %+v", open.Coverage)
	}
}
