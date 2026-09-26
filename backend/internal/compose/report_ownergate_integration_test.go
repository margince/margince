// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The owner gate on an install-wide report (reportownergate.go), through the
// real engine: a breakdown by owner answers over the owners the caller may
// measure and says so, on every door onto the report.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const winLossByOwner = `{"group_by":["owner_id","currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"won"}]}`

// seedWinsByThreeOwners plants one win for Rep1 (Team1), one for Rep3
// (Team2) and one nobody owns, each at a different amount so a total names
// which of them it counted.
func seedWinsByThreeOwners(t *testing.T, e *forecastEnv) {
	t.Helper()
	seedClosedWin(t, e, "Rep1 win", e.Rep1, 10000)
	seedClosedWin(t, e, "Rep3 win", e.Rep3, 300000)
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, status, closed_at, lost_reason, fx_rate_to_base, source, captured_by)
		VALUES ($1, 'Unowned win', $2, $3, 2000, 'EUR', 'won', '2026-01-15T10:00:00Z'::timestamptz, NULL, 1.0, 'manual', 'human:x')`,
		e.pipeline, e.stages[60])
}

// wonByOwner folds a win-loss breakdown by owner into owner → amount, with ""
// for the unowned group.
func wonByOwner(t *testing.T, rows []map[string]any) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	for _, row := range rows {
		out[ownerKey(row)] = wireInt(t, row, "won")
	}
	return out
}

// ownerKey is a row's owner as text, "" for the group nobody owns.
func ownerKey(row map[string]any) string {
	if row["owner_id"] == nil {
		return ""
	}
	return fmt.Sprint(row["owner_id"])
}

func TestAGroupingByOwnerCountsOnlyTheOwnersARepMayMeasure(t *testing.T) {
	e := setupForecast(t)
	seedWinsByThreeOwners(t, e)

	rep := e.dealReadCtx(e.Rep1, nil, principal.RowScopeOwn)
	result := e.runReport(rep, t, "win-loss", winLossByOwner)
	got := wonByOwner(t, result.Rows)
	if len(got) != 2 || got[e.Rep1.String()] != 10000 || got[""] != 2000 {
		t.Fatalf("a rep's breakdown by owner = %v, want only their own 10000 and the unowned 2000", got)
	}
	if result.PopulationNarrowed == nil || *result.PopulationNarrowed != narrowedToMeasurableOwners {
		t.Fatalf("population_narrowed = %v, want %q — an answer narrowed without saying so reads "+
			"as every owner's", result.PopulationNarrowed, narrowedToMeasurableOwners)
	}

	// The whole-result explanation reads the same narrowed set and says so too.
	derivation := e.explainReport(rep, t, "win-loss", result.DerivationURL)
	if derivation.TotalRows != 2 {
		t.Errorf("the drill-through opened %d deals, want the 2 its headline counted", derivation.TotalRows)
	}
	if derivation.PopulationNarrowed == nil || *derivation.PopulationNarrowed != narrowedToMeasurableOwners {
		t.Errorf("the drill-through's population_narrowed = %v, want %q",
			derivation.PopulationNarrowed, narrowedToMeasurableOwners)
	}
}

func TestAGroupingByOwnerCountsATeamManagersTeammates(t *testing.T) {
	e := setupForecast(t)
	seedWinsByThreeOwners(t, e)

	manager := e.dealReadCtx(ids.NewV7(), []ids.UUID{e.Team1}, principal.RowScopeTeam)
	got := wonByOwner(t, e.runReport(manager, t, "win-loss", winLossByOwner).Rows)
	if len(got) != 2 || got[e.Rep1.String()] != 10000 || got[""] != 2000 {
		t.Fatalf("a Team1 manager's breakdown = %v, want Rep1's 10000 and the unowned 2000, "+
			"and not Team2's Rep3", got)
	}
}

func TestAGroupingByOwnerIsWholeForAnAllScopeSeat(t *testing.T) {
	e := setupForecast(t)
	seedWinsByThreeOwners(t, e)

	result := e.runReport(e.Admin(), t, "win-loss", winLossByOwner)
	if got := wonByOwner(t, result.Rows); len(got) != 3 || got[e.Rep3.String()] != 300000 {
		t.Fatalf("an admin's breakdown = %v, want all three owners", got)
	}
	if result.PopulationNarrowed != nil {
		t.Errorf("population_narrowed = %q for a seat nothing was narrowed for", *result.PopulationNarrowed)
	}
}

// A drill-through handle is a URL the caller can edit, so a row handle naming
// a colleague is the filter refusal again, not a way around it.
func TestARowHandleNamingAColleagueIsRefusedForARep(t *testing.T) {
	e := setupForecast(t)
	seedWinsByThreeOwners(t, e)

	admin := e.runReport(e.Admin(), t, "win-loss", winLossByOwner)
	var colleagueHandle string
	for _, row := range admin.Rows {
		if ownerKey(row) == e.Rep3.String() {
			colleagueHandle = fmt.Sprint(row["derivation_url"])
		}
	}
	if colleagueHandle == "" {
		t.Fatalf("the admin's breakdown carries no handle for Rep3's row: %+v", admin.Rows)
	}

	rep := e.dealReadCtx(e.Rep1, nil, principal.RowScopeOwn)
	req := httptest.NewRequest(http.MethodGet, colleagueHandle, nil).WithContext(rep)
	rec := httptest.NewRecorder()
	e.handlers.ExplainReport(rec, req, "win-loss", crmcontracts.ExplainReportParams{})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a rep opening a colleague's cell got %d %s, want 403", rec.Code, rec.Body.String())
	}
}

// The typed grammar compares an owner with ne, lt and the rest, so it cannot
// judge one value. Any mention of the owner narrows instead.
func TestATypedQueryByOwnerCountsOnlyTheOwnersARepMayMeasure(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	// Above the privacy floor on both sides, so the floor withholds nothing
	// and the count says which population was measured.
	for range 7 {
		seedClosedWin(t, e, "Mine", e.Rep1, amount)
		seedClosedWin(t, e, "Theirs", e.Rep3, amount)
	}

	ask := func(ctx context.Context) map[string]any {
		t.Helper()
		answer, err := e.askAnalytics(ctx, t, analyticsquery.Query{
			Entity:   "win-loss",
			GroupBy:  []string{"owner_id"},
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
		})
		if err != nil {
			t.Fatalf("a typed breakdown by owner was refused: %v", err)
		}
		owners := map[string]any{}
		for _, row := range answer.Rows {
			owners[ownerKey(row)] = row["deals"]
		}
		return owners
	}
	if owners := ask(e.ownLensRepCtx(e.Rep1)); len(owners) != 1 || owners[e.Rep1.String()] == nil {
		t.Errorf("a rep's typed breakdown by owner = %v, want only their own", owners)
	}
	if owners := ask(e.reportReaderCtx()); len(owners) != 2 {
		t.Errorf("an all-scope reader's typed breakdown = %v, want both owners", owners)
	}
}
