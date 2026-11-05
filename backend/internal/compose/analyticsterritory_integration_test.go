// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The Territory example end to end. An admin adds a single-choice Territory
// field on Deal, puts it in reports, and the team fills it. Sales won then
// groups by territory with an unassigned share. A reclassification moves money
// between territories without changing the total. The forecast takes the same
// filter. Every deal is written through the deal store, at the shipped
// group floor.

import (
	"fmt"
	"maps"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type territoryBook struct {
	e        *integration.Env
	store    *deals.Store
	column   string
	pipeline ids.PipelineID
	open     ids.StageID
	won      ids.StageID
}

// newTerritoryBook creates the Territory field through the catalog writer and
// puts it in reports, as an admin does in Settings.
func newTerritoryBook(t *testing.T) territoryBook {
	t.Helper()
	e := integration.Setup(t)
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	admin := e.As(e.Rep1, nil, integration.CustomFieldAdminPerms)
	field, err := catalog.Create(admin, customfields.FieldSpec{
		Object: "deal", Label: "Territory", Type: customfields.TypePicklist,
		Options: []string{"North", "South", "West"}, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := catalog.Update(admin, ids.UUID(field.Id), customfields.FieldPatch{Reportable: &on}, nil); err != nil {
		t.Fatal(err)
	}
	pipeline, open, won := integration.DealFixture(t, e)
	return territoryBook{
		e: e, store: deals.NewStore(e.DB(), installseam.Deals()).WithFieldCatalog(catalog),
		column: *field.ColumnName, pipeline: pipeline, open: open, won: won,
	}
}

// deal opens one deal of euros in a territory ("" for none), and wins it when asked.
func (b territoryBook) deal(t *testing.T, territory string, euros int64, win bool) ids.DealID {
	t.Helper()
	amount, currency := euros*100, "EUR"
	custom := map[string]any{}
	if territory != "" {
		custom[b.column] = territory
	}
	created, err := b.store.CreateDeal(b.e.Admin(), deals.CreateDealInput{
		Name: fmt.Sprintf("%s deal", territory), PipelineID: b.pipeline, StageID: b.open,
		AmountMinor: &amount, Currency: &currency, CustomFields: custom,
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.DealKind](ids.UUID(created.Id))
	if win {
		verbal := "verbal"
		if _, err := b.store.AdvanceDeal(b.e.Admin(), id, deals.AdvanceDealInput{
			ToStageID: b.won, WonWithoutContractReason: &verbal,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// ask runs one question at the shipped floor and answers its rows keyed by
// territory, "<nil>" for deals with none.
func (b territoryBook) ask(t *testing.T, q analyticsquery.Query, measure string) map[string]int64 {
	t.Helper()
	ctx := b.e.Admin()
	var answer AnalyticsAnswer
	if err := database.WithWorkspaceTx(ctx, b.e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = RunAnalyticsQuery(ctx, tx, q, analyticsquery.DefaultFloor)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if answer.Withheld {
		t.Fatalf("the floor withheld part of %v, so the totals below describe less than the book", answer.Rows)
	}
	out := map[string]int64{}
	for _, row := range answer.Rows {
		out[fmt.Sprint(row[b.column])] = wholeNumber(t, row[measure])
	}
	return out
}

// wholeNumber reads a count or a sum as the engine answers it.
//
//craft:ignore naked-any An answer row is map[string]any, and a count and a sum arrive as different types.
func wholeNumber(t *testing.T, value any) int64 {
	t.Helper()
	if numeric, ok := value.(pgtype.Numeric); ok {
		f, err := numeric.Float64Value()
		if err != nil || !f.Valid {
			t.Fatalf("a sum %v that is no number: %v", value, err)
		}
		return int64(f.Float64)
	}
	n, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	if err != nil {
		t.Fatalf("a measure %v that is no number: %v", value, err)
	}
	return n
}

func (b territoryBook) salesWon(t *testing.T) map[string]int64 {
	t.Helper()
	return b.ask(t, analyticsquery.Query{
		Entity: "deals-by-stage", GroupBy: []string{b.column},
		Filters: []analyticsquery.Filter{
			{Field: "status", Op: analyticsquery.OpEq, Value: "won"},
			{Field: "currency", Op: analyticsquery.OpEq, Value: "EUR"},
		},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: "amount_minor", As: "won"}},
	}, "won")
}

func euros(cents map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(cents))
	for key, value := range cents {
		out[key] = value / 100
	}
	return out
}

// Sales won groups by territory with the unassigned share apart, and the
// groups add up to every won deal. Reclassifying one unassigned deal moves its
// money to North and leaves the total where it was.
func TestTheTerritoryExampleGroupsSalesWonAndReclassifies(t *testing.T) {
	b := newTerritoryBook(t)
	for range 5 {
		b.deal(t, "North", 36_000, true)
		b.deal(t, "South", 28_000, true)
		b.deal(t, "West", 16_000, true)
		b.deal(t, "", 2_000, true)
	}
	unassigned := b.deal(t, "", 10_000, true)

	want := map[string]int64{"North": 180_000, "South": 140_000, "West": 80_000, "<nil>": 20_000}
	if got := euros(b.salesWon(t)); !maps.Equal(got, want) {
		t.Fatalf("sales won by territory %v, want %v", got, want)
	}

	if _, err := b.store.UpdateDeal(b.e.Admin(), unassigned, deals.UpdateDealInput{
		CustomFields: map[string]any{b.column: "North"},
	}); err != nil {
		t.Fatalf("reclassifying the deal: %v", err)
	}
	want = map[string]int64{"North": 190_000, "South": 140_000, "West": 80_000, "<nil>": 10_000}
	got := euros(b.salesWon(t))
	if !maps.Equal(got, want) {
		t.Fatalf("after the reclassification sales won by territory %v, want %v", got, want)
	}
	total := int64(0)
	for _, value := range got {
		total += value
	}
	if total != 420_000 {
		t.Errorf("the reclassification moved the total to %d, want it to stay 420000", total)
	}
}

// The forecast is a deal population too, so a Territory filter narrows it
// rather than being ignored: North's forecast counts North's open deals.
func TestTheForecastTakesTheTerritoryFilter(t *testing.T) {
	b := newTerritoryBook(t)
	for range 5 {
		b.deal(t, "North", 10_000, false)
		b.deal(t, "South", 10_000, false)
	}
	count := func(filters []analyticsquery.Filter) int64 {
		return b.ask(t, analyticsquery.Query{
			Entity: objectForecast, Filters: filters,
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
		}, "deals")["<nil>"]
	}
	all := count(nil)
	north := count([]analyticsquery.Filter{{Field: b.column, Op: analyticsquery.OpEq, Value: "North"}})
	if all != 10 || north != 5 {
		t.Errorf("the forecast counts %d deals, %d of them in North; want 10 and 5", all, north)
	}
}
