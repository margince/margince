// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// The weekly retrospective over real migrated Postgres.
//
// The two guarantees this suite exists for are the two the plan calls out, and
// neither is visible from a unit test: a weekly must never become "the latest
// morning brief", and a past week must survive its deals being deleted.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly/narrative"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// weekClock is a Wednesday, so "the week that just closed" is unambiguous and
// the fixture's own week is not the one under review.
var weekClock = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

type weekEnv struct {
	*integration.Env
	engine *Engine
	repCtx context.Context
}

func setupWeekly(t *testing.T) *weekEnv {
	t.Helper()
	e := integration.Setup(t)
	return &weekEnv{
		Env: e,
		// No membership seam: this suite is about the PER-REP weekly, which is
		// gated on being its own owner and never asks which team a reader is
		// on. A seam bound here would be wiring the tests need and production
		// does not exercise on this path.
		engine: NewEngine(e.Pool, nil),
		repCtx: e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms),
	}
}

// THE PLAN'S FIRST GUARANTEE. A weekly row must never be read as the latest
// brief: briefLastView orders brief_run by generated_at to decide the next
// morning's overnight window, so a Friday weekly landing there would silently
// reset what Saturday's brief counts as "changed overnight".
func TestAWeeklyNeverBecomesTheLatestMorningBrief(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)

	if _, _, err := e.engine.AssembleFor(e.repCtx, weekClock); err != nil {
		t.Fatalf("assembling the week: %v", err)
	}

	// The weekly wrote NOTHING into the brief's tables. Asserted against the
	// tables rather than against the brief's behaviour, because this is a
	// structural claim: the two aggregates do not share a row.
	var runs int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM brief_run`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Errorf("assembling a weekly wrote %d brief_run row(s) — a weekly that lands there "+
			"becomes the latest brief and resets the next morning's overnight window", runs)
	}
}

// THE PLAN'S SECOND GUARANTEE. Deleting a deal cascades brief_item rows away,
// which is why the weekly cannot hang off them. A past week that quietly loses
// a line because somebody cleaned up a deal is a record nobody can trust.
func TestAPastWeekSurvivesItsDealsBeingDeleted(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	// A deal won inside the week under review, seeded through the REAL writer:
	// a hand-inserted row is one the product never produces, and a test that
	// supplies its own version of production proves nothing about production.
	pipeline, _, _ := integration.DealFixture(t, e.Env)
	var stage ids.UUID
	if err := owner.QueryRow(ctx,
		`SELECT id FROM stage WHERE pipeline_id = $1 ORDER BY position LIMIT 1`, pipeline).
		Scan(&stage); err != nil {
		t.Fatal(err)
	}
	repOwner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Weber Rahmenvertrag", PipelineID: pipeline,
		StageID: ids.From[ids.StageKind](stage), Source: "manual", OwnerID: &repOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.UUID(created.Id)
	// Closed inside the week. The close itself is a column update because the
	// week under review is in the past and the writer stamps now().
	closedAt := weekClock.AddDate(0, 0, -4)
	if _, err := owner.Exec(ctx, `
		UPDATE deal SET status = 'won', closed_at = $2, amount_minor = 4200000,
		                currency = 'EUR', fx_rate_to_base = 1
		 WHERE id = $1`, dealID, closedAt); err != nil {
		t.Fatal(err)
	}

	review, wrote, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	if !wrote {
		t.Fatal("the first assembly reported the week already existed")
	}
	if review.Counts.DealsWon != 1 {
		t.Fatalf("the week counts %d won, want 1", review.Counts.DealsWon)
	}
	if len(review.Deals) != 1 || review.Deals[0].Label != "Weber Rahmenvertrag" {
		t.Fatalf("the week's deal lines = %+v, want the won deal by name", review.Deals)
	}

	// Now the deal is deleted outright — the case brief_item cannot survive.
	if _, err := owner.Exec(ctx, `DELETE FROM deal WHERE id = $1`, dealID); err != nil {
		t.Fatalf("deleting the deal: %v", err)
	}

	after, err := e.engine.LatestReview(e.repCtx, nil)
	if err != nil {
		t.Fatalf("reading the review after the deal was deleted: %v", err)
	}
	if after.Counts.DealsWon != 1 {
		t.Errorf("the past week now counts %d won, want 1 — deleting a deal rewrote history",
			after.Counts.DealsWon)
	}
	if len(after.Deals) != 1 {
		t.Fatalf("the past week now has %d deal line(s), want 1 — the line was cascaded away",
			len(after.Deals))
	}
	// And it still says what it said: the label was frozen, not joined.
	if after.Deals[0].Label != "Weber Rahmenvertrag" {
		t.Errorf("the surviving line reads %q, want the name the deal carried that week",
			after.Deals[0].Label)
	}
}

// A second run inside the same week reads the first rather than writing a
// second. The dispatcher ticks more than once on purpose so a worker that was
// down still backfills; the constraint is what makes that safe.
func TestASecondAssemblyInOneWeekReadsTheFirst(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)

	first, created, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil || !created {
		t.Fatalf("first assembly: created=%v err=%v", created, err)
	}
	second, createdAgain, err := e.engine.AssembleFor(e.repCtx, weekClock.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("second assembly: %v", err)
	}
	if createdAgain {
		t.Error("the second assembly wrote a second review for one week")
	}
	if second.ID != first.ID {
		t.Errorf("the second assembly returned review %s, want the first (%s)", second.ID, first.ID)
	}

	var reviews int
	if err := owner.QueryRow(context.Background(),
		`SELECT count(*) FROM weekly_review`).Scan(&reviews); err != nil {
		t.Fatal(err)
	}
	if reviews != 1 {
		t.Errorf("%d reviews exist for one rep-week, want 1", reviews)
	}
}

// Another rep's week is never served as mine.
func TestAnotherRepsWeekIsNeverServedAsMine(t *testing.T) {
	e := setupWeekly(t)

	if _, _, err := e.engine.AssembleFor(e.repCtx, weekClock); err != nil {
		t.Fatal(err)
	}
	rep2 := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := e.engine.LatestReview(rep2, nil); err == nil {
		t.Error("rep2 was served rep1's weekly review")
	}
}

// The review measures the week that CLOSED, not the one being lived. A
// retrospective of a week still in progress would be rewritten every day it
// ran.
func TestTheReviewMeasuresTheWeekThatClosed(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	var thisWeek time.Time
	if err := owner.QueryRow(context.Background(),
		`SELECT date_trunc('week', $1::timestamptz)::date`, weekClock).Scan(&thisWeek); err != nil {
		t.Fatal(err)
	}
	want := thisWeek.AddDate(0, 0, -7)
	if !review.LocalWeekStart.Equal(want) {
		t.Errorf("the review covers the week of %s, want the closed week of %s",
			review.LocalWeekStart.Format(time.DateOnly), want.Format(time.DateOnly))
	}
}

// The count and the list must agree. A deal that closed this week also has a
// stage change that closed it — counting that as a move told the rep one thing
// twice, and the list already excluded it, so the number and the lines
// disagreed by exactly the deals that mattered most.
func TestTheMovedCountAgreesWithTheMovedLines(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	pipeline, _, _ := integration.DealFixture(t, e.Env)
	var first, second ids.UUID
	rows, err := owner.Query(ctx,
		`SELECT id FROM stage WHERE pipeline_id = $1 ORDER BY position LIMIT 2`, pipeline)
	if err != nil {
		t.Fatal(err)
	}
	stages := []ids.UUID{}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		stages = append(stages, id)
	}
	rows.Close()
	if len(stages) < 2 {
		t.Fatalf("the fixture seeds %d stages, this test needs 2", len(stages))
	}
	first, second = stages[0], stages[1]

	repOwner := ids.From[ids.UserKind](e.Rep1)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Closed after moving", PipelineID: pipeline,
		StageID: ids.From[ids.StageKind](first), Source: "manual", OwnerID: &repOwner,
	})
	if err != nil {
		t.Fatal(err)
	}
	dealID := ids.UUID(deal.Id)
	inWeek := weekClock.AddDate(0, 0, -4)
	// It moved AND closed inside the week — the shape that double-counted.
	if _, err := owner.Exec(ctx, `
		INSERT INTO deal_stage_history (deal_id, from_stage_id, to_stage_id, changed_by, changed_at)
		VALUES ($1, $2, $3, 'human:x', $4)`, dealID, first, second, inWeek); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE deal SET status = 'won', closed_at = $2, amount_minor = 1000,
		                currency = 'EUR', fx_rate_to_base = 1
		 WHERE id = $1`, dealID, inWeek); err != nil {
		t.Fatal(err)
	}

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	var movedLines int
	for _, line := range review.Deals {
		if line.Outcome == OutcomeMoved {
			movedLines++
		}
	}
	if review.Counts.DealsMoved != movedLines {
		t.Errorf("the week counts %d moved but lists %d moved line(s) — the number and the "+
			"list disagree by the deals that also closed",
			review.Counts.DealsMoved, movedLines)
	}
	if review.Counts.DealsWon != 1 {
		t.Errorf("the closed deal counts %d won, want 1", review.Counts.DealsWon)
	}
}

