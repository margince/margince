// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The owner gate on an install-wide report (reportownergate.go), through the
// real engine: a breakdown by owner answers over the owners the caller may
// measure and says so, on every door onto the report.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// On an install-wide report the drill-through lists colleagues' deals with
// their owners and amounts. That is what the ordinary deal read already serves
// the same caller, and a field mask that read applies is applied here too.
func TestADrillThroughShowsWhatTheOrdinaryReadShowsAndNoMore(t *testing.T) {
	e := setupForecast(t)
	seedClosedWin(t, e, "Rep3 win", e.Rep3, 300000)
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	const byStatus = `{"group_by":["status","currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"won"}]}`

	rep := e.dealReadCtx(e.Rep1, nil, principal.RowScopeOwn)
	result := e.runReport(rep, t, "win-loss", byStatus)
	detail := e.explainReport(rep, t, "win-loss", result.DerivationURL)
	if len(detail.Rows) != 1 {
		t.Fatalf("the rep's drill-through = %+v, want Rep3's one deal", detail.Rows)
	}
	row := detail.Rows[0]
	read, err := store.GetDeal(rep, ids.From[ids.DealKind](ids.MustParse(fmt.Sprint(row["id"]))), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("the same rep's ordinary read of the deal was refused: %v", err)
	}
	if read.OwnerId == nil || read.OwnerId.String() != ownerKey(row) ||
		read.AmountMinor == nil || *read.AmountMinor != wireInt(t, row, "amount_minor") {
		t.Errorf("the drill-through shows owner %v and amount %v; the ordinary read shows %v and %v",
			row["owner_id"], row["amount_minor"], read.OwnerId, read.AmountMinor)
	}

	masked := e.maskedDealReader(principal.MaskOutsideWriteAuthority)
	result = e.runReport(masked, t, "win-loss", byStatus)
	if result.ExcludedByPermission == nil || *result.ExcludedByPermission != 1 {
		t.Fatalf("excluded_by_permission = %v, want Rep3's masked deal counted as withheld",
			result.ExcludedByPermission)
	}
	if detail := e.explainReport(masked, t, "win-loss", result.DerivationURL); len(detail.Rows) != 0 {
		t.Errorf("a reader whose mask withholds Rep3's amount opened it in the drill-through: %+v", detail.Rows)
	}
}

// typedReader is an own-scope seat asking typed questions of deals and projects.
func (e *forecastEnv) typedReader(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"deal": {Read: true}, "project": {Read: true}, "forecast": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

// A row id among a typed question's MEASURES or FILTERS does not make it a
// listing. Only the grouping can, so the owner breakdown stays narrowed.
func TestATypedOwnerBreakdownStaysNarrowedWhateverElseNamesTheRow(t *testing.T) {
	e := setupForecast(t)
	company := e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Delivery Co', 'manual', 'human:x')`)
	for range 7 {
		for _, owner := range []ids.UUID{e.Rep1, e.Rep3} {
			e.seedID(t, `INSERT INTO project (id, name, company_id, owner_id, phase, source, captured_by)
				VALUES ($1, 'Delivery', $2, $3, 'delivering', 'manual', 'human:x')`, company, owner)
		}
	}
	rep := e.typedReader(e.Rep1)
	byOwner := analyticsquery.Query{Entity: "projects-gone-quiet", GroupBy: []string{"owner_id"}}
	cases := map[string]analyticsquery.Query{
		"count_distinct(project_id)": {
			Entity: byOwner.Entity, GroupBy: byOwner.GroupBy,
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountDistinct, Field: "project_id", As: "n"}},
		},
		"project_id is_not_null": {
			Entity: byOwner.Entity, GroupBy: byOwner.GroupBy,
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
			Filters:  []analyticsquery.Filter{{Field: "project_id", Op: analyticsquery.OpIsNotNull}},
		},
	}
	for name, q := range cases {
		answer, err := e.askAnalytics(rep, t, q)
		if err != nil {
			t.Fatalf("%s: refused: %v", name, err)
		}
		if len(answer.Rows) != 1 || ownerKey(answer.Rows[0]) != e.Rep1.String() {
			t.Errorf("%s: a rep's breakdown by owner = %+v, want only their own", name, answer.Rows)
		}
		if answer.PopulationNarrowed != narrowedToMeasurableOwners {
			t.Errorf("%s: population_narrowed = %q, want %q", name, answer.PopulationNarrowed, narrowedToMeasurableOwners)
		}
	}
}

