// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The persona half of the canonical corpus: a team manager reading their own
// team, a rep reading their own work, management reading the company.
//
// Every team proof over deals seeds a teammate and one off-team owner.
// A lens that shrinks to the caller, or leaks outward, changes the answer.
//
// Every group clears analyticsquery.DefaultFloor, so a withheld group never
// stands in for a wrong number.

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamManagerCtx is Rep1 reading deals through the team lens of Team1.
func (e *forecastEnv) teamManagerCtx() context.Context {
	return e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam)
}

// seedDealClosing plants an open deal with a chosen forecast category and a
// close date a number of days from today. A nil offset leaves the date empty.
func (e *forecastEnv) seedDealClosing(
	t *testing.T, owner ids.UUID, category string, closeInDays *int,
) {
	t.Helper()
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency,
			forecast_category, expected_close_date, source, captured_by)
		VALUES ($1, 'Deal', $2, $3, $4, 100000, 'EUR', $5,
		        CASE WHEN $6::int IS NULL THEN NULL ELSE current_date + $6::int END, 'manual', 'human:x')`,
		e.pipeline, e.stages[pipelineTestStage], owner, category, closeInDays)
}

// currentQuarter and currentMonth are today's period labels as the report
// spells them, cut in the zone the fixture installs (UTC).
func (e *forecastEnv) currentQuarter(t *testing.T) string { return e.periodLabel(t, patternQuarter) }
func (e *forecastEnv) currentMonth(t *testing.T) string   { return e.periodLabel(t, patternMonth) }

func (e *forecastEnv) periodLabel(t *testing.T, pattern string) string {
	t.Helper()
	var label string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT to_char(timezone('UTC', now()), $1)`, pattern).Scan(&label); err != nil {
		t.Fatalf("reading today's period label: %v", err)
	}
	return label
}

func seedMany(n int, seed func()) {
	for range n {
		seed()
	}
}

// rowsBy indexes an answer's rows by one column. A row without it lands under nil.
func rowsBy(t *testing.T, got []map[string]any, column string) map[any]map[string]any {
	t.Helper()
	out := map[any]map[string]any{}
	for _, row := range got {
		out[row[column]] = row
	}
	return out
}

func answerRows(ctx context.Context, t *testing.T, e *forecastEnv, query string) []map[string]any {
	t.Helper()
	got, err := askTool(ctx, t, e, query)
	if err != nil {
		t.Fatalf("the question was refused: %v", err)
	}
	rows := make([]map[string]any, 0, len(got.Rows))
	for _, raw := range got.Rows {
		rows = append(rows, toolRow(t, raw))
	}
	return rows
}

// M01: how much of my team's pipeline is commit, best case and pipeline. A
// commit deal past its close date reads as slipped, never as commit.
func proveM01ForecastByCategory(t *testing.T) {
	e := setupForecast(t)
	mate := reportingTeammate(t, e)
	future, past := new(30), new(-10)
	for _, c := range []struct {
		owner    ids.UUID
		category string
		close    *int
	}{{e.Rep1, "commit", future}, {mate, "best_case", future}, {mate, "pipeline", future}, {e.Rep1, "commit", past}} {
		seedMany(5, func() { e.seedDealClosing(t, c.owner, c.category, c.close) })
	}
	seedMany(5, func() { e.seedDealClosing(t, e.Rep3, "commit", future) })

	rows := rowsBy(t, answerRows(e.teamManagerCtx(), t, e, `{"entity":"forecast",
		"group_by":["forecast_category"],"measures":[{"fn":"count","as":"deals"},
		{"fn":"sum","field":"amount_base_minor","as":"value"}]}`), "forecast_category")
	for _, category := range []string{"commit", "best_case", "pipeline", "slipped"} {
		if row := rows[category]; row["deals"] != float64(5) || row["value"] != float64(500_000) {
			t.Errorf("%s = %v, want 5 deals worth 500000. A lens leak makes commit 10, "+
				"and so does a past-dated commit counted as commit", category, row)
		}
	}
}

// M02: how much of my team's open pipeline closes in each month. A deal with
// no close date has its own row.
func proveM02PipelineByCloseMonth(t *testing.T) {
	e := setupForecast(t)
	mate := reportingTeammate(t, e)
	seedMany(5, func() { e.seedDealClosing(t, e.Rep1, "pipeline", new(30)) })
	seedMany(5, func() { e.seedDealClosing(t, mate, "pipeline", new(65)) })
	seedMany(5, func() { e.seedDealClosing(t, e.Rep1, "pipeline", nil) })
	seedMany(5, func() { e.seedDealClosing(t, e.Rep3, "pipeline", new(30)) })
	seedMany(5, func() { seedClosedOwnedDeal(t, e, "Won", 100_000, "won", "manual", "", e.Rep1) })

	rows := answerRows(e.teamManagerCtx(), t, e, `{"entity":"pipeline-current",
		"group_by":["close_month"],"measures":[{"fn":"count","as":"deals"}]}`)
	if len(rows) != 3 {
		t.Fatalf("answered %d rows, want two months and the undated row: %v", len(rows), rows)
	}
	for _, row := range rows {
		if row["deals"] != float64(5) {
			t.Errorf("a month row counts %v, want 5, so a colleague's or a closed deal leaked", row)
		}
	}
	if _, ok := rowsBy(t, rows, "close_month")[nil]; !ok {
		t.Error("the deals with no close date have no row of their own")
	}
}

