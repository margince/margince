// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// The scorecard over real migrated Postgres.
//
// These cases are here rather than in a unit test because the thing under test
// IS the SQL: the lead block reads status movement out of audit_log's
// before/after images, which only exist because the real writers put them
// there. A test that hand-inserted its own audit rows would prove that this
// query can read rows this test knows how to write, and nothing about whether
// production writes them that way.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedLeadFor mints a lead through the real writer and returns its id.
func seedLeadFor(t *testing.T, e *weekEnv, name string, owner ids.UUID) ids.LeadID {
	t.Helper()
	ownerID := ids.From[ids.UserKind](owner)
	lead, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{
		FullName: &name, Status: "new", OwnerID: &ownerID, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding lead %s: %v", name, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(lead.Id))
}

// moveLead walks a lead up the ladder through UpdateLead, the writer whose
// audit images the scorecard reads.
func moveLead(t *testing.T, e *weekEnv, id ids.LeadID, to string) {
	t.Helper()
	if _, err := e.People.UpdateLead(e.Admin(), id, people.UpdateLeadInput{Status: &to}); err != nil {
		t.Fatalf("moving lead to %s: %v", to, err)
	}
}

// nextWeek is a clock one week after the fixture's own writes, so the week
// under review — the one that just CLOSED — is the week the seeding happened
// in. The audit log is append-only by trigger and by grant, so a test cannot
// move a transition into the window; it moves the window instead.
func nextWeek() time.Time { return time.Now().UTC().AddDate(0, 0, 7) }

// A REP WITH NO LEADS GETS NO LEAD BLOCK. The distinction the whole presence
// boolean exists for: absent means the funnel was not this rep's work, and a
// row of zeros would read as failure at something nobody asked of them.
func TestARepWithNoLeadsGetsNoLeadBlockRatherThanZeros(t *testing.T) {
	e := setupWeekly(t)
	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if review.Scorecard == nil {
		t.Fatal("a review carries a scorecard even when both blocks are absent")
	}
	if review.Scorecard.Lead != nil {
		t.Fatalf("a rep with no leads must have no lead block, got %+v", review.Scorecard.Lead)
	}
}

// A rep WITH leads gets a block, even when nothing in it moved. The other side
// of the case above: zeros are a finding, and only absence of leads is absence.
func TestARepWithUntouchedLeadsGetsABlockOfZerosNotAnAbsentOne(t *testing.T) {
	e := setupWeekly(t)
	seedLeadFor(t, e, "Untouched", e.Rep1)

	// Assembled from the FOLLOWING week, so the lead the fixture just wrote is
	// one the rep already carried when the week under review closed. Presence
	// asks "did they carry a funnel by then", and a lead minted after the week
	// must not conjure a block onto it.
	review, _, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	block := review.Scorecard.Lead
	if block == nil {
		t.Fatal("a rep who carries leads has a funnel to score, however still it stood")
	}
	if block.Advanced != 0 {
		t.Fatalf("nothing moved this week, so Advanced is 0, got %d", block.Advanced)
	}
}

// THE MOVEMENT COUNTS COME FROM THE AUDIT IMAGES. A lead that climbed the
// ladder inside the window is counted; the lead row itself cannot say this,
// because it carries only the status the lead ended on.
func TestALeadThatClimbedTheLadderThisWeekIsCountedAsAdvanced(t *testing.T) {
	e := setupWeekly(t)
	lead := seedLeadFor(t, e, "Climber", e.Rep1)
	moveLead(t, e, lead, "contacted")
	moveLead(t, e, lead, "engaged")

	review, _, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	// Two transitions: new→contacted and contacted→engaged. The lead's own row
	// now says `engaged` and could report at most one of them.
	if got := review.Scorecard.Lead.Advanced; got != 2 {
		t.Fatalf("two rungs climbed this week, want Advanced 2, got %d", got)
	}
}

// A lead that moved BEFORE the window is not this week's movement, even though
// the lead is still the rep's and still reads `engaged` today.
//
// Assembled two weeks on rather than by re-dating the audit row: the log is
// append-only, by trigger and by grant, so the window is what moves.
func TestALeadThatMovedBeforeTheWindowIsNotThisWeeksMovement(t *testing.T) {
	e := setupWeekly(t)
	lead := seedLeadFor(t, e, "Old mover", e.Rep1)
	moveLead(t, e, lead, "contacted")

	review, _, err := e.engine.AssembleFor(e.repCtx, nextWeek().AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	if review.Scorecard.Lead == nil {
		t.Fatal("the rep still carries the lead, so the block is present")
	}
	if got := review.Scorecard.Lead.Advanced; got != 0 {
		t.Fatalf("the move predates the week under review, want 0, got %d", got)
	}
}

// THE SCORECARD IS FROZEN. Re-reading a written review returns the figures as
// they were written, not a fresh count over today's rows.
func TestTheScorecardIsFrozenWithTheReview(t *testing.T) {
	e := setupWeekly(t)
	lead := seedLeadFor(t, e, "Frozen", e.Rep1)
	moveLead(t, e, lead, "contacted")

	written, created, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("the first assembly of a week writes it")
	}
	before := written.Scorecard.Lead.Advanced
	// The comparison below is only worth making against a real figure: 0 == 0
	// would hold whether or not anything is frozen.
	if before == 0 {
		t.Fatal("fixture must produce a non-zero count, or the freeze proves nothing")
	}

	// Move the same lead again AFTER the week was frozen. A scorecard that
	// recomputed on read would now report a different past.
	moveLead(t, e, lead, "engaged")

	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if got := read.Scorecard.Lead.Advanced; got != before {
		t.Fatalf("a frozen week must read back unchanged: wrote %d, read %d", before, got)
	}
}

// A REVIEW WRITTEN BEFORE THE SCORECARD EXISTED HAS NONE, and that is not an
// error. The read path answers nil so the panel draws nothing.
func TestAReviewWithNoScorecardRowReadsAsAbsentRatherThanFailing(t *testing.T) {
	e := setupWeekly(t)
	written, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `DELETE FROM weekly_review_scorecard WHERE weekly_review_id = $1`, written.ID)

	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatalf("a review predating the scorecard must still read: %v", err)
	}
	if read.Scorecard != nil {
		t.Fatalf("no row means no scorecard, got %+v", read.Scorecard)
	}
}