// A typed filter naming an owner is judged the way run_report judges one:
// a colleague is a 403 whatever the comparison, not an empty answer.
func TestATypedFilterNamingAColleagueIsRefused(t *testing.T) {
	e := setupForecast(t)
	for range 7 {
		seedClosedWin(t, e, "Mine", e.Rep1, 1000)
		seedClosedWin(t, e, "Theirs", e.Rep3, 1000)
	}
	rep := e.typedReader(e.Rep1)
	for _, op := range []analyticsquery.FilterOp{analyticsquery.OpEq, analyticsquery.OpNe, analyticsquery.OpLt} {
		_, err := e.askAnalytics(rep, t, analyticsquery.Query{
			Entity:   "win-loss",
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
			Filters:  []analyticsquery.Filter{{Field: "owner_id", Op: op, Value: e.Rep3.String()}},
		})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("owner_id %s <colleague>: err = %v, want permission denied", op, err)
		}
	}
	// Naming themselves is theirs to ask; a comparison other than equality
	// still splits by owner, so it is narrowed and says so.
	answer, err := e.askAnalytics(rep, t, analyticsquery.Query{
		Entity:   "win-loss",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
		Filters:  []analyticsquery.Filter{{Field: "owner_id", Op: analyticsquery.OpNe, Value: e.Rep1.String()}},
	})
	if err != nil {
		t.Fatalf("owner_id ne <self> was refused: %v", err)
	}
	if answer.PopulationNarrowed != narrowedToMeasurableOwners {
		t.Errorf("owner_id ne <self>: population_narrowed = %q, want %q",
			answer.PopulationNarrowed, narrowedToMeasurableOwners)
	}
}

// A scope the caller names is their question, on an install-wide report too:
// authorized, applied, and refused when it is outside their lens.
func TestATypedQuestionsNamedScopeIsAppliedToAnInstallWideReport(t *testing.T) {
	e := setupForecast(t)
	for range 7 {
		seedClosedWin(t, e, "Mine", e.Rep1, 1000)
		seedClosedWin(t, e, "Theirs", e.Rep3, 1000)
	}
	rep := e.typedReader(e.Rep1)
	count := analyticsquery.Query{
		Entity: "win-loss", Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
	}

	unscoped, err := e.askAnalytics(rep, t, count)
	if err != nil || len(unscoped.Rows) != 1 || fmt.Sprint(unscoped.Rows[0]["n"]) != "14" {
		t.Fatalf("the unscoped count = %+v (%v), want the installation's 14", unscoped.Rows, err)
	}
	mine := count
	mine.ScopeKind, mine.ScopeID = ScopeKindOwner, e.Rep1.String()
	answer, err := e.askAnalytics(rep, t, mine)
	if err != nil || len(answer.Rows) != 1 || fmt.Sprint(answer.Rows[0]["n"]) != "7" {
		t.Errorf("scoped to the rep, the count = %+v (%v), want their own 7", answer.Rows, err)
	}
	theirs := count
	theirs.ScopeKind, theirs.ScopeID = ScopeKindOwner, e.Rep3.String()
	if _, err := e.askAnalytics(rep, t, theirs); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("scoped to a colleague: err = %v, want the lens's refusal", err)
	}
}

// The explanation of a narrowed typed cell reads the same narrowed rows and
// says so.
func TestATypedExplanationCarriesItsAnswersNarrowing(t *testing.T) {
	e := setupForecast(t)
	for range 7 {
		seedClosedWin(t, e, "Mine", e.Rep1, 1000)
		seedClosedWin(t, e, "Theirs", e.Rep3, 1000)
	}
	rep := e.typedReader(e.Rep1)
	explanation, err := e.explainCell(rep, t, analyticsquery.Explain{
		Query: analyticsquery.Query{
			Entity: "win-loss", GroupBy: []string{"owner_id"},
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}},
		},
		Group: []any{e.Rep1.String()},
	})
	if err != nil {
		t.Fatalf("opening the rep's own cell was refused: %v", err)
	}
	if len(explanation.Rows) != 7 || explanation.PopulationNarrowed != narrowedToMeasurableOwners {
		t.Errorf("explanation = %d rows, narrowed %q; want 7 and %q",
			len(explanation.Rows), explanation.PopulationNarrowed, narrowedToMeasurableOwners)
	}
}
