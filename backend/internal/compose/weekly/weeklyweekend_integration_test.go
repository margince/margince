// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// The scorecard's population counts, asked of a week that has already moved on.
//
// These cases are integration cases for the reason the rest of this suite is:
// the thing under test is the reverse-apply SQL, and it reads audit images that
// only exist because the real writers put them there. A test that inserted its
// own audit rows would prove the query can read rows the test knows how to
// write, which is not the claim.
//
// The fixture never back-dates a write. The writers stamp now(), so every audit
// row lands after the closing cutoff of the week under review — which is exactly
// the situation being tested: a review assembled late, describing a week whose
// deals have changed since.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/auditverb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// weekBirth is an instant inside the week weekClock reviews — the Monday-to-
// Sunday week before that Wednesday. A deal dated here existed when the week
// closed; every write this suite makes afterwards happens at wall-clock now,
// which is long after that Sunday, so each one is a post-cutoff change and the
// reconstruction has to undo it.
var weekBirth = weekClock.AddDate(0, 0, -9)

// openDealFor mints one deal owned by the rep, through the real writer, and
// hands back the won stage beside it so a caller can close it the way the
// product does.
//
// The deal's created_at is then moved into the week under review. That is the
// one thing this fixture back-dates, and it has to: audit_log is immutable by
// trigger (trg_audit_no_mutate, BEFORE DELETE OR UPDATE), so a test cannot move
// the EDIT it makes into the past, and the whole
// situation being tested is a deal that existed during the week and changed
// after it. Moving the birth backwards is the only end of that pair a test can
// hold. The audit rows stay where the writers put them, which is exactly the
// side the reconstruction reads.
func openDealFor(t *testing.T, e *weekEnv, name string) (ids.DealID, ids.StageID) {
	t.Helper()
	pipeline, open, won := integration.DealFixture(t, e.Env)
	repOwner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: open,
		Source: "manual", OwnerID: &repOwner,
	})
	if err != nil {
		t.Fatalf("seeding deal %s: %v", name, err)
	}
	id := ids.From[ids.DealKind](ids.UUID(created.Id))
	bornInTheWeek(t, id)
	return id, won
}

