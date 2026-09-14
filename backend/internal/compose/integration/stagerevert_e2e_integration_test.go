// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Taking back a stage move the product made by itself.
//
// Split from the apply tests beside it because the two answer different
// questions: those ask whether a transition may move a deal at all, these ask
// what happens when somebody disagrees with a move it made. The fixtures they
// share — a governed transition and a real automatic apply — live there,
// because that is where they are built.
//
// Two doors reach one fact. A rep can press Undo or simply drag the deal back,
// and both must reach the ledger: counted on only one, the safety rate would
// be dodgeable by the route nobody instrumented.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// autoAppliedMove drives a real automatic apply and answers the card that made
// it, so an undo test starts from a move the product actually made.
func autoAppliedMove(t *testing.T, e *Env) (ids.DealID, ids.ApprovalID, deals.TransitionRef) {
	t.Helper()
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	ref := governedTransition(t, e, deal)
	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if applied == 0 {
		t.Fatal("nothing applied, so there is no automatic move to take back")
	}
	card := appliedCardFor(t, e, deal)
	deliverApprovalDecided(t, e, card)
	return deal, card, ref
}

// ledgerOutcomeFor answers how the ledger records one card's move.
func ledgerOutcomeFor(t *testing.T, e *Env, card ids.ApprovalID) (string, bool) {
	t.Helper()
	var outcome string
	var reversed bool
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT outcome, reversed_at IS NOT NULL
		  FROM stage_progression_outcome WHERE approval_id = $1`,
		card).Scan(&outcome, &reversed); err != nil {
		t.Fatalf("reading the ledger row: %v", err)
	}
	return outcome, reversed
}

// Undo inside the window moves the deal back and counts a reversal.
func TestUndoInsideTheWindowMovesTheDealBackAndCountsAReversal(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)
	if got := stageOf(t, e, deal); got != ref.ToStageID {
		t.Fatalf("the fixture's deal is at %s, not the stage the sweep moved it "+
			"to (%s) — the undo below would prove nothing", got, ref.ToStageID)
	}
	if outcome, _ := ledgerOutcomeFor(t, e, card); outcome != deals.ProgressionAutoApplied {
		t.Fatalf("the fixture's move reads as %q, not an automatic one", outcome)
	}

	if _, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID); err != nil {
		t.Fatalf("taking the move back: %v", err)
	}

	if got := stageOf(t, e, deal); got != ref.FromStageID {
		t.Fatalf("the deal is at %s after the undo, want the stage it came from (%s)",
			got, ref.FromStageID)
	}
	outcome, reversed := ledgerOutcomeFor(t, e, card)
	if outcome != deals.ProgressionReversed {
		t.Errorf("the ledger reads %q after an undo, want %q — the safety number "+
			"is taken over this column, so a move nobody counts is one that never "+
			"stops the transition", outcome, deals.ProgressionReversed)
	}
	if !reversed {
		t.Error("reversed_at is unset: the outcome and the timestamp are two " +
			"spellings of one fact and the report reads either")
	}

	// History is APPEND-ONLY. The move back is its own row; erasing the
	// original would leave the trail claiming the deal was never moved.
	var rows int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_history WHERE deal_id = $1`, deal).Scan(&rows); err != nil {
		t.Fatalf("counting history: %v", err)
	}
	if rows < 2 {
		t.Errorf("the deal has %d history rows after a move and an undo, want at "+
			"least 2 — the undo rewrote history instead of recording itself", rows)
	}
}

// An undo does not refute the evidence the move rested on.
//
// Taking a move back says the deal should not have moved. It does not say the
// criteria were misread — a rep may agree with every observation and still want
// the deal where it was, and a reversal that quietly withdrew the claims would
// retract evidence nobody disputed.
func TestAnUndoDoesNotRefuteTheEvidenceTheMoveRestedOn(t *testing.T) {
	e := Setup(t)
	deal, card, _ := autoAppliedMove(t, e)

	var before int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_evidence WHERE deal_id = $1 AND refuted_at IS NULL`,
		deal).Scan(&before); err != nil {
		t.Fatalf("counting standing evidence: %v", err)
	}
	if before == 0 {
		t.Fatal("the fixture cites no standing evidence, so nothing could be " +
			"refuted and this test would pass however the undo behaved")
	}

	if _, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID); err != nil {
		t.Fatalf("taking the move back: %v", err)
	}

	var after int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_evidence WHERE deal_id = $1 AND refuted_at IS NULL`,
		deal).Scan(&after); err != nil {
		t.Fatalf("counting standing evidence: %v", err)
	}
	if after != before {
		t.Errorf("standing evidence went from %d to %d across an undo: taking a "+
			"move back withdrew claims nobody disputed", before, after)
	}
}