// M03: which of my team's committed deals have slipped. A commit or best-case
// deal with a past or missing date counts. A pipeline-category deal does not.
func proveM03SlippedCommittedDeals(t *testing.T) {
	e := setupForecast(t)
	mate := reportingTeammate(t, e)
	past := new(-10)
	seedMany(5, func() { e.seedDealClosing(t, e.Rep1, "commit", past) })
	seedMany(5, func() { e.seedDealClosing(t, mate, "best_case", nil) })
	seedMany(5, func() { e.seedDealClosing(t, e.Rep1, "pipeline", past) })
	seedMany(5, func() { e.seedDealClosing(t, e.Rep1, "commit", new(30)) })
	seedMany(5, func() { e.seedDealClosing(t, e.Rep3, "commit", past) })

	rows := answerRows(e.teamManagerCtx(), t, e, `{"entity":"forecast",
		"measures":[{"fn":"count","as":"deals"}],
		"filters":[{"field":"forecast_category","op":"eq","value":"slipped"}]}`)
	if len(rows) != 1 || rows[0]["deals"] != float64(10) {
		t.Errorf("slipped = %v, want 10: five past-dated commit and five undated best case", rows)
	}
}

// M09: who on my team owns the most open pipeline. An owner outside the team
// is never named.
func proveM09OwnerWithMostPipeline(t *testing.T) {
	e := setupForecast(t)
	mate := reportingTeammate(t, e)
	seedMany(5, func() { e.seedDealClosing(t, e.Rep1, "pipeline", new(30)) })
	seedMany(7, func() { e.seedDealClosing(t, mate, "pipeline", new(30)) })
	seedMany(9, func() { e.seedDealClosing(t, e.Rep3, "pipeline", new(30)) })

	rows := rowsBy(t, answerRows(e.teamManagerCtx(), t, e, `{"entity":"pipeline-current",
		"group_by":["owner_id"],"measures":[{"fn":"sum","field":"amount_base_minor","as":"value"}]}`), "owner_id")
	if rows[mate.String()]["value"] != float64(700_000) || rows[e.Rep1.String()]["value"] != float64(500_000) {
		t.Errorf("owners = %v, want the teammate at 700000 and the manager at 500000", rows)
	}
	if _, named := rows[e.Rep3.String()]; named || len(rows) != 2 {
		t.Errorf("the answer names an owner outside the team: %v", rows)
	}
}

// M10: which reps on my team won the most this quarter. A loss and a win from
// last quarter stay out.
func proveM10WinsByRepThisQuarter(t *testing.T) {
	e := setupForecast(t)
	mate := reportingTeammate(t, e)
	seedMany(5, func() { seedClosedOwnedDeal(t, e, "Won", 10_000, "won", "manual", "", e.Rep1) })
	seedMany(5, func() { seedClosedOwnedDeal(t, e, "Lost", 10_000, "lost", "manual", "price", e.Rep1) })
	seedMany(7, func() { seedClosedOwnedDeal(t, e, "Won", 10_000, "won", "manual", "", mate) })
	seedMany(5, func() { seedClosedDealAgo(t, e, "Won", 10_000, "won", "3 months", mate, "manual", "went elsewhere") })
	seedMany(9, func() { seedClosedOwnedDeal(t, e, "Won", 10_000, "won", "manual", "", e.Rep3) })

	rows := rowsBy(t, answerRows(e.teamManagerCtx(), t, e, fmt.Sprintf(`{"entity":"win-loss",
		"group_by":["owner_id"],"measures":[{"fn":"count","as":"deals"}],
		"filters":[{"field":"status","op":"eq","value":"won"},
		{"field":"period_quarter","op":"eq","value":%q}]}`, e.currentQuarter(t))), "owner_id")
	if rows[mate.String()]["deals"] != float64(7) || rows[e.Rep1.String()]["deals"] != float64(5) {
		t.Errorf("wins = %v, want the teammate at 7 and the manager at 5. Last quarter's five "+
			"and the manager's losses stay out", rows)
	}
	if _, named := rows[e.Rep3.String()]; named || len(rows) != 2 {
		t.Errorf("the answer names a rep outside the team: %v", rows)
	}
}

