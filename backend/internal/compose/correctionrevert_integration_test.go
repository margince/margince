// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Taking back what the nightly close-date pass did.
//
// Kept apart from the sweep's own suite because it tests the opposite
// direction: that one proves the machine corrects the right deals, this one
// proves a person can undo one of those corrections, that the undo restores
// every field the correction moved, and that neither the next pass nor a staged
// card puts it back afterwards.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// correctionFor reads the lifecycle row the sweep wrote for one deal.
func (e *closeDateEnv) correctionFor(t *testing.T, dealID ids.UUID) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM deal_correction WHERE deal_id = $1 AND reversed_at IS NULL
		  ORDER BY applied_at DESC LIMIT 1`, dealID).Scan(&id); err != nil {
		t.Fatalf("no live correction recorded for the deal: %v", err)
	}
	return id
}

// The undo the generic restore path could not perform.
//
// Two walls stop it there: rejectPastCloseDate refuses any past close date on an
// open deal — which is exactly what undoing an overdue correction restores — and
// close_date_provisional is audited but is not a writable deal field, so the
// restore image filter refuses the whole entry rather than putting back the part
// it understands. Every one of the sweep's tiers touches both.
func TestACorrectionIsTakenBackWithEveryFieldItMoved(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Rolled forward overnight", e.early, nil, intp(-12), 3)
	wasClosing := today().AddDate(0, 0, -12)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	rolled := e.readSwept(t, deal)
	if rolled.expectedClose == nil || !rolled.expectedClose.After(today()) {
		t.Fatalf("the sweep did not roll the date: %v", rolled.expectedClose)
	}
	if !rolled.provisional {
		t.Fatal("the rolled date should be provisional")
	}

	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, deal)); err != nil {
		t.Fatalf("taking the correction back: %v", err)
	}

	// The PAST date is back — the value the deal actually held, restored from
	// the correction's own audit row rather than chosen by a caller.
	restored := e.readSwept(t, deal)
	if restored.expectedClose == nil || !restored.expectedClose.Equal(wasClosing) {
		t.Errorf("restored close date = %v, want the original %s",
			restored.expectedClose, wasClosing.Format(time.DateOnly))
	}
	// And it is marked unresolved rather than presented as a plan: a date in
	// the past that nobody stands behind must not read as a confirmed one.
	if !restored.provisional {
		t.Error("a restored past date must stay provisional — nobody has confirmed it")
	}

	// The ordinary editor still refuses what this path was allowed to do.
	yesterday := today().AddDate(0, 0, -1)
	_, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		ExpectedClose: &yesterday,
	})
	var pastClose *deals.PastCloseDateError
	if !errors.As(err, &pastClose) {
		t.Errorf("the normal editor accepted a past close date (%v) — the exception leaked", err)
	}
}

// A correction taken back does not come back tomorrow.
//
// This is the gap the lifecycle row exists to close. The two memories that
// already existed both read APPROVALS — one stops a live card multiplying, the
// other stops a declined one returning — and a reversal writes no approval at
// all. So a rep could undo a correction and watch the identical one reappear on
// the next pass, their answer lasting exactly until the sweep ran again.
func TestAReversedCorrectionIsNotReappliedTomorrow(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Undone and left alone", e.early, nil, intp(-12), 3)
	wasClosing := today().AddDate(0, 0, -12)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, deal)); err != nil {
		t.Fatal(err)
	}

	// A new day, a fresh pass, unchanged evidence. The proposed date has moved
	// with the calendar and so has the standing date, which is exactly why the
	// memory keys on the QUESTION rather than on either of them.
	e.nextDay()
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	after := e.readSwept(t, deal)
	if after.expectedClose == nil || !after.expectedClose.Equal(wasClosing) {
		t.Errorf("the next pass re-dated a correction somebody took back: %v", after.expectedClose)
	}
}

// A person's later edit is not overwritten by an undo, and the check is per
// FIELD: renaming the deal leaves the undo available, re-dating it does not.
func TestATakenBackCorrectionRefusesToOverwriteALaterEdit(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Corrected then re-dated", e.early, nil, intp(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	correction := e.correctionFor(t, deal)

	// The rep puts their own date on it — a real answer, not the machine's.
	chosen := today().AddDate(0, 0, 45)
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		ExpectedClose: &chosen,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := e.Deals.RevertCorrection(e.Admin(), correction)
	var conflict *deals.CorrectionReversalError
	if !errors.As(err, &conflict) {
		t.Fatalf("undo after a human edit → %v, want a conflict that names the field", err)
	}
	kept := e.readSwept(t, deal)
	if kept.expectedClose == nil || !kept.expectedClose.Equal(chosen) {
		t.Errorf("the rep's own date was overwritten: %v", kept.expectedClose)
	}
}

// Undo twice is one reversal, not a toggle.
func TestTakingACorrectionBackTwiceIsOneReversal(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Undone twice", e.early, nil, intp(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	correction := e.correctionFor(t, deal)
	if _, err := e.Deals.RevertCorrection(e.Admin(), correction); err != nil {
		t.Fatal(err)
	}
	_, err := e.Deals.RevertCorrection(e.Admin(), correction)
	var conflict *deals.CorrectionReversalError
	if !errors.As(err, &conflict) {
		t.Errorf("second undo → %v, want a refusal saying it is already taken back", err)
	}
	var reversals int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM deal_correction WHERE id = $1 AND reversed_at IS NOT NULL`,
		correction).Scan(&reversals); err != nil {
		t.Fatal(err)
	}
	if reversals != 1 {
		t.Errorf("reversed corrections = %d, want exactly 1", reversals)
	}
}

// A confirm staged before an Undo does not put the change back after it.
//
// The approval path is a second door onto the same write, and an unattended one:
// close_date_correction is in AutoApplyKinds, so a rep with autonomy on has
// these redeemed without looking. An Undo followed by a redemption must not
// restore what the Undo removed.
//
// TWO GUARDS STAND HERE, and this proves the outcome rather than either one.
// The version pin refuses first — a reversal writes the deal, so the staged
// card's pinned version no longer matches — and the reversal-memory check in
// closeDateConfirmEffect stands behind it. Disabling the memory check alone
// leaves this test green, because the pin catches the case; the memory check
// exists because the pin is a property of how this KIND is staged, and a
// memory that holds only while a configuration elsewhere stays put is not one
// to rely on. TestAReversedCorrectionIsNotReappliedTomorrow is what fails when
// the memory itself is broken.
func TestConfirmingACardAfterAnUndoDoesNotReapplyIt(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Confirmed after being undone", e.early, nil, intp(-12), 3)
	wasClosing := today().AddDate(0, 0, -12)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	// The card the sweep staged, and the correction it wrote.
	var approvalID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM approval WHERE kind = 'close_date_correction'
		   AND target_entity_id = $1 AND status = 'pending'`, deal).Scan(&approvalID); err != nil {
		t.Fatalf("the sweep staged no confirm: %v", err)
	}
	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, deal)); err != nil {
		t.Fatal(err)
	}

	// Now the card is decided — the state an auto-apply worker reaches.
	_, err := e.svc.Decide(e.Admin(), ids.From[ids.ApprovalKind](approvalID), true, nil)
	if err == nil {
		t.Error("confirming a taken-back correction succeeded — the undo was silently reversed")
	}

	after := e.readSwept(t, deal)
	if after.expectedClose == nil || !after.expectedClose.Equal(wasClosing) {
		t.Errorf("the deal was re-dated to %v by a card decided after the undo", after.expectedClose)
	}
}
