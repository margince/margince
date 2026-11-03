// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The manager half of the canonical corpus, answered through the MCP tool
// runner against a real database.
//
// Each case seeds a fixture that can DISCRIMINATE: a wrong engine returns a
// different number rather than the same one, and the failure message says which
// wrong answer it got. A proof whose fixture cannot tell right from wrong is
// prose with a test around it.

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedClosedOwnedDeal writes one won or lost deal owned by somebody, with a
// chosen source, closed now.
func seedClosedOwnedDeal(
	t *testing.T, e *forecastEnv, name string, amountMinor int64, status, source, lostReason string, owner ids.UUID,
) {
	t.Helper()
	seedClosedDealAgo(t, e, name, amountMinor, status, "0 days", owner, source, lostReason)
}

// seedClosedDealAgo is seedClosedOwnedDeal closed an interval ago, such as
// "3 months", which always lands in the previous quarter.
//
// The table refuses a reason on a won deal, so lostReason is set only when the
// status is lost.
func seedClosedDealAgo(
	t *testing.T, e *forecastEnv, name string, amountMinor int64, status, ago string, owner ids.UUID,
	source, lostReason string,
) {
	t.Helper()
	e.seedID(t, `INSERT INTO deal
		(id, name, pipeline_id, stage_id, owner_id, amount_minor, currency, status, closed_at,
		 fx_rate_to_base, fx_rate_date, lost_reason, expected_close_date, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'EUR', $7::text, (timezone('UTC', now()) - $10::interval) AT TIME ZONE 'UTC', 1, current_date,
		        CASE WHEN $7::text = 'lost' THEN $9::text END,
		        (now() + interval '30 days')::date, $8, 'human:x')`,
		name, e.pipeline, e.stages[pipelineTestStage], owner, amountMinor, status, source, lostReason, ago)
}

// M08 — win rate by deal COUNT and by VALUE, this quarter.
//
// The two rates are different questions and a manager acts differently on each:
// winning most of the small ones and losing the large one is a good count rate
// and a bad value rate. An engine that reported one number for both would hide
// exactly the quarter worth knowing about.
func proveM08WinRateByCountAndValue(t *testing.T) {
	e := setupForecast(t)
	// Six won at 10k and five lost at 60k: 6 of 11 deals won (55%) against
	// 60k of 360k in value (17%). Most deals won, most of the money lost —
	// deliberately opposite, so one number cannot land on both.
	//
	// Both groups clear the privacy floor. A cohort under it is WITHHELD, which
	// is the right answer to a question about few contacts and the wrong fixture
	// for a question about arithmetic: every assertion below would read null.
	for range 6 {
		seedClosedOwnedDeal(t, e, "Small win", 10_000, "won", "manual", "", e.Rep1)
	}
	for range 5 {
		seedClosedOwnedDeal(t, e, "Large loss", 60_000, "lost", "manual", "went elsewhere", e.Rep1)
	}
	// Last quarter's results, which "this quarter" must leave out.
	for range 5 {
		seedClosedDealAgo(t, e, "Old win", 10_000, "won", "3 months", e.Rep1, "manual", "")
		seedClosedDealAgo(t, e, "Old loss", 60_000, "lost", "3 months", e.Rep1, "manual", "went elsewhere")
	}

	got, err := askTool(e.wideLensCtx(e.Rep1), t, e, fmt.Sprintf(
		`{"entity":"win-loss","group_by":["status"],"measures":[`+
			`{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_base_minor","as":"value"}],`+
			`"filters":[{"field":"period_quarter","op":"eq","value":%q}]}`, e.currentQuarter(t)))
	if err != nil {
		t.Fatal(err)
	}
	byStatus := map[string]map[string]any{}
	for _, raw := range got.Rows {
		row := toolRow(t, raw)
		status, ok := row["status"].(string)
		if !ok {
			t.Fatalf("a row carries status %v (%T), want a string", row["status"], row["status"])
		}
		byStatus[status] = row
	}
	if len(byStatus) != 2 {
		t.Fatalf("the answer holds %d statuses, want won and lost", len(byStatus))
	}
	if n := byStatus["won"]["deals"]; n != float64(6) {
		t.Errorf("won deals = %v, want 6", n)
	}
	if n := byStatus["lost"]["deals"]; n != float64(5) {
		t.Errorf("lost deals = %v, want 5", n)
	}
	// The value side, where the count answer would be wrong. 300k lost against
	// 30k won: a manager reading only the count rate would call this a good
	// quarter.
	if v := byStatus["won"]["value"]; v != float64(60_000) {
		t.Errorf("won value = %v, want 60000 — the count rate and the value rate must be "+
			"separately readable, or lost money hides behind a majority of small wins", v)
	}
	if v := byStatus["lost"]["value"]; v != float64(300_000) {
		t.Errorf("lost value = %v, want 300000 — more deals won, more money lost", v)
	}
}