// G04: what did I win and lose this month. Only the rep's own closed deals.
func proveG04OwnWinsAndLossesThisMonth(t *testing.T) {
	e := setupForecast(t)
	seedMany(5, func() { seedClosedOwnedDeal(t, e, "Won", 10_000, "won", "manual", "", e.Rep1) })
	seedMany(6, func() { seedClosedOwnedDeal(t, e, "Lost", 10_000, "lost", "manual", "price", e.Rep1) })
	seedMany(5, func() { seedClosedDealAgo(t, e, "Won", 10_000, "won", "3 months", e.Rep1, "manual", "went elsewhere") })
	seedMany(7, func() { seedClosedOwnedDeal(t, e, "Colleague", 10_000, "won", "manual", "", e.Rep3) })

	rows := rowsBy(t, answerRows(e.ownLensRepCtx(e.Rep1), t, e, fmt.Sprintf(`{"entity":"win-loss",
		"group_by":["status"],"measures":[{"fn":"count","as":"deals"}],
		"filters":[{"field":"owner_id","op":"eq","value":%q},
		{"field":"period_month","op":"eq","value":%q}]}`, e.Rep1, e.currentMonth(t))), "status")
	if rows["won"]["deals"] != float64(5) || rows["lost"]["deals"] != float64(6) {
		t.Errorf("this month = %v, want 5 won and 6 lost of the rep's own. A colleague's "+
			"seven wins or last quarter's five wins leaked", rows)
	}
}

// G08: how much did we win this quarter compared with last quarter. Won deals
// only, in base currency.
func proveG08WonThisQuarterVersusLast(t *testing.T) {
	e := setupForecast(t)
	seedMany(5, func() { seedClosedOwnedDeal(t, e, "Won", 20_000, "won", "manual", "", e.Rep1) })
	seedMany(5, func() { seedClosedDealAgo(t, e, "Won", 10_000, "won", "3 months", e.Rep1, "manual", "went elsewhere") })
	seedMany(5, func() { seedClosedOwnedDeal(t, e, "Lost", 500_000, "lost", "manual", "price", e.Rep1) })

	rows := rowsBy(t, answerRows(e.wideLensCtx(e.Rep1), t, e, `{"entity":"win-loss",
		"group_by":["period_quarter"],"measures":[{"fn":"sum","field":"amount_base_minor","as":"value"}],
		"filters":[{"field":"status","op":"eq","value":"won"}]}`), "period_quarter")
	if len(rows) != 2 || rows[e.currentQuarter(t)]["value"] != float64(100_000) {
		t.Errorf("quarters = %v, want this quarter at 100000 and one earlier quarter", rows)
	}
	for quarter, row := range rows {
		if quarter != e.currentQuarter(t) && row["value"] != float64(50_000) {
			t.Errorf("%v = %v, want 50000, a lost deal is not won money", quarter, row)
		}
	}
}

// G10: which customers have the most open deals. One row per company, open
// deals only.
func proveG10CustomersWithMostOpenDeals(t *testing.T) {
	e := setupForecast(t)
	contoso := e.seedCompanyDeals(t, "Contoso", 6)
	fabrikam := e.seedCompanyDeals(t, "Fabrikam", 8)
	closedAt := e.seedCompanyDeals(t, "Closed only", 0)
	seedMany(5, func() {
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, owner_id, amount_minor,
				currency, status, closed_at, fx_rate_to_base, fx_rate_date, expected_close_date, source, captured_by)
			VALUES ($1, 'Won', $2, $3, $4, $5, 100000, 'EUR', 'won', now(), 1, current_date,
			        (now() + interval '30 days')::date, 'manual', 'human:x')`,
			e.pipeline, e.stages[20], closedAt, e.Rep1)
	})

	rows := rowsBy(t, answerRows(e.companyReaderCtx(e.Rep1), t, e, `{"entity":"open-deals-per-company",
		"group_by":["company_id"],"measures":[{"fn":"count","as":"deals"}]}`), "company_id")
	if rows[fabrikam.String()]["deals"] != float64(8) || rows[contoso.String()]["deals"] != float64(6) {
		t.Errorf("companies = %v, want Fabrikam at 8 and Contoso at 6", rows)
	}
	if _, named := rows[closedAt.String()]; named || len(rows) != 2 {
		t.Errorf("a company with only closed deals is listed: %v", rows)
	}
}

// G11: how many leads in each status, including the promoted and disqualified
// ones, which are archived.
func proveG11LeadsByStatus(t *testing.T) {
	e := setupForecast(t)
	contact := e.seedID(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Promoted contact', 'manual', 'human:x')`)
	for _, c := range []struct {
		status   string
		n        int
		archived bool
	}{{"new", 6, false}, {"engaged", 5, false}, {"promoted", 7, true}, {"disqualified", 5, true}} {
		seedMany(c.n, func() {
			e.seedID(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, archived_at,
					promoted_contact_id)
				VALUES ($1, 'Lead', $2, 'inbound', 'human:x', $3, CASE WHEN $4 THEN now() END,
				        CASE WHEN $2 = 'promoted' THEN $5::uuid END)`,
				c.status, e.Rep1, c.archived, contact)
		})
	}
	rows := rowsBy(t, answerRows(e.leadReaderCtx(), t, e,
		`{"entity":"leads-by-status","group_by":["status"],"measures":[{"fn":"count","as":"leads"}]}`), "status")
	for status, want := range map[string]float64{"new": 6, "engaged": 5, "promoted": 7, "disqualified": 5} {
		if rows[status]["leads"] != want {
			t.Errorf("%s = %v, want %v, the terminal statuses are counted too", status, rows[status], want)
		}
	}
}
