// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The converted measure on open-deals-per-company under a field mask and in
// its evidence, and a saved question that the currency rule now refuses.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const openDealsBasePlan = `{"group_by":["owner_id"],"aggregates":[` +
	`{"fn":"sum","field":"amount_base_minor","as":"base"},` +
	`{"fn":"count","field":"amount_base_minor","as":"priced"},` +
	`{"fn":"count","as":"deals"}]}`

// The drill-through behind a converted total recomputes the same figure, and a
// deal with no rate is counted without being priced.
func TestTheConvertedTotalPerCompanyReconcilesWithItsEvidence(t *testing.T) {
	e := setupForecast(t)
	seedRate(t, e, "0.5", 1)
	seedPricedDeal(t, e, "Home", 100_000, "EUR", "open")
	seedPricedDeal(t, e, "Abroad", 10_000, "USD", "open")
	seedPricedDeal(t, e, "Unrated", 2_000, "CHF", "open")

	result := e.runReport(e.Admin(), t, "open-deals-per-company", openDealsBasePlan)
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %+v, want the one unowned group", result.Rows)
	}
	row := result.Rows[0]
	// 100000 EUR + 10000 USD × 0.5. The CHF deal has no rate, so it is not
	// priced at zero or at one: it is left out and the count says so.
	if got := wireInt(t, row, "base"); got != 105_000 {
		t.Errorf("converted total = %d, want 105000", got)
	}
	if got := wireInt(t, row, "priced"); got != 2 {
		t.Errorf("priced = %d, want 2 of the 3 deals", got)
	}
	if got := wireInt(t, row, "deals"); got != 3 {
		t.Errorf("deals = %d, want 3", got)
	}
	handle, ok := row["derivation_url"].(string)
	if !ok || handle == "" {
		t.Fatalf("the converted row minted no derivation handle: %+v", row)
	}
	derivation := e.explainReport(e.Admin(), t, "open-deals-per-company", handle)
	if got := asInt(t, derivation.Aggregates["base"]); got != 105_000 {
		t.Errorf("the evidence recomputes %d, want the 105000 it explains", got)
	}
}

// The converted measure reads the masked amount, so a mask on amount_minor
// withholds it as it withholds the native sum.
func TestAMaskOnTheAmountWithholdsTheConvertedTotal(t *testing.T) {
	e := setupForecast(t)
	own := int64(100_000)
	e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &own, nil)

	result := e.runReport(e.maskedDealReader(principal.MaskAlways), t, "open-deals-per-company", openDealsBasePlan)
	if len(result.Rows) != 0 {
		t.Errorf("rows = %+v, want none: the only deal's amount is masked", result.Rows)
	}
	if result.ExcludedByPermission == nil || *result.ExcludedByPermission != 1 {
		t.Errorf("excluded_by_permission = %v, want the 1 masked deal", result.ExcludedByPermission)
	}
}

// A question saved before the currency rule still lists, and opening it gives
// the refusal with its repair rather than a figure in no currency.
func TestASavedQuestionThatMixesCurrenciesIsListedAndRefusedWhenOpened(t *testing.T) {
	e := setupForecast(t)
	e.seedDealPricedIn(t, "Home", nil, 100_000, "EUR")
	e.seedDealPricedIn(t, "Abroad", nil, 10_000, "USD")
	asker := e.askerCtx()

	legacy := analyticsquery.Query{
		Entity:   "open-deals-per-company",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.Sum, Field: fieldAmountMinor, As: "total"}},
	}
	// The answer the engine gave before the rule: euro cents and dollar cents
	// added into one number.
	before := AnalyticsAnswer{Columns: []string{"total"}, Rows: []map[string]any{{"total": 110_000}}, TotalSafe: true}
	var id ids.UUID
	if err := forecasting.NewStore(InstallationDB(e.Pool)).InTx(asker,
		func(ctx context.Context, tx pgx.Tx) error {
			var err error
			id, err = SaveReportRun(ctx, tx, legacy, before, analyticsquery.DefaultFloor)
			return err
		}); err != nil {
		t.Fatalf("saving the legacy question: %v", err)
	}

	if listed := listSavedQuestions(asker, t, e); len(listed) != 1 || listed[0] != id.String() {
		t.Errorf("the saved list is %v, want the legacy question %s so its reader can open it", listed, id)
	}
	run, err := e.readRun(asker, t, id)
	var refusal *analyticsquery.RefusalError
	if !errors.As(err, &refusal) {
		t.Fatalf("opening the legacy question answered %+v, %v; want the currency refusal", run.Answer, err)
	}
	for _, repair := range []string{"group by currency", "amount_base_minor"} {
		if !strings.Contains(refusal.Suggest, repair) {
			t.Errorf("the refusal %q does not offer %q", refusal.Suggest, repair)
		}
	}
}
