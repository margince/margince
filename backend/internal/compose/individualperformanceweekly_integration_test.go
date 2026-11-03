// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The weekly job and the team week with "Show individual performance" off.
// No team week is frozen or served, and each rep's own week still is.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func showIndividualPerformanceIn(t *testing.T, e *integration.Env, on bool) {
	t.Helper()
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		RowScope: principal.RowScopeAll,
	})
	if err := config.Save(ctx, StoredConfig(e.Pool), identity.IndividualPerformance, on); err != nil {
		t.Fatalf("switching individual performance to %v: %v", on, err)
	}
}

// teamWeekRows counts the rows of every team-week table the schema holds, read
// from the catalog so a new per-member table joins the count by itself.
func teamWeekRows(t *testing.T, e *integration.Env) map[string]int {
	t.Helper()
	rows, err := e.Pool.Query(context.Background(),
		`SELECT table_name FROM information_schema.tables
		  WHERE table_schema = 'public' AND table_name LIKE 'team\_weekly%' ORDER BY table_name`)
	if err != nil {
		t.Fatalf("listing the team-week tables: %v", err)
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("reading a table name: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("listing the team-week tables: %v", err)
	}
	if len(tables) < 2 {
		t.Fatalf("the schema holds %d team-week tables: the census is short", len(tables))
	}
	out := map[string]int{}
	for _, table := range tables {
		out[table] = e.WsCount(t, `SELECT count(*) FROM `+table)
	}
	return out
}

func TestIndividualPerformanceOffFreezesNoTeamWeekAndKeepsEachRepsOwn(t *testing.T) {
	e := integration.Setup(t)
	w := teamSnapshotWorker(e)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	showIndividualPerformanceIn(t, e, false)

	if err := w.measureOneWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the weekly job with the switch off: %v", err)
	}
	for table, n := range teamWeekRows(t, e) {
		if n != 0 {
			t.Errorf("with the switch off the job wrote %d rows into %s", n, table)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM weekly_review`); got == 0 {
		t.Error("with the switch off no rep got their own week")
	}

	showIndividualPerformanceIn(t, e, true)
	if err := w.measureOneWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the weekly job with the switch on: %v", err)
	}
	if got := teamWeekRows(t, e)["team_weekly_review"]; got == 0 {
		t.Fatal("with the switch on the job froze no team week, so the case above proved nothing")
	}
}

// A week frozen while the switch was on is not served once it is off.
func TestIndividualPerformanceOffRefusesATeamWeekFrozenBefore(t *testing.T) {
	e := integration.Setup(t)
	w := teamSnapshotWorker(e)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	if err := w.measureOneWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the weekly job: %v", err)
	}
	lead := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := w.engine.LatestTeamReview(lead, e.Team1, nil); err != nil {
		t.Fatalf("a lead's team week with the switch on: %v", err)
	}

	showIndividualPerformanceIn(t, e, false)
	if _, err := w.engine.LatestTeamReview(lead, e.Team1, nil); !errors.As(err, new(auth.IndividualPerformanceOffError)) {
		t.Errorf("a lead's team week with the switch off = %v, want the individual-performance refusal", err)
	}
	if reach := auth.TeamWeekReachOf(auth.WithIndividualPerformance(lead, false)); reach != auth.ReachesNoTeam {
		t.Errorf("Home offers a team week with the switch off: reach %v", reach)
	}
}

// preparedTeamWeekly builds a lead's weekly mail carrying their team's week,
// with the switch on, the way the job prepares it before it submits.
func preparedTeamWeekly(t *testing.T, e *mailEnv) (ids.UUID, weeklyMailCandidate, time.Time) {
	t.Helper()
	week := previewWeek(t)
	now := reviewHour(week)
	seedRecipientOutcomes(t, e, week)
	scope := identity.RoutineTeam
	if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{Scope: &scope, TeamID: &e.Team1}); err != nil {
		t.Fatal(err)
	}
	review, _, err := e.worker.engine.AssembleFor(e.repCtx, now)
	if err != nil {
		t.Fatal(err)
	}
	members := []weekly.TeamMember{{UserID: e.Rep1, DisplayName: "Alex Example"}}
	if _, _, err := e.worker.engine.AssembleTeamFor(e.repCtx, e.Team1, "Northstar sales", members, now); err != nil {
		t.Fatal(err)
	}
	candidate, ready := e.worker.prepareWeeklyMail(e.repCtx, review.ID, now)
	if !ready || !candidate.namesTeam {
		t.Fatalf("the lead's weekly is ready %v and names the team %v, want both", ready, candidate.namesTeam)
	}
	return review.ID, candidate, now
}

// A switch turned off after the team's week was built, and before the mail
// goes, stops the mail: the setting is read again at submission.
func TestIndividualPerformanceOffBeforeSubmissionSendsNoTeamWeek(t *testing.T) {
	e := setupWeeklyMail(t)
	reviewID, candidate, now := preparedTeamWeekly(t, e)
	showIndividualPerformanceIn(t, e.Env, false)
	e.worker.sendWeeklyMail(e.repCtx, reviewID, candidate, now)
	if got := e.relay.count(); got != 0 {
		t.Fatalf("with the switch turned off before submission the relay was handed %d team weeks", got)
	}
}

func TestATeamWeeklyStillGoesWhileIndividualPerformanceStaysOn(t *testing.T) {
	e := setupWeeklyMail(t)
	reviewID, candidate, now := preparedTeamWeekly(t, e)
	e.worker.sendWeeklyMail(e.repCtx, reviewID, candidate, now)
	if got := e.relay.count(); got != 1 {
		t.Fatalf("with the switch on the relay was handed %d team weeks, want 1", got)
	}
}
