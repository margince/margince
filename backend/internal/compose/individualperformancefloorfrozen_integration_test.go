// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The small-group floor over what was frozen before today's teams: a snapshot
// movement and a retained edition are judged by today's membership on every
// read. And a floored report reads its membership and its figures in one
// snapshot.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// freezeScope freezes one population's forecast through the real writer, as
// the capture does.
func (e *forecastEnv) freezeScope(t *testing.T, scope forecasting.Scope) ids.UUID {
	t.Helper()
	store := forecasting.NewStore(InstallationDB(e.Pool))
	now := clocktest.Now(t)
	var id ids.UUID
	if err := store.InTx(snapshotWriterCtx(e.WS), func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, now)
		if err != nil {
			return err
		}
		assembly, err := ForecastAssemble(ctx, tx, period, scope, now, base)
		if err != nil {
			return err
		}
		snapshot := forecasting.NewSnapshot{
			Period: period, Scope: scope, Trigger: forecasting.TriggerCall, BaseCurrency: base,
			Readings: assembly.Readings, TakenAt: now,
		}
		snapshot.StampChances(assembly)
		id, err = store.TakeSnapshot(ctx, tx, snapshot)
		return err
	}); err != nil {
		t.Fatalf("freezing a forecast: %v", err)
	}
	return id
}

func TestIndividualPerformanceOffWithholdsTheMovementOfANestedTeamsParent(t *testing.T) {
	e := setupForecast(t)
	teams := e.seedNestedTeams(t)
	for _, owner := range []ids.UUID{teams.anna, teams.ben} {
		e.seedDealClosingWithin(t, "Open", &owner, 100_000)
	}
	sales := forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &e.Team1}
	north := forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &teams.north}
	salesFrom, salesTo := e.freezeScope(t, sales), e.freezeScope(t, sales)
	northFrom, northTo := e.freezeScope(t, north), e.freezeScope(t, north)
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))
	store := newForecastStore(InstallationDB(e.Pool))

	e.showIndividualPerformance(t, false)
	if _, err := store.Movement(reader, forecasting.ReadingOpen, salesFrom, salesTo); !errors.As(err, new(auth.IndividualPerformanceOffError)) {
		t.Errorf("Sales's movement = %v, want the individual-performance refusal", err)
	}
	if _, err := store.Movement(reader, forecasting.ReadingOpen, northFrom, northTo); err != nil {
		t.Errorf("North's movement = %v, want an answer", err)
	}
}

// An edition captured while its team was safe is withheld once a new team
// would leave one of its members on their own.
func TestIndividualPerformanceOffWithholdsAnEditionANewTeamWouldExpose(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team1, "anna@floor.test", "ben@floor.test", "chris@floor.test")
	e.seedWinIn(t, members[0], "EUR")
	e.seedWinIn(t, members[1], "EUR")
	reportingWorkerIdentity(t, e)
	at := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	human := reportingActor(e)
	report, err := service.CreateReport(human, crmcontracts.ReportingReportInput{Name: "Sales", Audience: "workspace", Selection: crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, Period: "this_quarter",
		TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Freeze(human, ids.UUID(report.Id), report.Revision, "sales")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&reportScheduleSweepWorker{pool: e.Pool, now: func() time.Time { return at }}).Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	completed, err := service.GetExecution(human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("the edition was not captured: %+v %v", completed, err)
	}
	read := func() (crmcontracts.ReportingEdition, string) {
		t.Helper()
		edition, err := service.GetEdition(human, ids.UUID(*completed.EditionId))
		if err != nil {
			t.Fatal(err)
		}
		csv, err := reportingCSV(edition.Evaluation)
		if err != nil {
			t.Fatal(err)
		}
		return edition, string(csv)
	}

	e.showIndividualPerformance(t, false)
	safe, safeCSV := read()
	won := safe.Evaluation.Metrics[0].Value
	if won == nil || !strings.Contains(safeCSV, reportingCSVNumber(won)) {
		t.Fatalf("the safe edition's sales won = %v; want it shown in the view and the export", won)
	}
	e.seedTeam(t, "North", e.Rep1, members[0], members[1])
	exposed, exposedCSV := read()
	shown := exposed.Evaluation.Metrics[0].Value
	if !exposed.Withheld || (shown != nil && *shown == *won) || strings.Contains(exposedCSV, reportingCSVNumber(won)) {
		t.Errorf("the edition beside North = withheld %v, sales won %v; want the captured %v gone from the view and the export",
			exposed.Withheld, shown, *won)
	}
	evidence, err := service.EditionEvidence(human, ids.UUID(*completed.EditionId), "bookings_won", "bookings_won", "", nil, nil, 100)
	if err != nil || len(evidence.Rows) != 0 {
		t.Errorf("the withheld edition's evidence = %d rows, %v; want none", len(evidence.Rows), err)
	}
}

