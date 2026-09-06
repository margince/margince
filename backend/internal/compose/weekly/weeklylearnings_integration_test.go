// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// What a week taught, over real migrated Postgres.
//
// The three things a unit test cannot see: the write happens once and a second
// pass leaves it alone, a week that was READ and yielded nothing is stamped
// rather than left looking unexamined, and a review's learnings are reachable
// only by the rep whose week it was.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly/learnings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// grounded is a learning that cites one row, which is the minimum this lane
// accepts.
func grounded(text string) learnings.Learning {
	return learnings.Learning{
		Kind: learnings.KindWorked, Text: text,
		Citations: []learnings.Citation{
			{SubjectType: "deal", SubjectID: ids.NewV7(), Label: "Nordwind expansion"},
		},
	}
}

// A WEEK THAT WAS READ AND YIELDED NOTHING IS STAMPED, not left looking
// unexamined. This is the distinction the state column exists for: a rep whose
// week nobody looked at and one whose week held no lesson must not read alike.
func TestInsufficientEvidenceIsStampedNotSilent(t *testing.T) {
	e := setupWeekly(t)
	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if review.LearningsState != LearningsNotRun {
		t.Fatalf("a fresh review has not been learned from, got %q", review.LearningsState)
	}

	wrote, err := e.engine.RecordLearnings(e.repCtx, review.ID, nil, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("the first pass over a week writes its verdict")
	}

	read, err := e.engine.LatestReview(e.repCtx, &review.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if read.LearningsState != LearningsInsufficient {
		t.Fatalf("a pass that ran and found nothing stamps insufficient_evidence, got %q",
			read.LearningsState)
	}
	if len(read.Learnings) != 0 {
		t.Fatalf("nothing was learned, so nothing is stored, got %d", len(read.Learnings))
	}
}

// Learnings are written once and never replaced.
//
// Held by: TestLearningsAreWrittenOnceAndNeverReplaced
// (backend/internal/compose/weekly/weeklylearnings_integration_test.go)
//
// The dispatcher ticks more than
// once inside a week so a worker that was down still backfills, and a learning
// a rep read on Monday must not become a different claim on Tuesday.
func TestLearningsAreWrittenOnceAndNeverReplaced(t *testing.T) {
	e := setupWeekly(t)
	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	first := grounded("Reaching the sponsor early won Nordwind.")
	if _, err := e.engine.RecordLearnings(e.repCtx, review.ID,
		[]learnings.Learning{first}, weekClock); err != nil {
		t.Fatal(err)
	}

	// A second pass, with something else to say.
	second := grounded("Something entirely different.")
	wrote, err := e.engine.RecordLearnings(e.repCtx, review.ID,
		[]learnings.Learning{second}, weekClock)
	if err != nil {
		t.Fatalf("a second pass is a no-op, not a failure: %v", err)
	}
	if wrote {
		t.Fatal("a week that has been learned from is not learned from again")
	}

	read, err := e.engine.LatestReview(e.repCtx, &review.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Learnings) != 1 {
		t.Fatalf("wanted the first pass's one learning, got %d", len(read.Learnings))
	}
	if read.Learnings[0].Text != first.Text {
		t.Fatalf("the rep must still read what they read on Monday, got %q", read.Learnings[0].Text)
	}
}

// ANOTHER REP'S WEEK CANNOT RECEIVE LEARNINGS. The review id alone must not
// reach somebody else's week — the owner check is the row scope.
func TestAnotherRepsWeekCannotReceiveLearnings(t *testing.T) {
	e := setupWeekly(t)
	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	// Rep2 shares a team with Rep1 and still may not write their week.
	other := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	_, err = e.engine.RecordLearnings(other, review.ID,
		[]learnings.Learning{grounded("Not mine to write.")}, weekClock)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("another rep's review reads as absent, got %v", err)
	}

	read, err := e.engine.LatestReview(e.repCtx, &review.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if read.LearningsState != LearningsNotRun {
		t.Fatalf("the refused write must leave the week untouched, got %q", read.LearningsState)
	}
}

// A LATER TICK LEARNS A WEEK THAT HAS NO STAMP. The backfill case: a week
// assembled while the lane was unbound is still learnable when it comes back.
func TestALaterTickLearnsAWeekThatHasNoStamp(t *testing.T) {
	e := setupWeekly(t)
	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	// Days later, with the lane composed.
	wrote, err := e.engine.RecordLearnings(e.repCtx, review.ID,
		[]learnings.Learning{grounded("Weber went quiet after one contact.")},
		weekClock.AddDate(0, 0, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("a week nobody had looked at is still learnable")
	}

	read, err := e.engine.LatestReview(e.repCtx, &review.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if read.LearningsState != LearningsSynthesized {
		t.Fatalf("wanted synthesized, got %q", read.LearningsState)
	}
	if len(read.Learnings) != 1 || len(read.Learnings[0].Citations) != 1 {
		t.Fatalf("the learning and its citation must both survive, got %+v", read.Learnings)
	}
	if read.Learnings[0].Citations[0].Label != "Nordwind expansion" {
		t.Fatalf("a citation keeps the label it was written with, got %q",
			read.Learnings[0].Citations[0].Label)
	}
}
