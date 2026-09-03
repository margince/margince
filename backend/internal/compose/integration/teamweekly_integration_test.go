// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// The team's frozen week over real migrated Postgres.
//
// What these defend is the freeze and the tier gate. The freeze is why the
// table exists at all: team membership moves, so a snapshot that re-summed on
// read would put a rep who joined later into an older team week and drop one
// who left, and a lead comparing two quarters would be comparing two different
// teams without being told.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamClock is a Wednesday, so the week that closed is unambiguous.
var teamClock = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

type teamEnv struct {
	*integration.Env
	engine  *weekly.Engine
	leadCtx context.Context
	repCtx  context.Context
}

func setupTeamWeekly(t *testing.T) *teamEnv {
	t.Helper()
	e := integration.Setup(t)
	return &teamEnv{
		Env: e,
		// Bound the way compose binds it. An engine WITHOUT the seam refuses
		// every team read, so a suite built on one would prove nothing about
		// which reader is admitted — and the real identity service, not a stub,
		// because the question here is what team_membership says.
		engine:  weekly.NewEngine(e.Pool, identity.NewService(e.Pool)),
		leadCtx: e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
		repCtx:  ownScoped(e, e.Rep2),
	}
}

// ownScoped is a seat that reaches only its own rows.
func ownScoped(e *integration.Env, user ids.UUID) context.Context {
	perms := integration.AdminPerms
	perms.RowScope = principal.RowScopeOwn
	return e.As(user, []ids.UUID{e.Team1}, perms)
}

// members names the two reps sharing Team1 in the harness.
func members(e *teamEnv) []weekly.TeamMember {
	return []weekly.TeamMember{
		{UserID: e.Rep1, DisplayName: "Rep One"},
		{UserID: e.Rep2, DisplayName: "Rep Two"},
	}
}

// A team snapshot is a team question. An own-scoped seat asking for one would
// get a page about people whose rows they cannot read.
func TestAnOwnScopedSeatIsRefusedTheTeamWeek(t *testing.T) {
	e := setupTeamWeekly(t)

	_, err := e.engine.LatestTeamReview(e.repCtx, e.Team1, nil)

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an own-scoped seat got %v, wanted a refusal", err)
	}
}

// teamScoped is a lead: they may read deals, and their row scope reaches a
// team. Every other test here runs under AdminPerms, which is RowScopeAll and
// short-circuits the membership question — which is exactly why the leak below
// stayed invisible until someone asked for a team they are not on.
func teamScoped(e *integration.Env, user ids.UUID, teams []ids.UUID) context.Context {
	perms := integration.AdminPerms
	perms.RowScope = principal.RowScopeTeam
	return e.As(user, teams, perms)
}

// A LEAD OF ONE TEAM MUST NOT READ ANOTHER'S WEEK. The team id arrives from the
// query string and nothing on the row narrows it to the reader, so without a
// membership gate one changed parameter hands over every member of another
// team: their name, their won deals, their breached leads, and the one thing
// their own lead was told to raise with them.
func TestALeadOfOneTeamCannotOpenAnothersWeek(t *testing.T) {
	e := setupTeamWeekly(t)

	// Team2 is Rep3's in the harness; Rep1 and Rep2 share Team1.
	writeRepWeek(t, e, e.Rep3)
	if _, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team2, "Team Two",
		[]weekly.TeamMember{{UserID: e.Rep3, DisplayName: "Rep Three"}}, teamClock); err != nil {
		t.Fatal(err)
	}

	outsider := teamScoped(e.Env, e.Rep1, []ids.UUID{e.Team1})

	_, err := e.engine.LatestTeamReview(outsider, e.Team2, nil)

	// NOT FOUND, not permission denied. A refusal that distinguished "this team
	// exists but is not yours" from "no such team" would let an outsider
	// enumerate the chart one id at a time — the same reason a row-scope miss
	// reads as 404 everywhere else in this tree.
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a lead read another team's week: got %v, wanted ErrNotFound", err)
	}
}

// AND THE SAME SEAT READS ITS OWN TEAM. Without this case the test above passes
// over an engine that refuses everyone, which is how a refusal test proves
// nothing at all.
func TestATeamScopedLeadReadsTheirOwnTeamsWeek(t *testing.T) {
	e := setupTeamWeekly(t)

	writeRepWeek(t, e, e.Rep1)
	if _, _, err := e.engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}

	lead := teamScoped(e.Env, e.Rep1, []ids.UUID{e.Team1})

	read, err := e.engine.LatestTeamReview(lead, e.Team1, nil)
	if err != nil {
		t.Fatalf("a lead reading their own team's week: %v", err)
	}
	if read.TeamName != "Team One" {
		t.Errorf("got the week of %q, wanted Team One", read.TeamName)
	}
}

// A MANAGEMENT SEAT OPENS A TEAM IT IS NOT ON. Row scope "all" reaches every
// row by definition, so asking membership of it would refuse a reader the
// row-scope predicate then admits.
//
// This case is why the gate short-circuits on auth.Unbounded rather than asking
// membership of everyone: removing that short-circuit leaves every OTHER test
// here green, because their readers happen to be members of the team they ask
// about. Only a reader who is deliberately not can tell the difference.
func TestAManagementSeatOpensATeamItIsNotOn(t *testing.T) {
	e := setupTeamWeekly(t)

	writeRepWeek(t, e, e.Rep3)
	if _, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team2, "Team Two",
		[]weekly.TeamMember{{UserID: e.Rep3, DisplayName: "Rep Three"}}, teamClock); err != nil {
		t.Fatal(err)
	}

	// Rep1 is on Team1 and nowhere near Team2 — AdminPerms is RowScopeAll.
	management := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)

	read, err := e.engine.LatestTeamReview(management, e.Team2, nil)
	if err != nil {
		t.Fatalf("a management seat reading another team's week: %v", err)
	}
	if read.TeamName != "Team Two" {
		t.Errorf("got the week of %q, wanted Team Two", read.TeamName)
	}
}