// After the window, the undo is refused and the move stands.
func TestUndoAfterTheWindowIsRefusedAndTheAuditStays(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)

	// Push the decision past the transition's own undo window.
	if _, err := e.Pool.Exec(t.Context(), `
		UPDATE stage_progression_outcome
		   SET decided_at = now() - interval '73 hours'
		 WHERE approval_id = $1`, card); err != nil {
		t.Fatalf("ageing the move: %v", err)
	}

	_, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID)
	if err == nil {
		t.Fatal("a move older than its undo window was taken back anyway")
	}
	var closed *deals.UndoWindowClosedError
	if !errors.As(err, &closed) {
		t.Fatalf("the refusal is %v, want an UndoWindowClosedError so the caller "+
			"gets a 409 naming what to do instead", err)
	}

	if got := stageOf(t, e, deal); got != ref.ToStageID {
		t.Errorf("the deal moved to %s on a refused undo", got)
	}
	if outcome, reversed := ledgerOutcomeFor(t, e, card); reversed ||
		outcome != deals.ProgressionAutoApplied {
		t.Errorf("the refused undo still marked the ledger (%q, reversed=%v)",
			outcome, reversed)
	}
}

// A move a CONTACT approved is not undone by this verb.
//
// There was never anything automatic to take back. Moving the deal back is an
// ordinary stage move the rep can make, and that route counts as a reversal on
// the same ledger.
func TestAHumanApprovedMoveIsNotTakenBackByTheUndoVerb(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the proposal did not stage")
	}
	giveTheOwnerARealRole(t, e)
	card, open := pendingCardFor(t, e, deal)
	if !open {
		t.Fatal("no card to approve")
	}
	if _, err := compose.StageProgressionDecisions(e.Pool).Decide(
		e.Admin(), card, true, nil); err != nil {
		t.Fatalf("approving the card as a contact: %v", err)
	}
	deliverApprovalDecided(t, e, card)

	_, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID)
	if err == nil {
		t.Fatal("the undo verb took back a move a contact had approved")
	}
	var closed *deals.UndoWindowClosedError
	if !errors.As(err, &closed) {
		t.Fatalf("the refusal is %v, want an UndoWindowClosedError", err)
	}
}

// A manual move back over a recent automatic one counts as a reversal.
//
// The route a rep who disagrees actually takes. Counted only on the undo
// button, the safety number would be dodgeable by dragging the deal back —
// the transition would keep applying while contacts quietly corrected it.
func TestAManualMoveBackInsideTheWindowAlsoCountsAsReversalWithoutRefutingEvidence(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)

	var evidenceBefore int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_evidence WHERE deal_id = $1 AND refuted_at IS NULL`,
		deal).Scan(&evidenceBefore); err != nil {
		t.Fatalf("counting standing evidence: %v", err)
	}

	// No undo button — an ordinary stage move, back the way it came.
	if _, err := e.Deals.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{
		ToStageID: ref.FromStageID,
	}); err != nil {
		t.Fatalf("moving the deal back by hand: %v", err)
	}

	outcome, reversed := ledgerOutcomeFor(t, e, card)
	if outcome != deals.ProgressionReversed || !reversed {
		t.Errorf("a manual move back left the ledger at (%q, reversed=%v): the "+
			"safety number can be dodged by dragging the deal instead of pressing "+
			"undo, and the transition keeps applying", outcome, reversed)
	}

	var evidenceAfter int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_evidence WHERE deal_id = $1 AND refuted_at IS NULL`,
		deal).Scan(&evidenceAfter); err != nil {
		t.Fatalf("counting standing evidence: %v", err)
	}
	if evidenceAfter != evidenceBefore {
		t.Errorf("standing evidence went from %d to %d: moving a deal back "+
			"withdrew claims nobody disputed", evidenceBefore, evidenceAfter)
	}
}

