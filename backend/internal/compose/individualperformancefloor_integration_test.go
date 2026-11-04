// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The small-group floor over the difference between two teams, with the switch
// off, through the real report engine, typed query, forecast, reporting
// framework and targets. Membership is written as the fixtures of the switch's
// other suites write it.

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// nestedTeams is the handbook's example. Sales (Team1) is the reader Rep1,
// Anna, Ben and Chris; North is Rep1, Anna and Ben. Only Anna and Ben sell, so
// Sales minus North is Chris, who sold nothing.
type nestedTeams struct {
	north, anna, ben, chris ids.UUID
}

func (e *forecastEnv) seedNestedTeams(t *testing.T) nestedTeams {
	t.Helper()
	members := e.addMembers(t, e.Team1, "anna@floor.test", "ben@floor.test", "chris@floor.test")
	out := nestedTeams{anna: members[0], ben: members[1], chris: members[2]}
	out.north = e.seedTeam(t, "North", e.Rep1, out.anna, out.ben)
	e.seedWinIn(t, out.anna, "EUR")
	e.seedWinIn(t, out.ben, "EUR")
	return out
}

// seedTeam writes a live team with these members.
func (e *forecastEnv) seedTeam(t *testing.T, name string, members ...ids.UUID) ids.UUID {
	t.Helper()
	team := e.seedID(t, `INSERT INTO team (id, name) VALUES ($1, $2)`, name)
	e.joinTeam(t, team, members...)
	return team
}

// joinTeam seats existing members on a team. The suites of a company share with
// the switch off seat its issuer on one. Someone on no team would be all that
// the company minus a team leaves, so the floor would withhold the share.
func (e *forecastEnv) joinTeam(t *testing.T, team ids.UUID, members ...ids.UUID) {
	t.Helper()
	for _, member := range members {
		if _, err := e.owner.Exec(context.Background(),
			`INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, team, member); err != nil {
			t.Fatalf("adding a member to a team: %v", err)
		}
	}
}

func teamScopeBody(team ids.UUID) string {
	return `{"scope":{"kind":"team","id":"` + team.String() + `"}}`
}

func TestIndividualPerformanceOffWithholdsATeamThatANestedTeamLeavesToOneMember(t *testing.T) {
	e := setupForecast(t)
	teams := e.seedNestedTeams(t)
	reader := e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll)
	if status, code := e.reportRefusal(reader, t, "win-loss", teamScopeBody(e.Team1)); status != http.StatusOK {
		t.Fatalf("Sales with the switch on = %d %q, want 200", status, code)
	}

	e.showIndividualPerformance(t, false)
	status, code := e.reportRefusal(reader, t, "win-loss", teamScopeBody(e.Team1))
	requireRefusedIndividual(t, "Sales, which minus North is one member who sold nothing", status, code)
	if status, code := e.reportRefusal(reader, t, "win-loss", teamScopeBody(teams.north)); status != http.StatusOK {
		t.Errorf("North with the switch off = %d %q, want 200", status, code)
	}
	status, code = e.reportRefusal(reader, t, "win-loss", `{"scope":{"kind":"workspace"}}`)
	requireRefusedIndividual(t, "the company, which minus Sales is one colleague", status, code)

	chris := e.dealReadCtx(teams.chris, nil, principal.RowScopeAll)
	if status, code := e.reportRefusal(chris, t, "win-loss", teamScopeBody(e.Team1)); status != http.StatusOK {
		t.Errorf("Sales read by the one member North leaves out = %d %q, want 200", status, code)
	}

	e.joinTeam(t, teams.north, teams.chris)
	if status, code := e.reportRefusal(reader, t, "win-loss", teamScopeBody(e.Team1)); status != http.StatusOK {
		t.Errorf("Sales once Chris joins North = %d %q, want 200 on the next request", status, code)
	}
}

func TestIndividualPerformanceOffDoesNotSubtractAnArchivedTeam(t *testing.T) {
	e := setupForecast(t)
	teams := e.seedNestedTeams(t)
	reader := e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll)
	e.showIndividualPerformance(t, false)
	if _, err := e.owner.Exec(context.Background(), `UPDATE team SET archived_at = now() WHERE id = $1`, teams.north); err != nil {
		t.Fatal(err)
	}
	if status, code := e.reportRefusal(reader, t, "win-loss", teamScopeBody(e.Team1)); status != http.StatusOK {
		t.Errorf("Sales beside an archived North = %d %q, want 200", status, code)
	}
}

func TestIndividualPerformanceOffWithholdsATypedQueryOverANestedTeamsParent(t *testing.T) {
	e := setupForecast(t)
	teams := e.seedNestedTeams(t)
	amount := int64(100_000)
	for _, owner := range []ids.UUID{teams.anna, teams.ben} {
		e.seedOpenDeal(t, "Open", 20, &owner, &amount, nil)
	}
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))
	of := func(team ids.UUID) analyticsquery.Query {
		return analyticsquery.Query{
			Entity: "open-deals-per-company", ScopeKind: ScopeKindTeam, ScopeID: team.String(),
			Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
		}
	}
	e.showIndividualPerformance(t, false)
	if _, err := e.askAnalytics(reader, t, of(e.Team1)); !errors.As(err, new(auth.IndividualPerformanceOffError)) {
		t.Errorf("a typed query over Sales = %v, want the individual-performance refusal", err)
	}
	if _, err := e.askAnalytics(reader, t, of(teams.north)); err != nil {
		t.Errorf("a typed query over North = %v, want an answer", err)
	}
}

// Three teams that share members pairwise: together they give each
// colleague's figure, so each is withheld.
func TestIndividualPerformanceOffWithholdsThreePairwiseTeamsForecasts(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team1, "anna@floor.test", "ben@floor.test")
	anna, ben := members[0], members[1]
	chris := e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'chris@floor.test', 'Chris')`)
	second := e.seedTeam(t, "Second", e.Rep1, anna, chris)
	third := e.seedTeam(t, "Third", e.Rep1, ben, chris)
	for _, owner := range []ids.UUID{anna, ben, chris} {
		e.seedDealClosingWithin(t, "Theirs", &owner, 100_000)
	}
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))
	assemble := func(team ids.UUID) error {
		t.Helper()
		return forecasting.NewStore(InstallationDB(e.Pool)).InTx(reader, func(ctx context.Context, tx pgx.Tx) error {
			period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, clocktest.Now(t))
			if err != nil {
				return err
			}
			_, err = ForecastAssemble(ctx, tx, period, forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &team}, clocktest.Now(t), base)
			return err
		})
	}
	for _, team := range []ids.UUID{e.Team1, second, third} {
		if err := assemble(team); err != nil {
			t.Fatalf("a pairwise team's forecast with the switch on: %v", err)
		}
	}
	e.showIndividualPerformance(t, false)
	for _, team := range []ids.UUID{e.Team1, second, third} {
		if err := assemble(team); !errors.As(err, new(auth.IndividualPerformanceOffError)) {
			t.Errorf("the forecast of pairwise team %s = %v, want the individual-performance refusal", team, err)
		}
	}
}

