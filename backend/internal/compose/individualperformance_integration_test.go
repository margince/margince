// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The "Show individual performance" switch through the real report engine, the
// typed query, the scope resolver and the reporting framework's owner rule, in
// both states. The switch is written through the audited settings writer and
// read by the engine inside its own transaction, as a job reads it.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// showIndividualPerformance writes the switch as an admin's settings change does.
func (e *forecastEnv) showIndividualPerformance(t *testing.T, on bool) {
	t.Helper()
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:settings-admin", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if err := config.Save(ctx, StoredConfig(e.Pool), identity.IndividualPerformance, on); err != nil {
		t.Fatalf("switching individual performance to %v: %v", on, err)
	}
}

// reportRefusal runs a report and answers its status and problem code.
func (e *forecastEnv) reportRefusal(ctx context.Context, t *testing.T, report, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/"+report, strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.handlers.RunReport(rec, req, report)
	code := ""
	if rec.Code != http.StatusOK {
		var problem struct {
			Code string `json:"code"`
		}
		decodeWire(t, rec, rec.Code, &problem)
		code = problem.Code
	}
	return rec.Code, code
}

// addMembers seats more live colleagues on a team. While the switch is off a
// total covers three members or more. A suite measuring the whole company needs
// them beside the harness's two reps.
func (e *forecastEnv) addMembers(t *testing.T, team ids.UUID, emails ...string) []ids.UUID {
	t.Helper()
	members := make([]ids.UUID, 0, len(emails))
	for _, email := range emails {
		member := e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Member')`, email)
		if _, err := e.owner.Exec(context.Background(),
			`INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, team, member); err != nil {
			t.Fatalf("adding a member to the team: %v", err)
		}
		members = append(members, member)
	}
	return members
}

// seedWinIn plants one won deal in a currency, so a filter on the currency
// can isolate who sold it.
func (e *forecastEnv) seedWinIn(t *testing.T, owner ids.UUID, currency string) {
	t.Helper()
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency, status, closed_at, lost_reason, fx_rate_to_base, source, captured_by)
		VALUES ($1, 'Win', $2, $3, $4, 1000, $5, 'won', '2026-01-15T10:00:00Z'::timestamptz, NULL, 1.0, 'manual', 'human:x')`,
		e.pipeline, e.stages[60], owner, currency)
}

const individualOffCode = "individual_performance_off"

func requireRefusedIndividual(t *testing.T, what string, status int, code string) {
	t.Helper()
	if status != http.StatusForbidden || code != individualOffCode {
		t.Errorf("%s with individual performance off = %d %q, want 403 %q", what, status, code, individualOffCode)
	}
}

func TestIndividualPerformanceOffRefusesABreakdownByOwnerAndKeepsARepsOwn(t *testing.T) {
	e := setupForecast(t)
	seedWinsByThreeOwners(t, e)
	e.addMembers(t, e.Team2, "third@forecast.test")
	e.showIndividualPerformance(t, false)

	status, code := e.reportRefusal(e.Admin(), t, "win-loss", winLossByOwner)
	requireRefusedIndividual(t, "an admin's breakdown by owner", status, code)

	rep := e.dealReadCtx(e.Rep1, nil, principal.RowScopeOwn)
	got := wonByOwner(t, e.runReport(rep, t, "win-loss", winLossByOwner).Rows)
	if len(got) != 2 || got[e.Rep1.String()] != 10000 || got[""] != 2000 {
		t.Fatalf("a rep's own breakdown with the switch off = %v, want their own 10000 and the unowned 2000", got)
	}

	e.showIndividualPerformance(t, true)
	if got := wonByOwner(t, e.runReport(e.Admin(), t, "win-loss", winLossByOwner).Rows); len(got) != 3 {
		t.Fatalf("an admin's breakdown with the switch back on = %v, want all three owners", got)
	}
}

func TestIndividualPerformanceOffRefusesAFilterOrScopeNamingAColleague(t *testing.T) {
	e := setupForecast(t)
	seedWinsByThreeOwners(t, e)
	e.addMembers(t, e.Team2, "third@forecast.test")
	admin := e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll)
	byColleague := `{"filters":{"owner_id":"` + e.Rep3.String() + `"}}`
	ofColleague := `{"scope":{"kind":"owner","id":"` + e.Rep3.String() + `"}}`
	ofSelf := `{"scope":{"kind":"owner","id":"` + e.Rep1.String() + `"}}`

	for _, body := range []string{byColleague, ofColleague, ofSelf} {
		if status, _ := e.reportRefusal(admin, t, "win-loss", body); status != http.StatusOK {
			t.Fatalf("with the switch on, %s = %d, want 200", body, status)
		}
	}
	e.showIndividualPerformance(t, false)
	for _, body := range []string{byColleague, ofColleague} {
		status, code := e.reportRefusal(admin, t, "win-loss", body)
		requireRefusedIndividual(t, body, status, code)
	}
	if status, _ := e.reportRefusal(admin, t, "win-loss", ofSelf); status != http.StatusOK {
		t.Errorf("an admin's own scope with the switch off = %d, want 200", status)
	}
}

