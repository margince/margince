// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A named period moves closed-won and nothing else. The deals won in it are
// summed over the readable tree only. The pipeline stays the open deals at the
// read, the same figure under every period.
//
// The won deals are seeded with SQL. The close path stamps closed_at with the
// database clock, and a past period needs a deal won in it.
func TestTheRollupSumsTheNamedPeriodOverTheReadableTree(t *testing.T) {
	e := Setup(t)
	st := seedRollupStages(t, e)
	now := clocktest.Now(t)
	root := seedRollupCompany(t, e, "Period Root", &e.Rep1, nil)
	child := seedRollupCompany(t, e, "Period Child", &e.Rep1, &root)
	hidden := seedRollupCompany(t, e, "Hidden Child", &e.Rep3, &root)
	e.MakeCapturePrivate(t, "company", hidden, e.Rep3)
	seedRollupOpenDeal(t, e, st, root, new(int64(10_000)), new("EUR"))

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, rollupCompanyReadPerms(principal.RowScopeOwn))
	read := func(period compose.CompanyRollupPeriod) compose.CompanyRollupResult {
		t.Helper()
		res, err := compose.CompanyHierarchyRollup(rep, e.Pool, root, "tree", period, fixedClock(now))
		if err != nil {
			t.Fatalf("rollup for %+v: %v", period, err)
		}
		return res
	}
	current := read(compose.CompanyRollupPeriod{})
	start := current.ClosedWonPeriod.Start
	for _, company := range []ids.UUID{root, child, hidden} {
		seedRollupWonDeal(t, e, st, company, 1_000, "EUR", "1.0", start)
	}
	seedRollupWonDeal(t, e, st, child, 700, "EUR", "1.0", start.Add(-time.Hour))

	thisQuarter := read(compose.CompanyRollupPeriod{})
	lastQuarter := read(compose.CompanyRollupPeriod{Field: "period_quarter", Label: current.ClosedWonPeriod.Previous})
	if thisQuarter.ClosedWonMinor == nil || *thisQuarter.ClosedWonMinor != 2_000 {
		t.Errorf("this quarter's won = %v, want 2000 (root and child; the hidden child is not summed)", thisQuarter.ClosedWonMinor)
	}
	if lastQuarter.ClosedWonMinor == nil || *lastQuarter.ClosedWonMinor != 700 {
		t.Errorf("last quarter's won = %v, want 700 (the child's deal won an hour before this quarter)", lastQuarter.ClosedWonMinor)
	}
	if !lastQuarter.ClosedWonPeriod.Chosen || !lastQuarter.ClosedWonPeriod.End.Equal(start) {
		t.Errorf("last quarter = %+v, want a chosen period ending where this one starts", lastQuarter.ClosedWonPeriod)
	}
	if lastQuarter.ClosedWonPeriod.Next == nil || *lastQuarter.ClosedWonPeriod.Next != current.ClosedWonPeriod.Label {
		t.Errorf("last quarter's next = %v, want %s", lastQuarter.ClosedWonPeriod.Next, current.ClosedWonPeriod.Label)
	}
	for name, res := range map[string]compose.CompanyRollupResult{"this quarter": thisQuarter, "last quarter": lastQuarter} {
		if res.WeightedPipelineMinor != 4_000 {
			t.Errorf("%s: weighted pipeline = %d, want 4000, the open deal today whatever the period", name, res.WeightedPipelineMinor)
		}
		if len(res.RestrictedExcluded) != 1 || res.RestrictedExcluded[0].ID != hidden {
			t.Errorf("%s: restricted = %+v, want the hidden child disclosed by name", name, res.RestrictedExcluded)
		}
	}
}

// periodRollupWire is what the rollup's two doors share about one period: the
// read's own instants are left out, since the two reads run apart.
type periodRollupWire struct {
	ClosedWon        companyRollupMoneyWire `json:"closed_won"`
	WeightedPipeline companyRollupMoneyWire `json:"weighted_pipeline"`
	ClosedWonPeriod  struct {
		Grain    string  `json:"grain"`
		Label    string  `json:"label"`
		StartAt  string  `json:"start_at"`
		EndAt    string  `json:"end_at"`
		Anchor   string  `json:"anchor"`
		Chosen   bool    `json:"chosen"`
		Previous string  `json:"previous"`
		Next     *string `json:"next"`
	} `json:"closed_won_period"`
	CurrentPeriods map[string]string `json:"current_periods"`
}