// The NAMED week takes the same gate as the newest one. They are separate entry
// points, and a gate written into one alone leaves "?week=2026-06-01" open.
func TestNamingTheWeekDoesNotBypassTheTeamGate(t *testing.T) {
	e := setupTeamWeekly(t)

	writeRepWeek(t, e, e.Rep3)
	written, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team2, "Team Two",
		[]weekly.TeamMember{{UserID: e.Rep3, DisplayName: "Rep Three"}}, teamClock)
	if err != nil {
		t.Fatal(err)
	}

	outsider := teamScoped(e.Env, e.Rep1, []ids.UUID{e.Team1})

	// The week that was actually written, so a refusal cannot be a miss on the
	// date rather than the gate doing its job.
	_, err = e.engine.TeamReview(outsider, e.Team2, written.LocalWeekStart)

	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("naming another team's week: got %v, wanted ErrNotFound", err)
	}
}

// An engine with NO membership seam refuses a team read rather than serving it
// unchecked. An unbound seam is a wiring mistake, and the failure mode of
// serving anyway is handing every lead every team's week.
func TestAnUnboundMembershipSeamRefusesTheTeamWeek(t *testing.T) {
	e := setupTeamWeekly(t)

	writeRepWeek(t, e, e.Rep1)
	if _, _, err := e.engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}

	unwired := weekly.NewEngine(e.Pool, nil)
	lead := teamScoped(e.Env, e.Rep1, []ids.UUID{e.Team1})

	if _, err := unwired.LatestTeamReview(lead, e.Team1, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an unbound seam served a team week: got %v, wanted ErrNotFound", err)
	}
}

// The membership is frozen INTO the snapshot, which is the whole reason the
// table exists rather than a view.
//
// A rep who leaves the team afterwards stays in the week they were part of, and
// a rep who joins later does not appear in it.
func TestTheMembershipIsFrozenIntoTheWeek(t *testing.T) {
	e := setupTeamWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	// Both reps must have a week to be counted at all.
	writeRepWeek(t, e, e.Rep1)
	writeRepWeek(t, e, e.Rep2)

	written, created, err := e.engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	if !created || len(written.Reps) != 2 {
		t.Fatalf("the week was written with %d reps (created=%v), wanted 2 — "+
			"without both the freeze below proves nothing", len(written.Reps), created)
	}

	// Rep2 leaves the team, and is renamed.
	if _, err := owner.Exec(ctx,
		`DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`,
		e.Team1, e.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`UPDATE app_user SET display_name = 'Someone Else' WHERE id = $1`, e.Rep2); err != nil {
		t.Fatal(err)
	}

	read, err := e.engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(read.Reps) != 2 {
		t.Fatalf("the frozen week came back with %d reps after one left, wanted 2", len(read.Reps))
	}
	var found bool
	for _, rep := range read.Reps {
		if rep.UserID == e.Rep2 {
			found = true
			if rep.DisplayName != "Rep Two" {
				t.Errorf("the departed rep reads as %q, wanted the name they had that week",
					rep.DisplayName)
			}
		}
	}
	if !found {
		t.Error("a rep who left the team vanished from the week they were part of")
	}
}

// A second run answers the first snapshot rather than rewriting a week a lead
// may already have read.
//
// The dispatcher ticks more than once inside a week on purpose, so a worker
// that was down still backfills — which means this path runs repeatedly over
// the same team week by design, not by accident.
func TestASecondAssemblyReadsTheFirst(t *testing.T) {
	e := setupTeamWeekly(t)

	writeRepWeek(t, e, e.Rep1)
	first, created, err := e.engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("the first assembly wrote nothing")
	}

	second, createdAgain, err := e.engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}

	if createdAgain {
		t.Error("a second assembly wrote a second snapshot for one team week")
	}
	if second.ID != first.ID {
		t.Errorf("the second answered snapshot %v, wanted the first's %v", second.ID, first.ID)
	}
}

// A member with no review for the week is counted as UNREAD, not as a zero row.
//
// A rep on leave and a rep whose measurement failed look identical as zeros,
// and only one of those is a fact about their week.
func TestAMemberWithNoWeekIsCountedAsUnread(t *testing.T) {
	e := setupTeamWeekly(t)

	// Only Rep1 has a week.
	writeRepWeek(t, e, e.Rep1)

	review, _, err := e.engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}

	if review.Counts.RepsCounted != 1 {
		t.Errorf("counted %d reps, wanted 1", review.Counts.RepsCounted)
	}
	if review.RepsUnread != 1 {
		t.Errorf("reported %d unread, wanted 1 — a snapshot silently covering "+
			"one of two reps reads exactly like a team of one", review.RepsUnread)
	}
	if len(review.Reps) != 1 {
		t.Errorf("wrote %d rep rows, wanted only the one with a week", len(review.Reps))
	}
}

// writeRepWeek gives one rep a review for the week that closed.
func writeRepWeek(t *testing.T, e *teamEnv, user ids.UUID) {
	t.Helper()
	repCtx := e.As(user, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, _, err := e.engine.AssembleFor(repCtx, teamClock); err != nil {
		t.Fatalf("writing a week for %v: %v", user, err)
	}
}