// A grouping by the seat a meeting is credited to is a breakdown by colleague too.
func TestIndividualPerformanceOffRefusesAGroupingByMeetingHost(t *testing.T) {
	e := setupForecast(t)
	e.addMembers(t, e.Team2, "third@forecast.test")
	reader := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true}, "lead": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	byHost := `{"group_by":["host_user_id"]}`
	if status, code := e.reportRefusal(reader, t, "meeting-conversion", byHost); status != http.StatusOK {
		t.Fatalf("a breakdown by host with the switch on = %d %q, want 200", status, code)
	}
	e.showIndividualPerformance(t, false)
	status, code := e.reportRefusal(reader, t, "meeting-conversion", byHost)
	requireRefusedIndividual(t, "a breakdown by host", status, code)
}

func TestIndividualPerformanceOffRefusesATypedQueryByOwner(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for range 7 {
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, &amount, nil)
	}
	e.addMembers(t, e.Team2, "third@forecast.test")
	byOwner := analyticsquery.Query{
		Entity: "open-deals-per-company", GroupBy: []string{"owner_id"},
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "deals"}},
	}
	if _, err := e.askAnalytics(e.reportReaderCtx(), t, byOwner); err != nil {
		t.Fatalf("a typed breakdown by owner with the switch on: %v", err)
	}
	e.showIndividualPerformance(t, false)
	if _, err := e.askAnalytics(e.reportReaderCtx(), t, byOwner); !errors.As(err, new(auth.IndividualPerformanceOffError)) {
		t.Errorf("a typed breakdown by owner with the switch off = %v, want the individual-performance refusal", err)
	}
}

// A filter can leave a total that one colleague makes up, even on a team of
// three. So the floor counts who contributes to the answer, not who is on the
// team.
func TestIndividualPerformanceOffRefusesAFilteredTotalOneColleagueMakesUp(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team1, "two@forecast.test", "three@forecast.test")
	e.seedWinIn(t, e.Rep1, "EUR")
	e.seedWinIn(t, members[0], "EUR")
	e.seedWinIn(t, members[1], "EUR")
	lead := e.dealReadCtx(e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeTeam)
	team := `"scope":{"kind":"team","id":"` + e.Team1.String() + `"}`
	e.showIndividualPerformance(t, false)

	if status, code := e.reportRefusal(lead, t, "win-loss", `{`+team+`}`); status != http.StatusOK {
		t.Fatalf("the team's total over two other contributors = %d %q, want 200", status, code)
	}
	// win-loss answers per currency, so a currency one colleague sold in is
	// such a group even with no filter.
	e.seedWinIn(t, members[0], "USD")
	status, code := e.reportRefusal(lead, t, "win-loss", `{`+team+`,"filters":{"currency":"USD"}}`)
	requireRefusedIndividual(t, "a total only one colleague sold into", status, code)
	status, code = e.reportRefusal(lead, t, "win-loss",
		`{`+team+`,"group_by":["currency"],"aggregates":[{"fn":"sum","field":"amount_minor","as":"won"}]}`)
	requireRefusedIndividual(t, "a breakdown with a group one colleague sold into", status, code)
}

// A departed colleague's sales still belong to them, so they do not lift a
// total over the floor.
func TestIndividualPerformanceOffDoesNotCountADepartedColleague(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team1, "two@forecast.test", "three@forecast.test")
	e.seedWinIn(t, members[0], "EUR")
	e.seedWinIn(t, members[1], "EUR")
	team := `{"scope":{"kind":"team","id":"` + e.Team1.String() + `"}}`
	e.showIndividualPerformance(t, false)

	if status, code := e.reportRefusal(e.Admin(), t, "win-loss", team); status != http.StatusOK {
		t.Fatalf("a total over two live colleagues = %d %q, want 200", status, code)
	}
	e.deactivate(t, members[1])
	status, code := e.reportRefusal(e.Admin(), t, "win-loss", team)
	requireRefusedIndividual(t, "a total over one live and one departed colleague", status, code)
}