// bornInTheWeek dates a seeded deal into the week the review covers.
func bornInTheWeek(t *testing.T, id ids.DealID) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE deal SET created_at = $2 WHERE id = $1`, id, weekBirth); err != nil {
		t.Fatalf("dating the deal into the week under review: %v", err)
	}
}

// winDeal closes a deal the way the product does: an advance to the won stage,
// which is the writer that moves status and records the audit image the rewind
// reads. A direct status patch is not a door the product offers.
func winDeal(t *testing.T, e *weekEnv, id ids.DealID, won ids.StageID) {
	t.Helper()
	reason := "verbal"
	detail := "Rahmenvertrag liegt beim Kunden"
	if _, err := e.Deals.AdvanceDeal(e.Admin(), id, deals.AdvanceDealInput{
		ToStageID: won, WonWithoutContractReason: &reason,
		WonWithoutContractDetail: &detail,
	}); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}
}

// dealBlockAt assembles the rep's review and hands back its deal block.
func dealBlockAt(t *testing.T, e *weekEnv) *DealBlock {
	t.Helper()
	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	if review.Scorecard == nil || review.Scorecard.Deal == nil {
		t.Fatal("the rep has an open deal, so the week has a deal block to judge")
	}
	return review.Scorecard.Deal
}

// A DEAL CLOSED AFTER THE WEEK ENDED WAS STILL OPEN WHEN IT ENDED.
//
// The defect this holds: the population counts read the deal row, and the deal
// row is current. The dispatcher retries all week for a rep whose review landed
// un-narrated, so a review written on Thursday counted Thursday's pipeline as
// last Sunday's. Permanently: uq_weekly_review_user_week refuses a second review
// for the week, so the first figures stand as that week's record for good.
//
// Reading the count as 1 is the whole claim. With the reconstruction reverted
// this reads 0: the deal is won TODAY, and today is what the old query asked.
func TestADealClosedAfterTheWeekEndedStillCountsAsOpenThatWeek(t *testing.T) {
	e := setupWeekly(t)
	id, wonStage := openDealFor(t, e, "Weber Rahmenvertrag")

	// Won NOW — after the closing cutoff of the week under review. Through the
	// real writer, so the audit image the rewind reads is the one production
	// writes.
	winDeal(t, e, id, wonStage)

	block := dealBlockAt(t, e)
	if block.Open != 1 {
		t.Errorf("open deals at week end = %d, want 1 — the deal was won after the "+
			"week closed, so the week it belongs to had it open", block.Open)
	}
	// Present and zero, never absent: a week measured now always answers the
	// question, and absent is reserved for weeks scored before it was asked.
	if block.Unreconstructible == nil {
		t.Fatal("a week scored now must say what it could not rebuild, even when that is none")
	}
	if got := *block.Unreconstructible; got != 0 {
		t.Errorf("unreconstructible = %d, want 0 — nothing here is behind an erasure", got)
	}
}

// A DEAL ARCHIVED AFTER THE WEEK ENDED WAS STILL IN IT.
//
// Archival is the one field the rewind reads by VERB rather than by image: the
// archive writer audits the lifecycle and passes nil images either side, so
// there is no before-value to fold. Undoing an 'archive' means the deal was
// live at the cutoff, and this is what proves the verb path works — the image
// path cannot answer this case at all.
func TestADealArchivedAfterTheWeekEndedStillCountsInIt(t *testing.T) {
	e := setupWeekly(t)
	id, _ := openDealFor(t, e, "Kessler Wartung")

	if _, err := e.Deals.ArchiveDeal(e.Admin(), id, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	block := dealBlockAt(t, e)
	if block.Open != 1 {
		t.Errorf("open deals at week end = %d, want 1 — the deal was archived after "+
			"the week closed, and the archive verb is what says it was live before",
			block.Open)
	}
}

// A DEAL CREATED AFTER THE WEEK ENDED WAS NOT IN IT.
//
// The mirror of the two above, and the case a reverse-apply gets wrong by
// omission: rewinding tells you what a deal LOOKED like, never whether it
// existed. A deal minted this morning has no audit row before the cutoff and
// must not appear in a count of last week's pipeline.
//
// It is asserted through the block's absence rather than through Open == 0,
// because a rep whose only deal postdates the week has nothing to judge at all.
func TestADealCreatedAfterTheWeekEndedIsNotCountedInIt(t *testing.T) {
	e := setupWeekly(t)
	// Seeded WITHOUT the back-dating every other case here applies: this deal is
	// born at wall-clock now, which is after the reviewed week closed. That is
	// the whole premise, so it cannot share the helper.
	pipeline, open, _ := integration.DealFixture(t, e.Env)
	repOwner := ids.From[ids.UserKind](e.Rep1)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Neumann Erstauftrag", PipelineID: pipeline, StageID: open,
		Source: "manual", OwnerID: &repOwner,
	}); err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	if review.Scorecard == nil {
		t.Fatal("a review carries a scorecard even when its blocks are absent")
	}
	if d := review.Scorecard.Deal; d != nil && d.Open != 0 {
		t.Errorf("open deals at week end = %d, want 0 — the deal did not exist when "+
			"the week closed", d.Open)
	}
}

// AN ERASED DEAL IS COUNTED AS UNRECONSTRUCTIBLE, NEVER QUIETLY AT TODAY'S
// VALUES.
//
// The spine is append-only, so a scrub cannot rewrite the images captured
// before it — they are still sitting in audit_log. A rewind that read them
// would put back onto a rep's weekly review exactly what the scrub certified
// destroyed. So the boundary excludes the deal from every population count and
// tallies it separately, which is what tells a reader the counts are a floor.
func TestADealBehindAnErasureIsReportedRatherThanCountedAtTodaysValues(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	id, wonStage := openDealFor(t, e, "Vogel Ablösung")

	// An ordinary post-cutoff edit FIRST, so the deal has a rewind row at all;
	// then the scrub over the top of it. A deal with no post-cutoff change never
	// reaches the boundary, so seeding only the tombstone would pass whether or
	// not the boundary works.
	winDeal(t, e, id, wonStage)
	// The tombstone is written directly because NO WRITER IN THE TREE PRODUCES
	// ONE FOR A DEAL. privacy tombstones person, lead, activity, attachment,
	// deal_room_participant, scheduled_send and the AI records — never a deal —
	// so this row cannot be seeded through production, and the boundary excludes
	// nothing today.
	//
	// The boundary is still the right code to hold: it fails closed the day deal
	// erasure lands, and that day is what
	// TestEveryScrubbedRecordTypeIsOneAReaderOfTheBoundaryExpects exists to
	// announce. This case proves the query honours a tombstone when one exists;
	// the gate proves nobody adds one without deciding what a frozen week should
	// then report. Neither claims on its own that the path is live.
	if _, err := owner.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('system', 'system:retention', 'erase', 'deal', $1)`, id); err != nil {
		t.Fatalf("writing the erasure tombstone: %v", err)
	}

	block := dealBlockAt(t, e)
	if block.Unreconstructible == nil {
		t.Fatal("a week scored now must say what it could not rebuild")
	}
	if got := *block.Unreconstructible; got != 1 {
		t.Errorf("unreconstructible = %d, want 1 — the deal's week sits behind an "+
			"erasure and cannot be rebuilt from images the scrub certified gone", got)
	}
	if block.Open != 0 {
		t.Errorf("open deals at week end = %d, want 0 — an unreconstructible deal is "+
			"left out of the counts and reported, never folded in at today's values",
			block.Open)
	}
}

