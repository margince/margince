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
	"net/url"
	"testing"
	"time"
)

const pipelineCurrentPlan = `{"group_by":["stage_id"],"aggregates":[` +
	`{"fn":"count","as":"deals"},` +
	`{"fn":"sum","field":"amount_base_minor","as":"base"},` +
	`{"fn":"sum","field":"weighted_base_minor","as":"weighted_base"}]}`

// seedPricedDeal writes one deal in a named currency and status.
//
// Every case here measures ONE stage, so the probability is fixed rather than a
// parameter: a second stage would be a different question (which stage does a
// deal belong to) and none of these tests asks it.
func seedPricedDeal(t *testing.T, e *forecastEnv, name string, amountMinor int64, currency, status string) {
	t.Helper()
	// A closed deal must carry its frozen rate — deal_closed_fx refuses a
	// priced one without, which is the schema half of "history is not
	// re-converted". The base currency is no exception: the rate is 1 there,
	// and the constraint asks for a rate rather than for a conversion.
	e.seedID(t, `INSERT INTO deal
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
		name, e.pipeline, e.stages[pipelineTestStage], amountMinor, currency, status)
}

// pipelineTestStage is the win probability naming the one stage these cases
// seed into; forecastEnv keys its stages by it.
const pipelineTestStage = 60

// seedRate puts one USD→base rate on the sheet. USD because these cases only
// need A foreign currency, and one name keeps the arithmetic in the assertions
// readable.
func seedRate(t *testing.T, e *forecastEnv, rate string, daysAgo int) {
	t.Helper()
	e.seedID(t, `INSERT INTO fx_rate (id, from_currency, to_currency, rate, rate_date)
		VALUES ($1, 'USD', 'EUR', $2::numeric, (current_date - make_interval(days => $3::int))::date)`,
		rate, daysAgo)
}

// A won deal is money that arrived. It is not still in the pipeline, and a
// composition that counts it reports work the team cannot act on.
func TestPipelineCurrentHoldsOnlyOpenDeals(t *testing.T) {
	e := setupForecast(t)
	seedPricedDeal(t, e, "Still open", 10_000, "EUR", "open")
	seedPricedDeal(t, e, "Already won", 90_000, "EUR", "won")
	seedPricedDeal(t, e, "Lost", 70_000, "EUR", "lost")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())

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
	seedRate(t, e, "0.5", 1)
	seedPricedDeal(t, e, "Home", 10_000, "EUR", "open")
	seedPricedDeal(t, e, "Abroad", 10_000, "USD", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)

	if len(result.Rows) != 1 {
		t.Fatalf("a stage in two currencies drew %d rows, want one", len(result.Rows))
	}
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())
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
	seedRate(t, e, "0.5", 10)
	seedRate(t, e, "0.9", 2)
	seedPricedDeal(t, e, "Abroad", 10_000, "USD", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())

	if got := wireInt(t, row, "base"); got != 9_000 {
		t.Errorf("converted at %v, want the newest applicable rate's 9000", got)
	}
}

// A deal with no rate to convert by is COUNTED and not PRICED. Zeroing it would
// shrink the total and read as a smaller pipeline rather than a missing rate.
func TestPipelineCurrentCountsAnUnpricedDealAndExcludesItFromTheMoney(t *testing.T) {
	e := setupForecast(t)
	seedPricedDeal(t, e, "Priced", 10_000, "EUR", "open")
	// VND with no rate sheet entry at all.
	seedPricedDeal(t, e, "No rate", 5_000_000, "VND", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())

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
	seedPricedDeal(t, e, "Alpha", amount, "EUR", "open")
	seedPricedDeal(t, e, "Beta", amount, "EUR", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())

	if got := wireInt(t, row, "weighted_base"); got != 14_810 {
		t.Errorf("weighted base was %v, want 14810 — the sum of two per-deal roundings", got)
	}
}

// A converted headline must open the deals it was summed from, and those deals
// must add back up to it.
//
// The drill-through is where a reader checks a number they doubt, so a detail
// set that reconciles to something else is worse than no detail at all: it
// looks like proof.
func TestPipelineCurrentDetailReconcilesToTheConvertedHeadline(t *testing.T) {
	e := setupForecast(t)
	seedRate(t, e, "0.5", 1)
	seedPricedDeal(t, e, "Home", 10_000, "EUR", "open")
	seedPricedDeal(t, e, "Abroad", 10_000, "USD", "open")
	// Won, so it belongs to neither the headline nor the rows under it.
	seedPricedDeal(t, e, "Already won", 90_000, "EUR", "won")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())
	handle, ok := row["derivation_url"].(string)
	if !ok || handle == "" {
		t.Fatalf("the converted stage row minted no derivation handle: %+v", row)
	}

	derivation := e.explainReport(e.Admin(), t, "pipeline-current", handle)
	if len(derivation.Rows) != 2 {
		t.Fatalf("detail opened %d deals, want the two still in play: %+v",
			len(derivation.Rows), derivation.Rows)
	}
	if got := wireInt(t, row, "base"); got != 15_000 {
		t.Fatalf("headline was %d, want the converted 15000", got)
	}
}

// The detail behind a converted number reads the rate sheet its headline read.
//
// A handle pins WHICH rows a cell covered. Until the report engine converted
// money, it did not need to pin WHEN: the frame's as-of reached no arithmetic,
// so a drill-through sampling a fresh `now()` answered the same question. It
// stopped being harmless the moment a rate sheet could take effect between the
// two, because the drill-through is where a reader goes to check a figure they
// already doubt — and a detail set that reconciles to something else does not
// read as a discrepancy, it reads as proof.
//
// Two sheets either side of a day boundary, one deal, three readings.
func TestADerivationConvertsAtTheInstantItsHeadlineDid(t *testing.T) {
	e := setupForecast(t)
	seedRate(t, e, "0.5", 2)
	seedRate(t, e, "0.9", 0)
	seedPricedDeal(t, e, "Abroad", 10_000, "USD", "open")

	result := e.runReport(e.Admin(), t, "pipeline-current", pipelineCurrentPlan)
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())
	if got := wireInt(t, row, "base"); got != 9_000 {
		t.Fatalf("the headline converted to %d, want 9000 at today's 0.9 — the rest of "+
			"this case compares against it, so it has to be the sheet in force now", got)
	}
	handle, ok := row["derivation_url"].(string)
	if !ok || handle == "" {
		t.Fatalf("the converted stage row minted no derivation handle: %+v", row)
	}

	// Opened now, pinned to now: the same sheet, the same number.
	live := e.explainReport(e.Admin(), t, "pipeline-current", handle)
	if live.AsOfPinned == nil || !*live.AsOfPinned {
		t.Errorf("a freshly minted handle reports as_of_pinned %v — the mint is what "+
			"puts the instant in it, so an unpinned one here means it never went in",
			live.AsOfPinned)
	}
	if got := derivationSum(t, live, "base"); got != 9_000 {
		t.Errorf("the detail behind a 9000 headline recomputed %d", got)
	}

	// The same handle as it would have been minted the day before yesterday,
	// when 0.5 was the sheet in force. This is the reported defect: the pin has
	// to beat `now()`, or the detail prices the deal at today's rate and
	// silently disagrees with the number it explains.
	yesterday := e.explainReport(e.Admin(), t, "pipeline-current",
		rehandle(t, handle, asOfKey, time.Now().UTC().AddDate(0, 0, -1).Format(time.RFC3339Nano)))
	if got := derivationSum(t, yesterday, "base"); got != 5_000 {
		t.Errorf("a handle pinned before today's sheet recomputed %d, want 5000 at 0.5 — "+
			"the detail re-read the rate sheet instead of the one its headline used", got)
	}

	// A link minted before this key existed still resolves, and says so. It
	// recomputes at a fresh instant, which is honest only because the answer
	// carries as_of_pinned=false rather than passing it off as the headline's.
	old := e.explainReport(e.Admin(), t, "pipeline-current", rehandle(t, handle, asOfKey, ""))
	if old.AsOfPinned == nil || *old.AsOfPinned {
		t.Errorf("an unpinned handle reports as_of_pinned %v, want false — a reader "+
			"cannot tell the figures may have moved unless the answer says so",
			old.AsOfPinned)
	}
	if got := derivationSum(t, old, "base"); got != 9_000 {
		t.Errorf("the unpinned handle recomputed %d, want today's 9000", got)
	}
}

// rehandle rewrites one query parameter of a minted handle; an empty value
// removes it, which is what a link minted before that key looks like.
func rehandle(t *testing.T, handle, key, value string) string {
	t.Helper()
	parsed, err := url.Parse(handle)
	if err != nil {
		t.Fatalf("parsing the minted handle: %v", err)
	}
	q := parsed.Query()
	if value == "" {
		q.Del(key)
	} else {
		q.Set(key, value)
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// derivationSum reads one recomputed aggregate off a derivation answer.
func derivationSum(t *testing.T, d derivationWire, key string) int64 {
	t.Helper()
	if d.Aggregates == nil {
		t.Fatalf("the derivation recomputed no aggregates: %+v", d)
	}
	return wireInt(t, d.Aggregates, key)
}