// An ordinary forward move undoes nothing.
//
// The guard on the guard: reversal detection that matched any stage move would
// mark unrelated moves as reversals and report a transition as unsafe because
// its deals kept progressing.
func TestAnOrdinaryStageMoveIsNotCountedAsAReversal(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)

	// Forward again, to a third OPEN stage — the opposite direction from an
	// undo. Open, because a terminal stage needs a lost reason or a contract
	// and this test is about direction, not about closing a deal.
	var onward ids.StageID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM stage
		 WHERE pipeline_id = $1 AND id <> $2 AND id <> $3
		   AND archived_at IS NULL AND semantic = 'open'
		 ORDER BY "position" DESC LIMIT 1`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID).Scan(&onward); err != nil {
		t.Skipf("the fixture pipeline has no third open stage to move on to: %v", err)
	}
	if _, err := e.Deals.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{
		ToStageID: onward,
	}); err != nil {
		t.Fatalf("moving the deal onward: %v", err)
	}

	if outcome, reversed := ledgerOutcomeFor(t, e, card); reversed ||
		outcome != deals.ProgressionAutoApplied {
		t.Errorf("moving a deal ONWARD marked the earlier move reversed (%q, %v): "+
			"every progressing deal would report its transition as unsafe",
			outcome, reversed)
	}
}

// A deal that has moved on cannot be sent backwards by an old card.
//
// Undo means "put it back", which is only meaningful while the deal is still
// where the move put it. Without this check, choosing which approval to revert
// chooses where the deal lands: automatic A→B, a rep moves it on to C, and
// reverting the A→B card takes it C→A — a move nobody made and an undo of
// nothing.
func TestAnUndoIsRefusedOnceTheDealHasMovedOn(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)

	var onward ids.StageID
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT id FROM stage
		 WHERE pipeline_id = $1 AND id <> $2 AND id <> $3
		   AND archived_at IS NULL AND semantic = 'open'
		 ORDER BY "position" DESC LIMIT 1`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID).Scan(&onward); err != nil {
		t.Skipf("the fixture pipeline has no third open stage: %v", err)
	}
	if _, err := e.Deals.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{
		ToStageID: onward,
	}); err != nil {
		t.Fatalf("moving the deal onward: %v", err)
	}

	_, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID)
	if err == nil {
		t.Fatal("an old card sent a deal backwards from a stage the move never " +
			"touched: choosing which approval to revert chooses where the deal lands")
	}
	var closed *deals.UndoWindowClosedError
	if !errors.As(err, &closed) {
		t.Fatalf("the refusal is %v, want an UndoWindowClosedError", err)
	}
	if got := stageOf(t, e, deal); got != onward {
		t.Errorf("the deal is at %s after a refused undo, want %s", got, onward)
	}
}

// An undone move is not proposed again.
//
// readProtection asks whether any history row on this deal names a move it
// undid. The reversal has to write that link or the product re-proposes the
// exact move a contact just took back — the definition of not listening.
func TestAnUndoneMoveIsNotProposedAgain(t *testing.T) {
	e := Setup(t)
	deal, card, _ := autoAppliedMove(t, e)
	if _, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID); err != nil {
		t.Fatalf("taking the move back: %v", err)
	}

	var linked int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_history WHERE deal_id = $1 AND reversal_of IS NOT NULL`,
		deal).Scan(&linked); err != nil {
		t.Fatalf("reading the history link: %v", err)
	}
	if linked == 0 {
		t.Fatal("the undo wrote no reversal_of link, so readProtection cannot see " +
			"that this deal has already had the argument and the product will " +
			"propose the same move again")
	}
}

// A manual move back also records what it undid.
func TestAManualMoveBackRecordsWhatItUndid(t *testing.T) {
	e := Setup(t)
	deal, _, ref := autoAppliedMove(t, e)
	if _, err := e.Deals.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{
		ToStageID: ref.FromStageID,
	}); err != nil {
		t.Fatalf("moving the deal back by hand: %v", err)
	}

	var linked int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT count(*) FROM deal_stage_history WHERE deal_id = $1 AND reversal_of IS NOT NULL`,
		deal).Scan(&linked); err != nil {
		t.Fatalf("reading the history link: %v", err)
	}
	if linked == 0 {
		t.Error("dragging a deal back left no reversal_of link, so the product " +
			"will propose the move again — the undo BUTTON records it and this " +
			"route does not, which is the half nobody presses")
	}
}