// TWO POST-CUTOFF EDITS TO DIFFERENT COLUMNS, AND BOTH MUST BE UNDONE.
//
// A patch carries only the columns it moved, so the oldest post-cutoff change
// is the right source for the columns IT touched and says nothing about the
// rest. A reconstruction that reads one row and falls back to the current value
// for everything else undoes the first edit and keeps the second — a row that
// existed at no instant, mixing last week's status with this week's close date.
func TestEachColumnIsRewoundByItsOwnOldestChange(t *testing.T) {
	e := setupWeekly(t)
	id, wonStage := openDealFor(t, e, "Hartmann Ausbau")

	// First edit: the close date moves. Second: the deal is won. Different
	// columns, in that order, both after the week closed.
	future := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	if _, err := e.Deals.UpdateDeal(e.Admin(), id, deals.UpdateDealInput{
		ExpectedClose: &future,
	}); err != nil {
		t.Fatalf("moving the close date: %v", err)
	}
	winDeal(t, e, id, wonStage)

	block := dealBlockAt(t, e)
	// Won AFTER the week, so it was open during it — this holds the status
	// column, whose own oldest change is the SECOND edit.
	if block.Open != 1 {
		t.Errorf("open deals at week end = %d, want 1 — the win came after the week "+
			"closed, and the edit before it must not hide that", block.Open)
	}
	// And the close date, whose own oldest change is the FIRST edit. Asserted
	// beside the status because one column proves only its own path: a rewind
	// that read one shared row could get either right while mixing the other in.
	//
	// The deal was seeded with no close date, so it was NOT sound at week end.
	// The post-cutoff edit gave it a firm future one; reading today's row would
	// count it as sound and report a deal in better shape than the week left it.
	if block.CloseDateSound != 0 {
		t.Errorf("deals with a sound close date at week end = %d, want 0 — the date "+
			"was set after the week closed, so that week had none", block.CloseDateSound)
	}
}