// The reporting framework marks every colleague's fact withheld, on the rule
// live readings, captured editions and evidence all share.
func TestIndividualPerformanceOffWithholdsEveryColleaguesFact(t *testing.T) {
	e := setupForecast(t)
	admin := e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll)
	marked := func() []reporting.Fact {
		t.Helper()
		facts := []reporting.Fact{{OwnerID: e.Rep1}, {OwnerID: e.Rep3}, {}}
		if err := InstallationDB(e.Pool).Tx(admin, func(tx pgx.Tx) error {
			owners, err := readerMeasurableOwners(admin, tx)
			if err == nil {
				markOwnerWithheld(owners, facts)
			}
			return err
		}); err != nil {
			t.Fatalf("marking withheld owners: %v", err)
		}
		return facts
	}
	if facts := marked(); facts[1].OwnerWithheld {
		t.Fatal("with the switch on, an admin may not measure a colleague")
	}
	e.showIndividualPerformance(t, false)
	facts := marked()
	if facts[0].OwnerWithheld || !facts[1].OwnerWithheld || facts[2].OwnerWithheld {
		t.Errorf("with the switch off, withheld = own %v, colleague %v, unowned %v; want false, true, false",
			facts[0].OwnerWithheld, facts[1].OwnerWithheld, facts[2].OwnerWithheld)
	}
}

// exportShare downloads a frozen share's CSV as reader.
func (e *forecastEnv) exportShare(reader context.Context, t *testing.T, token string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/forecast/shared/"+token+"/export.csv", nil).WithContext(reader)
	rec := httptest.NewRecorder()
	e.shareRoutes().ExportForecastShare(rec, req, token)
	return rec.Code, rec.Body.String()
}

// A frozen company forecast's CSV lists each deal's owner. With the switch off
// it lists only the reader's own deals, on the same resolver the screen reads
// through.
func TestIndividualPerformanceOffClearsColleaguesFromAShareExport(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team2, "two@forecast.test", "three@forecast.test")
	e.seedDealClosingWithin(t, "Mine", &e.Rep1, 100_000)
	for _, owner := range append([]ids.UUID{e.Rep3}, members...) {
		e.seedDealClosingWithin(t, "Theirs", &owner, 250_000)
	}
	id := e.freezeWorkspace(t)
	_, token := e.issue(e.writerCtx(t), t, NewShare{
		Kind: shareKindSnapshot, Target: "someone@example.test",
		Scope: forecasting.Scope{Kind: forecasting.ScopeWorkspace}, SnapshotID: &id,
		ExpiresAt: clocktest.Now(t).UTC().Add(24 * time.Hour),
	})
	reader := e.forecastReader(e.dealReadCtx(e.Rep1, nil, principal.RowScopeAll))

	status, body := e.exportShare(reader, t, token)
	if status != http.StatusOK || !strings.Contains(body, e.Rep3.String()) {
		t.Fatalf("with the switch on the export = %d and names the colleague %v, want 200 and yes", status, strings.Contains(body, e.Rep3.String()))
	}
	e.showIndividualPerformance(t, false)
	status, body = e.exportShare(reader, t, token)
	if status != http.StatusOK {
		t.Fatalf("with the switch off the export = %d, want 200", status)
	}
	if !strings.Contains(body, e.Rep1.String()) {
		t.Error("with the switch off the export dropped the reader's own owner id")
	}
	for _, colleague := range append([]ids.UUID{e.Rep3}, members...) {
		if strings.Contains(body, colleague.String()) {
			t.Errorf("with the switch off the export names colleague %s", colleague)
		}
	}
	if strings.Contains(body, "Theirs") || strings.Count(body, "\n") != 2 {
		t.Errorf("with the switch off the export lists a colleague's deal, want the header and the reader's own row:\n%s", body)
	}
}

