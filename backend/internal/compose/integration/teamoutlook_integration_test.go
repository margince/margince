// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// Where the TEAM's week was landing, over real migrated Postgres.
//
// The one thing worth proving here is that the team's outlook is read over the
// TEAM's book. It is not the sum of its members': a deal owned by nobody on the
// team is in neither, and one the team works but a member owns is in both. A
// team snapshot that quietly took a rep's landing would look identical on the
// page and be a different number.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// teamScopeSpy is a ForecastWeek that records WHICH book it was asked for.
//
// A stub is right here: the question is which scope the team path resolves, and
// the arithmetic behind a landing belongs to forecasting's own suite. What this
// cannot fake is the id — the seam is handed a team id or it is not.
type teamScopeSpy struct {
	repCalls  int
	teamCalls int
	askedFor  ids.UUID
	outlooks  []weekly.Outlook
}

func (s *teamScopeSpy) CloseWeek(
	_ context.Context, _ pgx.Tx, _, _ time.Time,
) ([]weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	s.repCalls++
	return s.outlooks, nil, nil, nil
}

func (s *teamScopeSpy) CloseTeamWeek(
	_ context.Context, _ pgx.Tx, teamID ids.UUID, _, _ time.Time,
) ([]weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	s.teamCalls++
	s.askedFor = teamID
	return s.outlooks, nil, nil, nil
}

func oneTeamHorizon() []weekly.Outlook {
	return []weekly.Outlook{{
		PeriodKind:     "quarter",
		PeriodStart:    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		BaseCurrency:   "EUR",
		WonMinor:       180_000_00,
		CommitMinor:    120_000_00,
		BestCaseMinor:  390_000_00,
		WeightedMinor:  260_000_00,
		ForwardMeasure: "commit_evidence",
		Closing: weekly.OutlookSide{
			Known: true, SnapshotID: ids.NewV7(), LandingMinor: 300_000_00,
		},
	}}
}

// THE TEAM OUTLOOK IS READ OVER THE TEAM'S BOOK, and the seam is asked for the
// team being snapshotted rather than for whoever happened to be assembling it.
func TestTheTeamOutlookFreezesTheTeamScopeSnapshot(t *testing.T) {
	e := setupTeamWeekly(t)
	spy := &teamScopeSpy{outlooks: oneTeamHorizon()}
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithForecast(spy)

	written, created, err := engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("the first assembly of a team week writes it")
	}
	if spy.teamCalls != 1 {
		t.Fatalf("the team path asks the team seam once, got %d calls", spy.teamCalls)
	}
	if spy.repCalls != 0 {
		t.Fatal("a team snapshot must not take a rep's landing: the two are different books")
	}
	if spy.askedFor != e.Team1 {
		t.Fatalf("the seam was asked for %s, wanted the team being snapshotted (%s)",
			spy.askedFor, e.Team1)
	}
	if len(written.Outlook) != 1 {
		t.Fatalf("the returned snapshot carries its outlook, got %d horizons", len(written.Outlook))
	}
}

// FROZEN, and readable after the snapshots it was read from are gone. The whole
// reason the figures are copied rather than pointed at.
func TestTheTeamOutlookSurvivesItsSnapshots(t *testing.T) {
	e := setupTeamWeekly(t)
	spy := &teamScopeSpy{outlooks: oneTeamHorizon()}
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithForecast(spy)

	if _, _, err := engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	// Retention removes the snapshots. The copied figures must stand.
	e.WsExec(t, `DELETE FROM forecast_snapshot`)

	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Outlook) != 1 {
		t.Fatalf("a retrospective that empties out is not a record, got %d horizons",
			len(read.Outlook))
	}
	if read.Outlook[0].WonMinor != 180_000_00 {
		t.Fatalf("the frozen figure must read back unchanged, got %d", read.Outlook[0].WonMinor)
	}
}

// AN ABSENT SEAM LEAVES THE SNAPSHOT WITHOUT AN OUTLOOK, and that is not a
// failure: an installation that composed no forecast still gets its team week.
func TestAnAbsentForecastSeamStillWritesTheTeamWeek(t *testing.T) {
	e := setupTeamWeekly(t)
	// setupTeamWeekly builds the engine WITHOUT a forecast, which is the state
	// under test.
	written, created, err := e.engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatalf("a team with no forecast lane still has a week: %v", err)
	}
	if !created {
		t.Fatal("the first assembly writes the week")
	}
	if len(written.Outlook) != 0 {
		t.Fatalf("no forecast composed means no outlook, got %d", len(written.Outlook))
	}
}

// refusingForecast is a seam that refuses, the way forecasting's scope
// authority refuses a team the caller is not on.
type refusingForecast struct{ calls int }

func (r *refusingForecast) CloseWeek(
	_ context.Context, _ pgx.Tx, _, _ time.Time,
) ([]weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	return nil, nil, nil, apperrors.ErrNotFound
}

func (r *refusingForecast) CloseTeamWeek(
	_ context.Context, _ pgx.Tx, _ ids.UUID, _, _ time.Time,
) ([]weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	r.calls++
	return nil, nil, nil, apperrors.ErrNotFound
}

// A REFUSED OUTLOOK COSTS THE OUTLOOK, NOT THE WEEK.
//
// forecasting's scope authority refuses ScopeTeam to a caller who is not in the
// team, and that refusal arrives INSIDE the transaction that has already
// written the snapshot and its reps. Letting it escape rolls all of that back,
// so a lead whose row scope is narrower than the team they are snapshotting
// loses the whole retrospective rather than just its landing — the counts, the
// membership, the focus lines, everything.
//
// The rep-level review had exactly this bug once. This is the team-level one.
func TestARefusedTeamOutlookStillLeavesTheWeekWritten(t *testing.T) {
	e := setupTeamWeekly(t)
	refusing := &refusingForecast{}
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithForecast(refusing)

	written, created, err := engine.AssembleTeamFor(
		e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatalf("a refused landing must not cost the team its week: %v", err)
	}
	if !created {
		t.Fatal("the week is still written")
	}
	if refusing.calls != 1 {
		t.Fatalf("the seam was asked once, got %d", refusing.calls)
	}
	if len(written.Outlook) != 0 {
		t.Fatalf("a refused landing is absent, got %d horizons", len(written.Outlook))
	}

	// And it is READABLE, which is what the rollback destroyed: the row exists
	// and answers for the week it was written about. Reps are counted as
	// UNREAD here because this fixture seeds no member weeks — that is the
	// snapshot working, and the figure only exists because the row survived.
	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatalf("the snapshot must still be there to read: %v", err)
	}
	if read.TeamName != "Team One" {
		t.Fatalf("the frozen snapshot was rolled back with the outlook, got %q", read.TeamName)
	}
	if read.RepsUnread != len(members(e)) {
		t.Fatalf("the membership pass ran and recorded %d unread of %d",
			read.RepsUnread, len(members(e)))
	}
}