// Freezing twice leaves ONE scorecard. The review's own insert usually stops a
// second pass, so this calls the freeze directly — otherwise the test would
// pass on the upstream arbiter and prove nothing about this one.
func TestFreezingAScorecardTwiceLeavesTheFirstOne(t *testing.T) {
	e := setupWeekly(t)
	written, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	second := Scorecard{Lead: &LeadBlock{Advanced: 99}}
	if err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
		return insertScorecard(e.repCtx, tx, written.ID, second)
	}); err != nil {
		t.Fatalf("a second freeze must be refused quietly, not fail: %v", err)
	}

	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if read.Scorecard != nil && read.Scorecard.Lead != nil && read.Scorecard.Lead.Advanced == 99 {
		t.Fatal("the second freeze overwrote the first; the record of a week must not be rewritten")
	}
}

// THE SCORECARD REACHES THE WIRE. The Go value being right proves nothing about
// what a client receives: a block mapped to the wrong field, or not mapped at
// all, reads to the frontend as "this feature does nothing".
func TestTheScorecardTravelsOnTheWireWithItsBlocks(t *testing.T) {
	e := setupWeekly(t)
	lead := seedLeadFor(t, e, "Wire", e.Rep1)
	moveLead(t, e, lead, "contacted")

	review, _, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	wire := reviewToWire(review)
	if wire.Scorecard == nil {
		t.Fatal("a review with a scorecard must carry it on the wire")
	}
	if wire.Scorecard.Lead == nil {
		t.Fatal("the rep carries leads, so the lead block must travel")
	}
	if got := wire.Scorecard.Lead.Advanced; got != 1 {
		t.Fatalf("one rung was climbed, want Advanced 1 on the wire, got %d", got)
	}
	// The rep has no deals, so that block must be OMITTED rather than zeroed —
	// the distinction the whole presence design rests on.
	if wire.Scorecard.Deal != nil {
		t.Fatalf("a rep with no deals sends no deal block, got %+v", wire.Scorecard.Deal)
	}
}

// A DEMOTION IS NOT AN ADVANCE. The ladder has a direction, and a lead pushed
// back down it must not read as progress — the count is judged from BOTH ends
// of the transition, mirroring people.LeadStatus.Advances.
func TestALeadPushedBackDownTheLadderIsNotCountedAsAnAdvance(t *testing.T) {
	e := setupWeekly(t)
	lead := seedLeadFor(t, e, "Backwards", e.Rep1)
	moveLead(t, e, lead, "contacted")
	moveLead(t, e, lead, "engaged")
	// Back down a rung. Its to_status is `contacted`, which a filter reading
	// only the destination would score as a climb.
	moveLead(t, e, lead, "contacted")

	review, _, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	if got := review.Scorecard.Lead.Advanced; got != 2 {
		t.Fatalf("two climbs and one demotion is 2 advances, got %d", got)
	}
}

// A WON DEAL IS NOT AN OPEN ONE. `open` is the denominator every coverage count
// is read against, so counting closed deals in it understates every rate the
// panel draws.
func TestAWonDealLeavesTheOpenDenominator(t *testing.T) {
	e := setupWeekly(t)
	pipeline, openStage, wonStage := integration.DealFixture(t, e.Env)
	e.SeedDeal(t, "Still working", pipeline, openStage, &e.Rep1)
	closing := e.SeedDeal(t, "Closed this week", pipeline, openStage, &e.Rep1)
	// A win needs a contract or a stated reason for having none; the fixture
	// takes the reason, because this case is about the open DENOMINATOR and not
	// about how a deal is allowed to close.
	verbal := "verbal"
	if _, err := e.Deals.AdvanceDeal(e.Admin(),
		ids.From[ids.DealKind](closing),
		deals.AdvanceDealInput{
			ToStageID: wonStage, WonWithoutContractReason: &verbal,
		}); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}

	review, _, err := e.engine.AssembleFor(e.repCtx, nextWeek())
	if err != nil {
		t.Fatal(err)
	}
	block := review.Scorecard.Deal
	if block == nil {
		t.Fatal("the rep has deals, so the block is present")
	}
	if block.Open != 1 {
		t.Fatalf("one deal is still open and one was won, want Open 1, got %d", block.Open)
	}
}

// A LEAD MINTED AFTER THE WEEK DOES NOT CONJURE A BLOCK ONTO IT. Presence asks
// whether the rep carried a funnel BY the end of the week under review; a lead
// that did not exist then says nothing about that week.
func TestALeadCreatedAfterTheWeekLeavesItsFunnelAbsent(t *testing.T) {
	e := setupWeekly(t)
	seedLeadFor(t, e, "Arrived later", e.Rep1)

	// weekClock is June 2026, long before the fixture's own writes, so the week
	// it reviews closed before this lead existed.
	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if review.Scorecard.Lead != nil {
		t.Fatalf("the lead postdates the week, so it has no funnel, got %+v",
			review.Scorecard.Lead)
	}
}