// The undo window is the one the move was made under.
//
// An admin shortening the window must not retroactively close it on moves
// already made, and lengthening it must not reopen ones contacts were told had
// closed. The promise is made when the move is applied.
func TestTheUndoWindowIsTheOneTheMoveWasMadeUnder(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)

	var frozen *int
	if err := e.Pool.QueryRow(t.Context(),
		`SELECT undo_window_hours FROM stage_progression_outcome WHERE approval_id = $1`,
		card).Scan(&frozen); err != nil {
		t.Fatalf("reading the frozen window: %v", err)
	}
	if frozen == nil {
		t.Fatal("the automatic move froze no undo window, so a later policy edit " +
			"decides whether it can still be taken back")
	}

	// The admin shortens the window to an hour, and ages the move past it.
	if _, err := e.Deals.SetTransitionPolicy(e.Admin(), deals.SetTransitionPolicyInput{
		TransitionRef: ref, Mode: deals.ModeAuto, UndoWindowHours: ptrTo(1),
	}); err != nil {
		t.Fatalf("shortening the window: %v", err)
	}
	if _, err := e.Pool.Exec(t.Context(),
		`UPDATE stage_progression_outcome SET decided_at = now() - interval '2 hours'
		  WHERE approval_id = $1`, card); err != nil {
		t.Fatalf("ageing the move: %v", err)
	}

	// Two hours old, under the ONE hour the admin just set but inside the
	// seventy-two the move was made under. It must still be undoable.
	if _, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID); err != nil {
		t.Fatalf("the undo was refused on a window the admin shortened AFTER the "+
			"move was made: %v — a contact told they had three days to take this "+
			"back finds it closed because somebody edited a setting", err)
	}
}

func ptrTo[T any](v T) *T { return &v }

// An AGENT moving a deal back is not a contact taking a move back.
//
// The undo verb is human-only, and this door must not be the way around it.
// Read as manual, an agent's ordinary advance would record a reversal nobody
// made, attribute it to the agent's user, and let one machine undo what
// another machine did with no contact in the loop.
//
// readProtection names its human positively for the same reason, and says so:
// an agent moving a deal is precisely what a human has not done.
func TestAnAgentMovingADealBackIsNotCountedAsAHumanReversal(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)

	agentCtx := principal.WithActor(e.Admin(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(),
		UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Scopes: principal.ScopeSet{principal.ScopeWrite: struct{}{}},
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
		},
	})
	if _, err := e.Deals.AdvanceDeal(agentCtx, deal, deals.AdvanceDealInput{
		ToStageID: ref.FromStageID,
	}); err != nil {
		t.Fatalf("the agent could not move the deal back, so this test never "+
			"reaches the question it asks: %v", err)
	}

	outcome, reversed := ledgerOutcomeFor(t, e, card)
	if reversed || outcome != deals.ProgressionAutoApplied {
		t.Errorf("an AGENT's move back was counted as a human reversal (%q, %v): "+
			"the undo verb is human-only and this is the way around it, with the "+
			"reversal recorded against a contact who did nothing", outcome, reversed)
	}
}