// The rollup route, read_view and the win-loss report give one quarter's won
// sales the same total, and refuse a malformed period alike.
func TestARollupPeriodAnswersAlikeOnTheRouteTheToolAndTheReport(t *testing.T) {
	d := newTwoDoors(t, "rollup-period-doors", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	root := createCompanyRollupCompany(t, d.e, "Period Doors Root", "")
	child := createCompanyRollupCompany(t, d.e, "Period Doors Child", root)
	for company, amount := range map[string]int64{root: 30_000, child: 12_000} {
		var deal AnyMap
		if status := d.e.Call(t, "POST", "/v1/deals", AnyMap{
			"name": "Period win", "amount_minor": amount, "currency": "EUR", "pipeline_id": stages.PipelineID,
			"stage_id": stages.Open, "company_id": company, "source": "manual",
		}, nil, &deal); status != http.StatusCreated {
			t.Fatalf("create deal = %d %v", status, deal)
		}
		if status := d.e.Call(t, "POST", "/v1/deals/"+deal["id"].(string)+"/advance",
			AnyMap{"to_stage_id": stages.Won, "won_without_contract_reason": "verbal"}, nil, nil); status != http.StatusOK {
			t.Fatalf("winning the deal = %d", status)
		}
	}

	var opening periodRollupWire
	decode(t, d.rest(t, "GET", "/v1/companies/"+root+"/hierarchy-rollup", nil), &opening)
	quarter := opening.CurrentPeriods["period_quarter"]
	rest := d.rest(t, "GET", "/v1/companies/"+root+"/hierarchy-rollup?period_quarter="+url.QueryEscape(quarter), nil)
	tool := d.tool(t, "read_view", map[string]any{"view": "company_hierarchy_rollup", "id": root, "period_quarter": quarter})
	rollup := sameRecords[periodRollupWire](t, "company_hierarchy_rollup", rest, tool)
	if rollup.ClosedWon.AmountMinor != 42_000 || rollup.ClosedWonPeriod.Label != quarter || !rollup.ClosedWonPeriod.Chosen ||
		rollup.ClosedWonPeriod.Anchor != "closed_at" {
		t.Errorf("rollup = %+v, want 42000 won in the chosen quarter %s, placed by closed_at", rollup, quarter)
	}

	var reported int64
	for _, company := range []string{root, child} {
		var report struct {
			Rows []map[string]any `json:"rows"`
		}
		decode(t, d.rest(t, "POST", "/v1/reports/win-loss", AnyMap{
			"filters":    AnyMap{"company_id": company, "status": "won", "period_quarter": quarter},
			"aggregates": []AnyMap{{"fn": "sum", "field": "amount_base_minor", "as": "won"}},
		}), &report)
		if len(report.Rows) != 1 {
			t.Fatalf("win-loss for %s answered %d rows, want one", company, len(report.Rows))
		}
		won, ok := report.Rows[0]["won"].(float64)
		if !ok {
			t.Fatalf("win-loss won = %v, want a number", report.Rows[0]["won"])
		}
		reported += int64(won)
	}
	if reported != rollup.ClosedWon.AmountMinor {
		t.Errorf("win-loss reports %d won in %s, the rollup %d", reported, quarter, rollup.ClosedWon.AmountMinor)
	}

	for query, field := range map[string]string{
		"period_quarter=2026-Q9":                     "period_quarter",
		"period_year=2026&period_month=2026-03":      "period_month",
		"period_month=" + url.QueryEscape("2999-01"): "period_month",
	} {
		var problem fieldHistoryProblem
		status := d.e.Call(t, "GET", "/v1/companies/"+root+"/hierarchy-rollup?"+query, nil, d.bearer, &problem)
		assertFieldHistoryValidation422(t, status, problem, field, "invalid")
	}
	refused := d.toolRefused(t, "read_view", map[string]any{"view": "company_hierarchy_rollup", "id": root, "period_month": "2999-01"})
	if !strings.Contains(refused, "has not begun") {
		t.Errorf("read_view refused a future month with %q, want the route's own sentence", refused)
	}
}