func TestIndividualPerformanceOffWithholdsANestedTeamsParentReadings(t *testing.T) {
	e := setupForecast(t)
	teams := e.seedNestedTeams(t)
	service := newReportingService(e.Pool, func() time.Time { return time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC) })
	won := func(team ids.UUID) crmcontracts.ReportingMetric {
		t.Helper()
		result, err := service.Evaluate(reportingActor(e), crmcontracts.ReportingSelection{
			Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(team)}, Period: "this_quarter",
			TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.Metrics[0]
	}
	e.showIndividualPerformance(t, false)
	sales := won(e.Team1)
	if sales.Value != nil || !sales.Coverage.Withheld || sales.Coverage.Reason == nil ||
		!strings.Contains(*sales.Coverage.Reason, "another team's members are taken out") {
		t.Errorf("Sales's sales won = %v %+v, want withheld with the difference named", sales.Value, sales.Coverage)
	}
	if north := won(teams.north); north.Value == nil || *north.Value != 2000 {
		t.Errorf("North's sales won = %v, want 2000", north.Value)
	}
}

func TestIndividualPerformanceOffWithholdsANestedTeamsParentTarget(t *testing.T) {
	e := setupForecast(t)
	teams := e.seedNestedTeams(t)
	service := newReportingService(e.Pool, func() time.Time { return time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC) })
	admin := reportingActor(e)
	target := func(team ids.UUID) crmcontracts.ReportingTarget {
		t.Helper()
		created, err := service.CreateTarget(admin, crmcontracts.ReportingTargetInput{
			Metric: "meetings_held", Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(team)},
			PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
			Value: 20, Reason: "Month plan",
		})
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	sales, north := target(e.Team1), target(teams.north)
	e.showIndividualPerformance(t, false)
	if listsTarget(admin, t, service, sales) {
		t.Error("Sales's target is listed although Sales minus North is one member")
	}
	if !listsTarget(admin, t, service, north) {
		t.Error("North's target is withheld")
	}
}

// The floor subtracts the teams whose totals a reader may ask for, and no others.
func TestTheFloorSubtractsTheTeamsTheScopeResolverAccepts(t *testing.T) {
	e := setupForecast(t)
	seat := e.addMembers(t, e.Team1, "seat@floor.test")[0]
	e.seedTeam(t, "Second", seat)
	archived := e.seedTeam(t, "Archived", seat)
	if _, err := e.owner.Exec(context.Background(), `UPDATE team SET archived_at = now() WHERE id = $1`, archived); err != nil {
		t.Fatal(err)
	}
	reader := e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll)
	err := InstallationDB(e.Pool).Tx(reader, func(tx pgx.Tx) error {
		subtracted, err := readableTeamMembers(reader, tx, []ids.UUID{seat})
		if err != nil {
			return err
		}
		var all []ids.UUID
		if err := tx.QueryRow(reader, `SELECT array_agg(team_id) FROM team_membership WHERE user_id = $1`, seat).Scan(&all); err != nil {
			return err
		}
		for _, team := range all {
			_, resolveErr := ResolveAnalyticsScope(reader, tx, RequestedScope{Kind: ScopeKindTeam, ID: &team})
			if accepted, inFloor := resolveErr == nil, slices.Contains(subtracted[seat], team); accepted != inFloor {
				t.Errorf("team %s: the resolver accepts it %v (%v), the floor subtracts it %v", team, accepted, resolveErr, inFloor)
			}
		}
		if len(all) != 3 {
			t.Errorf("the seat is on %d teams, want 3", len(all))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