// A reversal is measured against the rule that authorized the move.
//
// The safety ceiling exists to count exactly this outcome. Left to the
// decision path alone an undo would never trip it — nothing decides a card
// here, so the sweep that runs on decisions never sees it, and a run of undos
// would age out of the window with the transition still applying.
func TestAnUndoIsCountedAgainstTheRuleThatAuthorizedTheMove(t *testing.T) {
	e := Setup(t)
	deal, card, ref := autoAppliedMove(t, e)

	// A record sitting JUST under its ceiling, so this one undo is what
	// crosses it. The fixture already seeded 240 clean rows, so the unsafe
	// count is chosen against the real denominator rather than a guess: it is
	// marked on rows that already exist.
	var reviewed int
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT count(*) FROM stage_progression_outcome
		 WHERE pipeline_id = $1 AND from_stage_id = $2 AND to_stage_id = $3
		   AND outcome IN ('approved_clean', 'approved_edited', 'rejected', 'reversed')`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID).Scan(&reviewed); err != nil {
		t.Fatalf("counting the reviewed record: %v", err)
	}
	// The undo adds ONE unsafe row and moves the auto-applied row into the
	// reviewed denominator, so after it the rate is (marked+1)/(reviewed+1).
	// Mark the smallest number that leaves this just UNDER the 1% ceiling
	// before the undo and just over it after — computed against the real
	// denominator rather than assumed, because the fixture's size is the
	// governedTransition helper's to choose and it has changed once already.
	after := reviewed + 1
	// The smallest count whose (marked+1)/after strictly exceeds 1%. Solved
	// rather than derived by division, because integer truncation is what got
	// this wrong twice: after/100 is the floor, and the guards below caught it
	// sitting a row short of the ceiling on both attempts.
	marked := 1
	for float64(marked+1)/float64(after) <= 0.01 {
		marked++
	}
	if marked < 1 {
		t.Fatalf("the fixture's record (%d reviewed) is too small to sit just "+
			"under a 1%% ceiling", reviewed)
	}
	// The guard on the guard: the arithmetic must actually straddle the
	// ceiling, or this test passes for a reason that has nothing to do with
	// the undo being counted.
	if float64(marked)/float64(reviewed) > 0.01 {
		t.Fatalf("the seeded record is ALREADY over its ceiling (%d/%d), so the "+
			"undo is not what crosses it", marked, reviewed)
	}
	if float64(marked+1)/float64(after) <= 0.01 {
		t.Fatalf("the record is still under its ceiling after the undo "+
			"(%d/%d), so nothing would suspend however the undo behaved",
			marked+1, after)
	}
	justUnder := marked
	if _, err := e.Pool.Exec(t.Context(), `
		UPDATE stage_progression_outcome SET reversed_at = now()
		 WHERE id IN (
		     SELECT id FROM stage_progression_outcome
		      WHERE pipeline_id = $1 AND from_stage_id = $2 AND to_stage_id = $3
		        AND outcome = 'approved_clean'
		      LIMIT $4)`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID, justUnder); err != nil {
		t.Fatalf("spoiling the record: %v", err)
	}
	if off, why := suspensionStateOf(t, e, ref); off {
		t.Fatalf("the rule is already suspended (%s), so the undo below could "+
			"not be what suspends it", why)
	}

	if _, err := e.Deals.RevertStageProgression(e.Admin(), deal, card.UUID); err != nil {
		t.Fatalf("taking the move back: %v", err)
	}

	if off, _ := suspensionStateOf(t, e, ref); !off {
		t.Error("the undo did not count against the rule: a run of undos ages " +
			"out of the window with the transition still applying, and the " +
			"ceiling that exists to catch exactly this never fires")
	}
}

// suspensionStateOf answers whether the product has turned this rule off.
func suspensionStateOf(t *testing.T, e *Env, ref deals.TransitionRef) (bool, string) {
	t.Helper()
	var at *time.Time
	var reason *string
	if err := e.Pool.QueryRow(t.Context(), `
		SELECT suspended_at, suspended_reason FROM stage_progression_policy
		 WHERE pipeline_id = $1 AND from_stage_id = $2 AND to_stage_id = $3`,
		ref.PipelineID, ref.FromStageID, ref.ToStageID).Scan(&at, &reason); err != nil {
		t.Fatalf("reading the rule's suspension: %v", err)
	}
	if at == nil {
		return false, ""
	}
	return true, *reason
}
