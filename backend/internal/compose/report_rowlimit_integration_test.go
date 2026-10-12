// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
)

// A report at its row limit says how many groups it did not show.
//
// The limit caps the rows, not the question: a reader handed a thousand of
// 1001 groups has the top of an answer, and total_rows is the only thing in
// the envelope that can say so.
func TestAReportAtItsRowLimitStillCountsEveryGroup(t *testing.T) {
	e := setupForecast(t)
	// One group per stage, one deal past the limit, so the cap cuts one group
	// and an off-by-one in either direction shows up as 1000 or 1002.
	groups := reportRowLimit + 1
	seedStagesWithADealEach(t, e, groups)

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id"],"aggregates":[{"fn":"count","as":"deals"}]}`)

	if len(result.Rows) != reportRowLimit {
		t.Fatalf("the report returned %d rows, so the limit of %d is not what bounded it",
			len(result.Rows), reportRowLimit)
	}
	if result.TotalRows != groups {
		t.Errorf("total_rows = %d, want %d: the envelope reports its page length, so the cap is invisible",
			result.TotalRows, groups)
	}
}

// A report inside its limit reports the groups it shows, and takes no second
// query to say so.
func TestAReportInsideItsLimitReportsWhatItShows(t *testing.T) {
	e := setupForecast(t)
	seedStagesWithADealEach(t, e, 3)

	result := e.runReport(e.Admin(), t, "deals-by-stage",
		`{"group_by":["stage_id"],"aggregates":[{"fn":"count","as":"deals"}]}`)

	if len(result.Rows) != 3 || result.TotalRows != 3 {
		t.Errorf("%d rows and total_rows = %d, want 3 and 3", len(result.Rows), result.TotalRows)
	}
}

// seedStagesWithADealEach plants n stages on the env's pipeline, each holding
// one live deal, so a report grouped by stage_id has n groups and no more.
//
// In bulk, like the rest of this file seeds: n is above the row limit here, and
// the question under test is how the read path counts, not how a stage is
// written.
//
// Positions start above the pipeline's current highest, because uq_stage_position
// is unique per pipeline and the env has already seeded its own stages low.
func seedStagesWithADealEach(t *testing.T, e *forecastEnv, n int) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		WITH s AS (
			INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
			SELECT $1, 'Stage ' || i, p.top + i, 'open', 50
			  FROM generate_series(1, $2) AS i,
			       (SELECT coalesce(max(position), 0) AS top FROM stage WHERE pipeline_id = $1) p
			RETURNING id
		)
		INSERT INTO deal (name, pipeline_id, stage_id, amount_minor, currency,
		                  expected_close_date, source, captured_by)
		SELECT 'Deal in ' || s.id, $1, s.id, 10000, 'EUR',
		       (now() + interval '30 days')::date, 'manual', 'human:x' FROM s`,
		e.pipeline, n); err != nil {
		t.Fatalf("seeding %d stages each holding a deal: %v", n, err)
	}
}

// The count binds its own statement, so a dimension carrying a frame token
// still counts.
//
// win-loss groups by period_month, whose expression names the installation
// timezone through a token the engine binds a value for. The rows statement
// binds first, and counting from its bound arguments would number the count's
// placeholders past the values that bind appended, so this case fails with a
// parameter error where a report of plain columns passes.
func TestAReportGroupedByATokenBearingDimensionCountsItsGroups(t *testing.T) {
	e := setupForecast(t)
	months := reportRowLimit + 1
	seedClosedDealAMonthApart(t, e, months)

	result := e.runReport(e.Admin(), t, "win-loss",
		`{"group_by":["period_month"],"aggregates":[{"fn":"count","as":"deals"}]}`)

	if len(result.Rows) != reportRowLimit {
		t.Fatalf("the report returned %d rows, so the limit of %d is not what bounded it",
			len(result.Rows), reportRowLimit)
	}
	if result.TotalRows != months {
		t.Errorf("total_rows = %d, want %d months", result.TotalRows, months)
	}
}

// seedClosedDealAMonthApart plants n won deals, each closed in a month of its
// own, so a report grouped by period_month has n groups.
//
// A priced closed deal carries its rate: deal_closed_fx wants fx_rate_to_base
// once status leaves 'open', because a figure converted at an unknown rate is
// not a figure.
func seedClosedDealAMonthApart(t *testing.T, e *forecastEnv, n int) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO deal (name, pipeline_id, stage_id, amount_minor, currency,
		                  fx_rate_to_base, expected_close_date, closed_at,
		                  status, source, captured_by)
		SELECT 'Won in month ' || i, $1, $2, 10000, 'EUR', 1,
		       (now() - make_interval(months => i))::date,
		       now() - make_interval(months => i), 'won', 'manual', 'human:x'
		  FROM generate_series(1, $3) AS i`,
		e.pipeline, e.stages[60], n); err != nil {
		t.Fatalf("seeding %d won deals a month apart: %v", n, err)
	}
}
