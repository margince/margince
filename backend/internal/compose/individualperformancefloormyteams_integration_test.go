// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An edition captured as a lead's "My teams" holds the lead's population. A
// reader with other teams is judged on the roster it kept, never on their own.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reportingSeat is reportingActor as another seat, row scope and teams. A
// publication reads its owner's grants from a role, so the role is seeded.
func reportingSeat(t *testing.T, e *forecastEnv, user ids.UUID, scope principal.RowScope, teams []ids.UUID) context.Context {
	t.Helper()
	actor, ok := principal.Actor(reportingActor(e))
	if !ok {
		t.Fatal("missing fixture principal")
	}
	actor.ID, actor.UserID, actor.TeamIDs, actor.Permissions.RowScope = "human:"+user.String(), user, teams, scope
	raw, err := json.Marshal(struct {
		Objects  map[string]principal.ObjectGrant `json:"objects"`
		RowScope principal.RowScope               `json:"row_scope"`
	}{actor.Permissions.Objects, scope})
	if err != nil {
		t.Fatal(err)
	}
	role := e.seedID(t, `INSERT INTO role(id,key,name,permissions) VALUES($1,$2,'Reporting seat',$3)`, "reporting-"+user.String(), raw)
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO role_assignment(role_id,user_id) VALUES($1,$2)`, role, user); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE app_user SET seat_type='full' WHERE id=$1`, user); err != nil {
		t.Fatal(err)
	}
	return principal.WithActor(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7()), actor)
}

func TestIndividualPerformanceOffJudgesAMyTeamsEditionOnTheRosterItKept(t *testing.T) {
	e := setupForecast(t)
	members := e.addMembers(t, e.Team1, "anna@floor.test", "ben@floor.test", "chris@floor.test")
	e.seedWinIn(t, members[0], "EUR")
	e.seedWinIn(t, members[1], "EUR")
	lead := reportingSeat(t, e, e.Rep1, principal.RowScopeTeam, []ids.UUID{e.Team1})
	reader := reportingSeat(t, e, e.Rep3, principal.RowScopeAll, []ids.UUID{e.Team2})
	at := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	report, err := service.CreateReport(lead, crmcontracts.ReportingReportInput{Name: "My teams", Audience: "workspace", Selection: crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: crmcontracts.ReportingScopeKindManagedTeams}, Period: "this_quarter",
		TargetBasis: "fiscal_quarter", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Freeze(lead, ids.UUID(report.Id), report.Revision, "my-teams")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&reportScheduleSweepWorker{pool: e.Pool, now: func() time.Time { return at }}).Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	completed, err := service.GetExecution(lead, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("the edition was not captured: %+v %v", completed, err)
	}
	edition := ids.UUID(*completed.EditionId)
	won := func() *float64 {
		t.Helper()
		read, err := service.GetEdition(reader, edition)
		if err != nil {
			t.Fatal(err)
		}
		return read.Evaluation.Metrics[0].Value
	}

	e.showIndividualPerformance(t, false)
	captured := won()
	if captured == nil || *captured == 0 {
		t.Fatalf("the lead's edition read by another team's member = %v, want the captured total", captured)
	}
	north := e.seedTeam(t, "North", e.Rep1, members[0], members[1])
	if shown := won(); shown != nil && *shown == *captured {
		t.Errorf("the lead's edition beside North = %v; the roster it kept minus North is one member, so it is withheld", *shown)
	}
	if _, err := e.owner.Exec(context.Background(), `DELETE FROM team WHERE id = $1`, north); err != nil {
		t.Fatal(err)
	}
	if shown := won(); shown == nil || *shown != *captured {
		t.Fatalf("the lead's edition once North is gone = %v, want %v", shown, *captured)
	}
	if _, err := e.owner.Exec(context.Background(), `UPDATE report_edition
		SET manifest = jsonb_set(manifest, '{evaluation,context,member_ids}', '[]'::jsonb) WHERE id = $1`, edition); err != nil {
		t.Fatal(err)
	}
	if shown := won(); shown != nil && *shown == *captured {
		t.Errorf("a My teams edition whose roster is gone = %v, want it withheld", *shown)
	}
}
