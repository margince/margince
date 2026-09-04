// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What the open pipeline is worth, in one currency.
//
// Every case here is a decision the report makes that a reader would otherwise
// have to take on trust: that a won deal is gone from the composition, that a
// deal in another currency is converted at its own rate rather than added as a
// bare number, that a closed one keeps the rate it closed at, and that a deal
// whose rate is missing is COUNTED and not PRICED rather than silently zeroed.
//
// The arithmetic is asserted against literal expected values rather than
// against a formula recomputed here, because a test that reproduces the
// production expression proves only that it was copied correctly.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const pipelineCurrentPlan = `{"group_by":["stage_id"],"aggregates":[` +
	`{"fn":"count","as":"deals"},` +
	`{"fn":"sum","field":"amount_base_minor","as":"base"},` +
	`{"fn":"sum","field":"weighted_base_minor","as":"weighted_base"}]}`

// seedPricedDeal writes one deal in a named currency and status.
func seedPricedDeal(
	t *testing.T, e *forecastEnv, name string, probability int, amountMinor int64, currency, status string,
) ids.UUID {
	t.Helper()
	// A closed deal must carry its frozen rate — deal_closed_fx refuses a
	// priced one without, which is the schema half of "history is not
	// re-converted". The base currency is no exception: the rate is 1 there,
	// and the constraint asks for a rate rather than for a conversion.
	return e.seedID(t, `INSERT INTO deal
		(id, name, pipeline_id, stage_id, amount_minor, currency, status, closed_at,
		 fx_rate_to_base, fx_rate_date, lost_reason, expected_close_date, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6::text, $7::text,
		        CASE WHEN $7::text = 'open' THEN NULL ELSE now() END,
		        CASE WHEN $7::text = 'open' THEN NULL
		             WHEN $6::text = 'EUR' THEN 1::numeric ELSE 0.5::numeric END,
		        CASE WHEN $7::text = 'open' THEN NULL ELSE current_date END,
		        -- deal_lost_reason wants one exactly when the deal was lost, and
		        -- deal_lost_reason_only_when_lost refuses one otherwise.
		        CASE WHEN $7::text = 'lost' THEN 'went elsewhere' END,
		        (now() + interval '30 days')::date, 'manual', 'human:x')`,
		name, e.pipeline, e.stages[probability], amountMinor, currency, status)
}

func seedRate(t *testing.T, e *forecastEnv, from string, rate string, daysAgo int) {
	t.Helper()
	e.seedID(t, `INSERT INTO fx_rate (id, from_currency, to_currency, rate, rate_date)
		VALUES ($1, $2::text, 'EUR', $3::numeric, (current_date - make_interval(days => $4::int))::date)`,
		from, rate, daysAgo)
}

// A won deal is money that arrived. It is not still in the pipeline, and a
// composition that counts it reports work the team cannot act on.
func TestPipelineCurrentHoldsOnlyOpenDeals(t *testing.T) {
	e := setupForecast(t)
	seedPricedDeal(t, e, "Still open", 60, 10_000, "EUR", "open")
	seedPricedDeal(t, e, "Already won", 60, 90_000, "EUR", "won")
	seedPricedDeal(t, e, "Lost", 60, 70_000, "EUR", "lost")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[60].String())

	if got := wireInt(t, row, "deals"); got != 1 {
		t.Errorf("open stage counted %v deals, want the one still in play", got)
	}
	if got := wireInt(t, row, "base"); got != 10_000 {
		t.Errorf("open pipeline totalled %v, want only the open deal's 10000", got)
	}
}

// One stage trading in three currencies is ONE row, because each deal was
// converted on its own before anything was added.
func TestPipelineCurrentConvertsEachDealAndDrawsOneRowPerStage(t *testing.T) {
	e := setupForecast(t)
	// 0.5 EUR per unit, in force since yesterday.
	seedRate(t, e, "USD", "0.5", 1)
	seedPricedDeal(t, e, "Home", 60, 10_000, "EUR", "open")
	seedPricedDeal(t, e, "Abroad", 60, 10_000, "USD", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)

	if len(result.Rows) != 1 {
		t.Fatalf("a stage in two currencies drew %d rows, want one", len(result.Rows))
	}
	row := dealsByStageRow(t, result, e.stages[60].String())
	// 10000 EUR + (10000 USD × 0.5) = 15000, and never the 20000 that adding
	// the two native amounts would produce.
	if got := wireInt(t, row, "base"); got != 15_000 {
		t.Errorf("converted total was %v, want 15000", got)
	}
}

// A rate that arrives after the answer's as-of date does not reach back and
// re-price what was already read.
func TestPipelineCurrentTakesTheLatestRateOnOrBeforeAsOf(t *testing.T) {
	e := setupForecast(t)
	seedRate(t, e, "USD", "0.5", 10)
	seedRate(t, e, "USD", "0.9", 2)
	seedPricedDeal(t, e, "Abroad", 60, 10_000, "USD", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[60].String())

	if got := wireInt(t, row, "base"); got != 9_000 {
		t.Errorf("converted at %v, want the newest applicable rate's 9000", got)
	}
}

// A deal with no rate to convert by is COUNTED and not PRICED. Zeroing it would
// shrink the total and read as a smaller pipeline rather than a missing rate.
func TestPipelineCurrentCountsAnUnpricedDealAndExcludesItFromTheMoney(t *testing.T) {
	e := setupForecast(t)
	seedPricedDeal(t, e, "Priced", 60, 10_000, "EUR", "open")
	// VND with no rate sheet entry at all.
	seedPricedDeal(t, e, "No rate", 60, 5_000_000, "VND", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[60].String())

	if got := wireInt(t, row, "deals"); got != 2 {
		t.Errorf("counted %v deals, want both — an unconvertible deal is still pipeline", got)
	}
	if got := wireInt(t, row, "base"); got != 10_000 {
		t.Errorf("money total was %v, want only the deal that could be priced", got)
	}
}

// The weighted total is the sum of each deal's OWN rounded weight. Rounding the
// stage total once instead differs by up to a minor unit per deal, and the rows
// a reader opens would stop adding up to the figure above them.
func TestPipelineCurrentWeightsPerDealAndNotOnTheTotal(t *testing.T) {
	e := setupForecast(t)
	// 12341 × 60% = 7404.6 → 7405 each, so two round to 14810 while their
	// combined raw total rounds to 14809.
	const amount = int64(12_341)
	seedPricedDeal(t, e, "Alpha", 60, amount, "EUR", "open")
	seedPricedDeal(t, e, "Beta", 60, amount, "EUR", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[60].String())

	if got := wireInt(t, row, "weighted_base"); got != 14_810 {
		t.Errorf("weighted base was %v, want 14810 — the sum of two per-deal roundings", got)
	}
}