// A floored report reads its membership and its figures in one
// `REPEATABLE READ` snapshot. A door below it is refused rather than answered.
func TestAFlooredReportRunsInOneSnapshot(t *testing.T) {
	e := setupForecast(t)
	teams := e.seedNestedTeams(t)
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))
	question := analyticsquery.Query{
		Entity: "open-deals-per-company", ScopeKind: ScopeKindTeam, ScopeID: teams.north.String(),
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
	}
	e.showIndividualPerformance(t, false)
	ask := func(level pgx.TxIsoLevel) error {
		return InstallationDB(e.Pool).TxIsolated(reader, level, func(tx pgx.Tx) error {
			_, err := RunAnalyticsQuery(reader, tx, question, analyticsquery.DefaultFloor)
			return err
		})
	}
	if err := ask(pgx.ReadCommitted); !errors.Is(err, errFloorOutsideOneSnapshot) {
		t.Errorf("a floored query under READ COMMITTED = %v, want it refused", err)
	}
	if err := ask(pgx.RepeatableRead); err != nil {
		t.Errorf("a floored query under REPEATABLE READ = %v, want an answer", err)
	}
}

// A share of a team of the reader and one colleague answers that colleague's
// figures. While the switch is off, opening it and its export are refused.
func TestIndividualPerformanceOffRefusesATwoMemberTeamsShare(t *testing.T) {
	e := setupForecast(t)
	teammate := reportingTeammate(t, e)
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, 100_000)
	e.seedDealClosingWithin(t, "Theirs", &teammate, 250_000)
	team := forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &e.Team1}
	snapshot := e.freezeScope(t, team)
	expires := clocktest.Now(t).UTC().Add(24 * time.Hour)
	_, frozen := e.issue(e.writerCtx(t), t, NewShare{
		Kind: shareKindSnapshot, Target: "someone@example.test", Scope: team, SnapshotID: &snapshot, ExpiresAt: expires,
	})
	_, live := e.issue(e.writerCtx(t), t, NewShare{Kind: "live", Target: "forecast", Scope: team, ExpiresAt: expires})
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))
	open := func(token string) (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/v1/forecast/shared/"+token, nil).WithContext(reader)
		rec := httptest.NewRecorder()
		e.shareRoutes().OpenForecastShare(rec, req, token)
		return rec.Code, rec.Body.String()
	}
	reads := map[string]func() (int, string){
		"the snapshot share":  func() (int, string) { return open(frozen) },
		"the live share":      func() (int, string) { return open(live) },
		"the snapshot export": func() (int, string) { return e.exportShare(reader, t, frozen) },
	}
	for what, read := range reads {
		if status, body := read(); status != http.StatusOK {
			t.Errorf("with the switch on, %s = %d %s, want 200", what, status, body)
		}
	}
	e.showIndividualPerformance(t, false)
	for what, read := range reads {
		if status, body := read(); status != http.StatusForbidden || !strings.Contains(body, individualOffCode) {
			t.Errorf("with the switch off, %s of a two-member team = %d %s, want 403 %s", what, status, body, individualOffCode)
		}
	}
}