// Delivered is scoped to what was promised. Counting everything finished in the
// week against everything due in it can print "4 of 2", which is not a number
// anybody can act on.
func TestDeliveredNeverExceedsPromised(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)

	// A task due BEFORE the week but finished inside it: real work, and not
	// part of this week's promise.
	before := weekClock.AddDate(0, 0, -20)
	inWeek := weekClock.AddDate(0, 0, -4)
	id := integration.SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, occurred_at, due_at, is_done, done_at,
		                      assignee_id, source, captured_by)
		VALUES ($1, 'task', 'older promise', $2, $2, true, $3, $4, 'manual', 'human:x')`,
		before, inWeek, e.Rep1)
	_ = id

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if review.Counts.TasksDone > review.Counts.TasksDue {
		t.Errorf("the week reports %d done of %d due — a ratio over one is not a reading",
			review.Counts.TasksDone, review.Counts.TasksDue)
	}
}

// The sentence is a SECOND write onto a committed review, and it is
// idempotent by replacement: a later pass is a correction, not an addition.
func TestTheWeeksSentenceIsWrittenAndReplaced(t *testing.T) {
	e := setupWeekly(t)

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	// Before any pass: no sentence and no stamp, which is what tells the
	// screen nobody looked.
	if review.Narrative != "" || review.NarratedAt != nil {
		t.Fatalf("a freshly measured week already carries a sentence: %+v", review)
	}

	for _, sentence := range []string{"First reading.", "Corrected reading."} {
		if err := e.engine.Narrate(e.repCtx, review.ID, sentence, weekClock); err != nil {
			t.Fatalf("narrating %q: %v", sentence, err)
		}
	}
	after, err := e.engine.LatestReview(e.repCtx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Narrative != "Corrected reading." {
		t.Errorf("the week reads %q, want only the corrected sentence", after.Narrative)
	}
	if after.NarratedAt == nil {
		t.Error("the narrated review carries no stamp")
	}
}

// A pass that ran and found the week unremarkable writes the STAMP with no
// sentence. Collapsing that into "no sentence" would make an honest quiet week
// indistinguishable from a week nobody looked at.
func TestAPassThatFoundNothingStillStampsTheWeek(t *testing.T) {
	e := setupWeekly(t)

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.engine.Narrate(e.repCtx, review.ID, "", weekClock); err != nil {
		t.Fatalf("narrating an empty sentence: %v", err)
	}
	after, err := e.engine.LatestReview(e.repCtx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Narrative != "" {
		t.Errorf("an empty sentence was stored as %q, want none", after.Narrative)
	}
	if after.NarratedAt == nil {
		t.Error("a pass that ran and found nothing left no stamp — indistinguishable from one that never ran")
	}
}

// Another rep's review is never narratable, even with its id in hand.
func TestAnotherRepsWeekCannotBeNarrated(t *testing.T) {
	e := setupWeekly(t)

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	rep2 := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	if err := e.engine.Narrate(rep2, review.ID, "not yours to say", weekClock); err == nil {
		t.Error("rep2 wrote a sentence onto rep1's week")
	}
	// And rep1's week is untouched.
	after, err := e.engine.LatestReview(e.repCtx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Narrative != "" || after.NarratedAt != nil {
		t.Errorf("the refused write still landed: %+v", after)
	}
}

// The writer refuses prose the column cannot hold, in characters rather than
// bytes — a caller that reaches Narrate without going through the parser would
// otherwise learn the ceiling from a driver error at 06:00 on a Monday.
func TestTheWriterRefusesProseTheColumnCannotHold(t *testing.T) {
	e := setupWeekly(t)

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	// Umlauts: at the ceiling in characters, well over it in bytes. A
	// byte-counted check would refuse this, and the column would not.
	atCeiling := strings.Repeat("ü", narrative.MaxNarrativeRunes)
	if err := e.engine.Narrate(e.repCtx, review.ID, atCeiling, weekClock); err != nil {
		t.Errorf("the writer refused %d characters the column holds: %v",
			narrative.MaxNarrativeRunes, err)
	}
	if err := e.engine.Narrate(e.repCtx, review.ID, atCeiling+"ü", weekClock); err == nil {
		t.Error("the writer accepted prose past the column's ceiling")
	}
}