// A team of the reader and one colleague has a target that is in effect that
// colleague's own. It is withheld while the switch is off.
func TestIndividualPerformanceOffWithholdsATwoMemberTeamsTarget(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	reportingTeammate(t, e)
	target, err := service.CreateTarget(reportingActor(e), crmcontracts.ReportingTargetInput{
		Metric: "meetings_held", Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)},
		PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		Value: 15, Reason: "Month plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	rep := reportingRepSeat(e)
	listed := func() bool {
		t.Helper()
		list, err := service.ListTargets(rep, nil, 100, reporting.TargetFilter{})
		if err != nil {
			t.Fatal(err)
		}
		return slices.ContainsFunc(list.Data, func(listed crmcontracts.ReportingTarget) bool { return listed.Id == target.Id })
	}
	if !listed() {
		t.Fatal("with the switch on the rep's own team target is not listed")
	}
	if _, err := service.GetTarget(rep, ids.UUID(target.Id)); err != nil {
		t.Fatalf("with the switch on the rep's own team target = %v", err)
	}

	e.showIndividualPerformance(t, false)
	if listed() {
		t.Error("with the switch off a two-member team's target is still listed")
	}
	if _, err := service.GetTarget(rep, ids.UUID(target.Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("with the switch off a two-member team's target = %v, want not found", err)
	}
}

// listsTarget answers whether the reader's target list holds target.
func listsTarget(reader context.Context, t *testing.T, service *reporting.Service, target crmcontracts.ReportingTarget) bool {
	t.Helper()
	list, err := service.ListTargets(reader, nil, 100, reporting.TargetFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(list.Data, func(listed crmcontracts.ReportingTarget) bool { return listed.Id == target.Id })
}

// The target floor checks archived teams as it checks live ones. It lists only
// teams with enough colleagues, so a small archived team's target stays withheld.
func TestIndividualPerformanceOffFloorChecksArchivedTeams(t *testing.T) {
	e := setupForecast(t)
	reportingTeammate(t, e)
	second := e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'second@forecast.test', 'Second')`)
	for _, member := range []ids.UUID{e.Rep1, e.Rep3, second} {
		if _, err := e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, e.Team2, member); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE team SET archived_at = now() WHERE id = ANY($1)`, []ids.UUID{e.Team1, e.Team2}); err != nil {
		t.Fatal(err)
	}
	e.showIndividualPerformance(t, false)
	admin := reportingActor(e)
	var floor reporting.TargetFloor
	if err := InstallationDB(e.Pool).Tx(admin, func(tx pgx.Tx) error {
		var err error
		floor, err = targetFloor(admin, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !floor.Applies || slices.Contains(floor.Teams, e.Team1) || !slices.Contains(floor.Teams, e.Team2) {
		t.Errorf("floor = %+v; want the archived team of two withheld (%v) and the archived team of three shown (%v)", floor, e.Team1, e.Team2)
	}
}

// A company of the reader and one live colleague has a target that is in
// effect that colleague's own. It is withheld while the switch is off.
func TestIndividualPerformanceOffWithholdsATwoMemberCompanysTarget(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	admin := reportingActor(e)
	target, err := service.CreateTarget(admin, crmcontracts.ReportingTargetInput{
		Metric: "meetings_held", Scope: crmcontracts.ReportingScope{Kind: "workspace"},
		PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		Value: 40, Reason: "Month plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	reportingTeammate(t, e)
	e.showIndividualPerformance(t, false)
	if !listsTarget(admin, t, service, target) {
		t.Fatal("with the switch off a company of three members does not show its target")
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE app_user SET archived_at = now()
		WHERE NOT is_agent AND id <> ALL($1)`, []ids.UUID{e.Rep1, e.Rep3}); err != nil {
		t.Fatal(err)
	}
	if listsTarget(admin, t, service, target) {
		t.Error("with the switch off a company of the reader and one colleague still lists its target")
	}
}

// A team of the reader and one colleague shows none of its totals while the
// switch is off. That holds even for a total made up of the reader's own deal,
// because the rest of the team is one colleague. Two more live members lift it.
func TestIndividualPerformanceOffWithholdsATwoMemberTeamsReadings(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	reportingTeammate(t, e)
	e.seedWinIn(t, e.Rep1, "EUR")
	reader := reportingActor(e)
	selection := crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, Period: "this_quarter",
		TargetBasis: "fiscal_quarter", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "meetings_held"},
	}
	won := func() crmcontracts.ReportingMetric {
		t.Helper()
		result, err := service.Evaluate(reader, selection)
		if err != nil {
			t.Fatal(err)
		}
		for _, metric := range result.Metrics {
			if metric.Coverage.Withheld && metric.Id != "bookings_won" && metric.Value != nil {
				t.Errorf("%s is withheld but carries a value", metric.Id)
			}
			if metric.Id == "bookings_won" {
				return metric
			}
		}
		t.Fatal("the evaluation has no bookings_won")
		return crmcontracts.ReportingMetric{}
	}
	if metric := won(); metric.Value == nil || *metric.Value != 1000 {
		t.Fatalf("with the switch on, the team's sales won = %v, want 1000", metric.Value)
	}

	e.showIndividualPerformance(t, false)
	metric := won()
	if metric.Value != nil || !metric.Coverage.Withheld || metric.Coverage.Status != "unavailable" {
		t.Errorf("with the switch off, a two-member team's sales won = %v %+v, want withheld with no value", metric.Value, metric.Coverage)
	}
	if metric.Coverage.Reason == nil || !strings.Contains(*metric.Coverage.Reason, "fewer than two other members") {
		t.Errorf("the withheld reading's reason = %v, want the team floor", metric.Coverage.Reason)
	}

	e.addMembers(t, e.Team1, "floor-a@forecast.test", "floor-b@forecast.test")
	if metric := won(); metric.Value == nil || *metric.Value != 1000 {
		t.Errorf("with three other members, the team's sales won = %v, want 1000", metric.Value)
	}
}

// A thin team's movement chart is withheld with its metric: the evaluation
// reads no captured movement for it and keeps none of its facts. The same two
// captures show movement while the switch is on.
func TestIndividualPerformanceOffWithholdsATwoMemberTeamsMovement(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return at }
	service := newReportingService(e.Pool, now)
	reader := reportingActor(e)
	reportingTeammate(t, e)
	capture := crmcontracts.ReportingCaptureContext{Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, PipelineId: ptrUUID(e.pipeline)}
	framework, err := service.PublishFramework(reader, 0, crmcontracts.ReportingFrameworkInput{Template: "sales", Reason: "Capture the team", Qualification: []crmcontracts.ReportingQualification{}, CaptureContexts: []crmcontracts.ReportingCaptureContext{capture}})
	if err != nil {
		t.Fatal(err)
	}
	writer := principal.WithCorrelationID(principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:movement-fixture"), ids.NewV7())
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).WithClock(now)
	owner := ids.From[ids.UserKind](e.Rep1)
	closeDate := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	amount, currency := int64(10000), "EUR"
	deal, err := store.CreateDeal(writer, deals.CreateDealInput{Name: "Movement source", AmountMinor: &amount, Currency: &currency, ExpectedClose: &closeDate, PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.stages[20]), OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	worker := &forecastSnapshotSweepWorker{pool: e.Pool, now: now, log: slog.New(slog.DiscardHandler)}
	if err := worker.snapshotWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	at = at.AddDate(0, 0, 7)
	amount = 13000
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	if err := worker.snapshotWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	selection := crmcontracts.ReportingSelection{Scope: capture.Scope, PipelineId: capture.PipelineId, Period: "this_month", TargetBasis: "month", CloseWindow: "fiscal_quarter", Metrics: []crmcontracts.ReportingMetricID{"open_pipeline"}, Blocks: []crmcontracts.ReportingBlockKind{"pipeline_movement"}}
	evaluate := func() (crmcontracts.ReportingChart, int) {
		t.Helper()
		var out reporting.Evaluation
		db := InstallationDB(e.Pool)
		if err := db.Tx(reader, func(tx pgx.Tx) error {
			var err error
			out, err = metricEvaluator{forecast: newForecastStore(db)}.Evaluate(reader, tx, selection, framework, at)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		movement := 0
		for _, fact := range out.Facts {
			if strings.HasPrefix(fact.ContextID, "movement") {
				movement++
			}
		}
		for _, chart := range out.Result.Charts {
			if chart.Kind == "pipeline_movement" {
				return chart, movement
			}
		}
		t.Fatal("the evaluation has no movement chart")
		return crmcontracts.ReportingChart{}, 0
	}
	if chart, facts := evaluate(); len(chart.Points) == 0 || facts == 0 {
		t.Fatalf("with the switch on, the team's movement = %d points and %d facts, want both", len(chart.Points), facts)
	}

	e.showIndividualPerformance(t, false)
	chart, facts := evaluate()
	if !chart.Coverage.Withheld || len(chart.Points) != 0 || chart.CaptureStatus != nil || facts != 0 {
		t.Errorf("a thin team's movement = %d points, %d facts, capture %v, withheld %v; want nothing read and withheld",
			len(chart.Points), facts, chart.CaptureStatus, chart.Coverage.Withheld)
	}
}