// M12 — which lead sources created the most won revenue.
//
// Grouped by the source the deal actually carries, and over WON deals only:
// counting open pipeline here would credit a source for revenue nobody has.
func proveM12WonRevenueByLeadSource(t *testing.T) {
	e := setupForecast(t)
	// Two sources, unequal, plus five LOST outbound deals worth far more than
	// either won total.
	//
	// The lost cohort is the discriminator and an open deal would not be: this
	// report's base predicate already excludes open deals before any caller
	// filter runs, so seeding one proves nothing about the status filter. An
	// engine that ignored the filter would count the losses and report outbound
	// as the best source in the business.
	for range 5 {
		seedClosedOwnedDeal(t, e, "Referral win", 16_000, "won", "referral", "", e.Rep1)
		seedClosedOwnedDeal(t, e, "Outbound win", 4_000, "won", "outbound", "", e.Rep1)
		seedClosedOwnedDeal(t, e, "Outbound loss", 200_000, "lost", "outbound", "went elsewhere", e.Rep1)
	}

	got, err := askTool(e.wideLensCtx(e.Rep1), t, e,
		`{"entity":"win-loss","group_by":["source"],"measures":[`+
			`{"fn":"sum","field":"amount_base_minor","as":"value"}],`+
			`"filters":[{"field":"status","op":"eq","value":"won"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[string]any{}
	for _, raw := range got.Rows {
		row := toolRow(t, raw)
		source, ok := row["source"].(string)
		if !ok {
			t.Fatalf("a row carries source %v (%T), want a string", row["source"], row["source"])
		}
		bySource[source] = row["value"]
	}
	if v := bySource["referral"]; v != float64(80_000) {
		t.Errorf("referral won revenue = %v, want 80000", v)
	}
	// An engine ignoring the status filter answers 1020000 here and reports
	// outbound as the best source in the business, on a million of losses.
	if v := bySource["outbound"]; v != float64(20_000) {
		t.Errorf("outbound won revenue = %v, want 20000 — a lost deal is not won revenue", v)
	}
	if len(bySource) != 2 {
		t.Errorf("the answer holds %d sources, want exactly referral and outbound — a third "+
			"row means the cohort reached deals this fixture did not seed", len(bySource))
	}
}

// M11, why are we losing and how many deals support each recorded reason.
//
// Grouped by the reason somebody WROTE on the deal. The answer is then the
// business's own vocabulary, not a status breakdown wearing its name.
func proveM11LossReasonsByRecordedReason(t *testing.T) {
	e := setupForecast(t)
	// Two reasons at different weights, plus a won cohort.
	//
	// The won cohort is the discriminator. Two CHECK constraints bind a reason
	// to the loss, so a won deal carries NULL here. An engine that dropped the
	// status filter answers a third row of wins under no reason. That row
	// reads as the commonest thing going wrong in the business.
	//
	// Every count clears analyticsquery.DefaultFloor, which is what the sizes
	// are for rather than realism. A group under the floor is withheld instead
	// of answered, and a filter excluding between one and the floor is refused
	// outright. A smaller fixture would prove the floor works and say nothing
	// about the grouping.
	for range 7 {
		seedClosedOwnedDeal(t, e, "Lost on price", 20_000, "lost", "manual", "price", e.Rep1)
	}
	for range 5 {
		seedClosedOwnedDeal(t, e, "Lost to a rival", 30_000, "lost", "manual", "went elsewhere", e.Rep1)
	}
	for range 6 {
		seedClosedOwnedDeal(t, e, "Won anyway", 50_000, "won", "manual", "", e.Rep1)
	}

	got, err := askTool(e.wideLensCtx(e.Rep1), t, e,
		`{"entity":"win-loss","group_by":["lost_reason"],"measures":[`+
			`{"fn":"count","as":"deals"}],`+
			`"filters":[{"field":"status","op":"eq","value":"lost"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	byReason := map[string]any{}
	for _, raw := range got.Rows {
		row := toolRow(t, raw)
		reason, ok := row["lost_reason"].(string)
		if !ok {
			t.Fatalf("a row carries lost_reason %v (%T), want a string: a null bucket here is the won cohort",
				row["lost_reason"], row["lost_reason"])
		}
		byReason[reason] = row["deals"]
	}
	if v := byReason["price"]; v != float64(7) {
		t.Errorf("price = %v, want 7", v)
	}
	if v := byReason["went elsewhere"]; v != float64(5) {
		t.Errorf("went elsewhere = %v, want 5", v)
	}
	if len(byReason) != 2 {
		t.Errorf("the answer holds %d reasons, want exactly the two that were recorded — a third "+
			"row is the won cohort arriving under no reason", len(byReason))
	}
}

// M05 — is our pipeline too concentrated in a few large deals?
//
// Count and value together, because concentration IS the gap between them: ten
// deals worth a million where one is 900k is a different book from ten worth
// a hundred thousand each, and the count alone cannot tell them apart.
func proveM05ConcentrationNeedsCountAndValue(t *testing.T) {
	e := setupForecast(t)
	small := int64(10_000)
	for range 9 {
		e.seedOpenDeal(t, "Ordinary", pipelineTestStage, &e.Rep1, &small, nil)
	}
	whale := int64(900_000)
	e.seedOpenDeal(t, "The whale", pipelineTestStage, &e.Rep1, &whale, nil)

	got, err := askTool(e.wideLensCtx(e.Rep1), t, e,
		`{"entity":"pipeline-current","measures":[`+
			`{"fn":"count","as":"deals"},{"fn":"sum","field":"amount_base_minor","as":"value"},`+
			`{"fn":"max","field":"amount_base_minor","as":"largest"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("an ungrouped question answered %d rows, want 1", len(got.Rows))
	}
	row := toolRow(t, got.Rows[0])
	if n := row["deals"]; n != float64(10) {
		t.Errorf("deals = %v, want 10", n)
	}
	if v := row["value"]; v != float64(990_000) {
		t.Errorf("value = %v, want 990000", v)
	}
	// The largest beside the total is what makes concentration sayable: 900k of
	// 990k rests on one deal, and no count could reveal that.
	if v := row["largest"]; v != float64(900_000) {
		t.Errorf("largest = %v, want 900000 — without it a reader cannot tell a concentrated "+
			"book from an even one", v)
	}
}

// M07 — which open deals are stuck with no agreed next step.
//
// Answered through the REPORT path rather than the typed grammar, and that is
// the finding rather than a workaround: `stalled` is a report FILTER, and the
// analytics schema publishes dimensions and measures only
// (analyticsqueryseam.go). So an agent asking this question through
// run_analytics_query is told the field does not exist, while the same question
// answers fine over HTTP.
//
// Stalled is a shared rule (deals.StalledSQL) rather than a threshold this
// report invents: a deal flagged here and unflagged on the board would make one
// of the two screens wrong.
func proveM07StuckDealsUseTheSharedRule(t *testing.T) {
	e := setupForecast(t)
	amount := int64(50_000)
	// One deal untouched for 90 days, one created just now.
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor,
		currency, status, expected_close_date, source, captured_by, created_at, updated_at)
		VALUES ($1, 'Idle for months', $2, $3, $4, 50000, 'EUR', 'open',
		        (now() + interval '30 days')::date, 'manual', 'human:x',
		        now() - interval '90 days', now() - interval '90 days')`,
		e.pipeline, e.stages[pipelineTestStage], e.Rep1)
	e.seedOpenDeal(t, "Fresh", pipelineTestStage, &e.Rep1, &amount, nil)
	// Idle as long, but deferred to a date ahead: that is an agreed next step.
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor,
		currency, status, expected_close_date, source, captured_by, created_at, updated_at, wait_until)
		VALUES ($1, 'Waiting on legal', $2, $3, $4, 50000, 'EUR', 'open',
		        (now() + interval '30 days')::date, 'manual', 'human:x',
		        now() - interval '90 days', now() - interval '90 days', now() + interval '14 days')`,
		e.pipeline, e.stages[pipelineTestStage], e.Rep1)

	result := e.runReport(e.wideLensCtx(e.Rep1), t, "deals-by-stage",
		`{"group_by":["stage_id","currency"],"aggregates":[{"fn":"count","as":"deals"}],`+
			`"filters":{"stalled":true}}`)
	row := dealsByStageRow(t, result, e.stages[pipelineTestStage].String())
	if got := wireInt(t, row, "deals"); got != 1 {
		t.Errorf("stalled deals = %d, want 1. A fresh deal and a deal deferred to a later date "+
			"are not stuck. A rule counting either flags work with a next step", got)
	}
}