// UNDOING AN EDIT IS NOT UNARCHIVING A DEAL.
//
// The archival rewind reads the VERB rather than an image, and 'restore' looks
// like the opposite of 'archive' until you read what writes it: auditverb calls
// a restore "an ordinary update in every respect except what the trail calls
// it", written by the reversal path over any field at all. There is no
// un-archive writer for a deal — dealarchive.go has ArchiveDeal and no
// counterpart — so reading 'restore' as one would drop a never-archived deal
// out of the week because somebody undid a name correction on it.
func TestUndoingAnEditDoesNotReadAsUnarchivingTheDeal(t *testing.T) {
	e := setupWeekly(t)
	id, _ := openDealFor(t, e, "Brandt Servicevertrag")

	// A post-cutoff edit recorded the way the reversal path records one: the
	// same update door, carrying the restore verb. The deal is never archived.
	renamed := "Brandt Servicevertrag (korrigiert)"
	if _, err := e.Deals.UpdateDeal(e.Admin(), id, deals.UpdateDealInput{
		Name:  &renamed,
		Trail: auditverb.Trail{Verb: auditverb.Restore},
	}); err != nil {
		t.Fatalf("undoing the edit: %v", err)
	}

	block := dealBlockAt(t, e)
	if block.Open != 1 {
		t.Errorf("open deals at week end = %d, want 1 — the deal was never archived, "+
			"and a restore-verb row is an ordinary edit rather than an un-archive",
			block.Open)
	}
}

// A DEAL ARCHIVED BEFORE THE WEEK AND AGAIN AFTER IT WAS ARCHIVED ALL WEEK.
//
// The archival rewind reads the post-cutoff 'archive' verb as "it was live
// before". That holds only while a deal cannot be archived twice — and it can:
// retention's archiveDeal writes archived_at unconditionally, so a deal a human
// already archived takes a second row when a sweep reaches it. Reading the later
// verb alone would report the deal open through a week it spent archived, and
// the current archived_at cannot correct that because it holds the LATER stamp.
func TestADealArchivedBeforeAndAgainAfterTheWeekIsNotReportedOpen(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	id, _ := openDealFor(t, e, "Seidel Altvertrag")

	// Archived BEFORE the cutoff: the row is dated into the reviewed week, the
	// way the deal's own birth is, because a test cannot back-date an audit row.
	if _, err := e.Deals.ArchiveDeal(e.Admin(), id, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		UPDATE audit_log SET occurred_at = $2
		 WHERE entity_type = 'deal' AND entity_id = $1 AND action = 'archive'`,
		id, weekBirth.AddDate(0, 0, 1)); err != nil {
		// audit_log refuses UPDATE by trigger, so the pre-cutoff row is written
		// directly instead — the same shape ArchiveDeal writes, at an instant the
		// writer cannot be asked for.
		if _, insErr := owner.Exec(ctx, `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type,
			                       entity_id, occurred_at)
			VALUES ('human', 'human:fixture', 'archive', 'deal', $1, $2)`,
			id, weekBirth.AddDate(0, 0, 1)); insErr != nil {
			t.Fatalf("seeding the pre-cutoff archive: %v", insErr)
		}
	}
	// And archived again after the week, which is the row the rewind sees first.
	if _, err := owner.Exec(ctx,
		`INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		 VALUES ('system', 'system:retention', 'archive', 'deal', $1)`, id); err != nil {
		t.Fatalf("seeding the retention archive: %v", err)
	}

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	if review.Scorecard == nil {
		t.Fatal("a review carries a scorecard even when its blocks are absent")
	}
	if d := review.Scorecard.Deal; d != nil && d.Open != 0 {
		t.Errorf("open deals at week end = %d, want 0 — the deal was already archived "+
			"when the week closed, and a later archive does not make it live", d.Open)
	}
}
