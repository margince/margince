// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Whether the team snapshot job runs at all.
//
// It drives snapshotTeams — the production entry point — rather than a copy of
// its queries. That is the whole point: liveTeams filtered on an app_user
// column that does not exist, so every run of this job errored on its first
// statement and team_weekly_review was written by nothing. Nothing failed,
// because nothing tested it: the table's own suite calls AssembleTeamFor
// directly and never reaches the query that finds the teams to call it for.

import (
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// teamJobClock is a Wednesday, so the week that closed is unambiguous.
var teamJobClock = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

// teamSnapshotWorker builds the worker the way addWeeklyReviewJobs does, so a
// wiring mistake there is a wiring mistake here. The narrator and the mail
// relay are omitted on purpose: both are absent by design in an installation
// without them, and neither is what this test is about.
func teamSnapshotWorker(e *integration.Env) *weeklyGenerateWorker {
	return &weeklyGenerateWorker{
		engine: weekly.NewEngine(e.Pool, newTeammatesSeam(e.Pool)),
		pool:   e.Pool,
		users:  identity.NewService(e.Pool),
		now:    func() time.Time { return teamJobClock },
		log:    slog.Default(),
	}
}

// seedManagerRoles gives every harness seat the role a Team Lead has.
//
// The harness inserts app_user rows with no role assignment at all, and the job
// resolves each acting seat's authority for real — so without this every team
// is skipped as "a team whose members' roles do not grant reading deals", and
// the job reports success having written nothing. That is a true refusal, and a
// test that accepted it would be asserting the skip rather than the snapshot.
func seedManagerRoles(t *testing.T, e *integration.Env, users ...ids.UUID) {
	t.Helper()
	// The harness seeds no RBAC catalog at all, so the role is created here
	// with the shape migration 1788244324 gave the seeded manager: deal:read
	// and a row scope that reaches a team. Both halves matter — the job needs
	// the object grant to assemble and the team scope to re-read.
	e.WsExec(t, `INSERT INTO role (key, name, permissions)
	             VALUES ('team_lead_under_test', 'Team Lead', $1::jsonb)`,
		`{"objects":{"deal":{"read":true},"person":{"read":true},`+
			`"activity":{"read":true},"installation_settings":{"read":true}},`+
			`"row_scope":"team"}`)
	for _, user := range users {
		e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
		             SELECT r.id, $1 FROM role r WHERE r.key = 'team_lead_under_test'`, user)
	}
}

// THE JOB RUNS AND WRITES A ROW. Asserted through snapshotTeams rather than
// AssembleTeamFor, because everything between the two is what was broken: the
// query that lists the teams, the seat it acts as, and the members it counts.
func TestTheTeamSnapshotJobWritesAWeek(t *testing.T) {
	e := integration.Setup(t)
	w := teamSnapshotWorker(e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)

	// Each member needs their own week first — a team week is a total over
	// them, and a team whose members have no reviews counts every one unread.
	for _, rep := range []ids.UUID{e.Rep1, e.Rep2} {
		if _, _, err := w.engine.AssembleFor(
			e.As(rep, []ids.UUID{e.Team1}, integration.AdminPerms), teamJobClock,
		); err != nil {
			t.Fatalf("writing %v's week: %v", rep, err)
		}
	}

	if failures := w.snapshotTeams(ctx, e.WS, teamJobClock); len(failures) > 0 {
		t.Fatalf("the team snapshot job failed: %v", failures)
	}

	if got := e.WsCount(t, `SELECT count(*) FROM team_weekly_review`); got == 0 {
		t.Fatal("the job reported success and wrote no team week at all")
	}
}

// AND A SECOND TICK DOES NOT FAIL. The dispatcher runs every few hours, so this
// path re-reads a snapshot it has already written by design. That re-read takes
// the team gate, under a MEMBER's authority rather than the system principal —
// so a worker whose engine has no membership seam refuses its own snapshot from
// the second tick onward, and the workspace job fails forever.
func TestASecondTickOfTheTeamSnapshotJobSucceeds(t *testing.T) {
	e := integration.Setup(t)
	w := teamSnapshotWorker(e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)

	for _, rep := range []ids.UUID{e.Rep1, e.Rep2} {
		if _, _, err := w.engine.AssembleFor(
			e.As(rep, []ids.UUID{e.Team1}, integration.AdminPerms), teamJobClock,
		); err != nil {
			t.Fatalf("writing %v's week: %v", rep, err)
		}
	}
	if failures := w.snapshotTeams(ctx, e.WS, teamJobClock); len(failures) > 0 {
		t.Fatalf("the first tick failed: %v", failures)
	}

	if failures := w.snapshotTeams(ctx, e.WS, teamJobClock); len(failures) > 0 {
		t.Fatalf("the second tick failed: %v", failures)
	}

	// One row per team, not one per tick.
	if got := e.WsCount(t, `SELECT count(*) FROM team_weekly_review`); got != 2 {
		t.Errorf("after two ticks there are %d team weeks, wanted one per team (2)", got)
	}
}
