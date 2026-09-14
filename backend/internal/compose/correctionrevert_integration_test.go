// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Taking back what the nightly close-date pass did.
//
// Kept apart from the sweep's own suite because it tests the opposite
// direction: that one proves the machine corrects the right deals, this one
// proves a contact can undo one of those corrections, that the undo restores
// every field the correction moved, and that neither the next pass nor a staged
// card puts it back afterwards.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// correctionAuditIDFor reads the AUDIT ROW a correction records — the id the
// receipt names on its Undo control and the id a real client's restore
// request carries, as opposed to correctionFor's deal_correction.id, which
// never reaches the wire.
func (e *closeDateEnv) correctionAuditIDFor(t *testing.T, dealID ids.UUID) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT audit_log_id FROM deal_correction WHERE deal_id = $1 AND reversed_at IS NULL
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

	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, deal), nil, nil); err != nil {
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
	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, deal), nil, nil); err != nil {
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

// A contact's later edit is not overwritten by an undo, and the check is per
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

	_, err := e.Deals.RevertCorrection(e.Admin(), correction, nil, nil)
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
	if _, err := e.Deals.RevertCorrection(e.Admin(), correction, nil, nil); err != nil {
		t.Fatal(err)
	}
	_, err := e.Deals.RevertCorrection(e.Admin(), correction, nil, nil)
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
	// A card left over from before corrections were applied unasked. The sweep
	// raises none now, but an installation upgrading mid-week has them pending
	// against deals it has since corrected — which is exactly when this hazard
	// bites, because the card and the undo disagree about the same deal.
	swept := e.readSwept(t, deal)
	if swept.expectedClose == nil {
		t.Fatalf("the sweep left no corrected date to confirm: %+v", swept)
	}
	approvalID := e.stageLegacyCard(t, deal, *swept.expectedClose)
	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, deal), nil, nil); err != nil {
		t.Fatal(err)
	}

	// Now the card is decided — the state an auto-apply worker reaches.
	_, err := e.svc.Decide(e.Admin(), approvalID, true, nil)
	if err == nil {
		t.Error("confirming a taken-back correction succeeded — the undo was silently reversed")
	}

	after := e.readSwept(t, deal)
	if after.expectedClose == nil || !after.expectedClose.Equal(wasClosing) {
		t.Errorf("the deal was re-dated to %v by a card decided after the undo", after.expectedClose)
	}
}

// magicReceipt reads the receipt through the REAL judge — the same adapter the
// server binds — rather than a stub. A stub would prove the seam carries an
// answer; only this proves the answer is the right one for a correction.
func (e *closeDateEnv) magicReceipt(t *testing.T, since time.Time) (crmcontracts.MagicReceipt, error) {
	t.Helper()
	seam := restoreSeamFor(e.Env)
	svc := magic.NewService(e.Pool, nil, time.Now).
		WithUndoJudge(magicUndoJudge{
			seam:        seam,
			corrections: deals.NewStore(e.DB(), DealsInstallation()),
		})
	return svc.Read(e.Admin(), &since, 50)
}

// The receipt offers an Undo on a correction the sweep actually made.
//
// THIS IS THE JOIN the whole engine exists for, and the one the generic
// evaluator cannot make on its own: it refuses exactly these rows, because a
// correction writes close_date_provisional and the ordinary update shape cannot
// spell that field. Asked only that way, every correction reads "cannot be
// undone" — which is the hardcoded false this replaced, arrived at by a longer
// route.
func TestTheReceiptOffersUndoOnARealCorrection(t *testing.T) {
	e := setupCloseDate(t)
	e.seedSweepDeal(t, "Corrected overnight", e.early, nil, intp(-12), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	since := time.Now().Add(-time.Hour)
	receipt, err := e.magicReceipt(t, since)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	line, ok := receiptLineForDeal(receipt, "expected_close_date")
	if !ok {
		t.Fatal("the correction the sweep made is not on the receipt's done lane")
	}
	if line.Undo == nil || !line.Undo.Undoable {
		reason := "<nil>"
		if line.Undo != nil && line.Undo.Reason != nil {
			reason = *line.Undo.Reason
		}
		t.Fatalf("the correction reads not-undoable (%s) — the receipt offers no way back", reason)
	}
	if line.Undo.AuditId == nil {
		t.Error("an undoable line carries no audit id for the control to name")
	}

	// And once taken back, the same line stops offering it: a second Undo on a
	// reversed correction is a control that can only refuse.
	if _, err := e.Deals.RevertCorrection(e.Admin(), e.correctionFor(t, e.firstDealID(t)), nil, nil); err != nil {
		t.Fatal(err)
	}
	after, err := e.magicReceipt(t, since)
	if err != nil {
		t.Fatal(err)
	}
	reversed, ok := receiptLineForDeal(after, "expected_close_date")
	if !ok {
		t.Fatal("the correction left the receipt after being undone")
	}
	if reversed.Undo == nil || reversed.Undo.Undoable {
		t.Error("a correction already taken back still offers an Undo")
	}
}

// The write-side half of TestTheReceiptOffersUndoOnARealCorrection: the
// receipt's AuditId is what a real client's Undo control sends to the
// generic restore ROUTE, never to RevertCorrection directly, so this is the
// path that must actually put the deal back. Before this path knew about
// corrections, the receipt read a line as undoable and pressing Undo
// refused it every time — the generic evaluator refuses exactly what a
// correction writes.
func TestTheGenericRestoreRouteHonorsACorrectionTheReceiptOffersUndoOn(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Corrected overnight, undone through the real route", e.early, nil, intp(-12), 3)
	wasClosing := today().AddDate(0, 0, -12)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	auditID := e.correctionAuditIDFor(t, deal)

	seam := NewRestoreSeam(e.Pool, NewDispatcher(NewProvider(e.Pool),
		NewOverlayProvider(e.Pool, failClosedOverlayMeter(), nil), e.Pool),
		deals.NewStore(e.DB(), DealsInstallation()))
	entry, err := seam.Restore(e.Admin(), "deal", deal, auditID, currentVersion(t, e.Env, "deal", deal))
	if err != nil {
		t.Fatalf("putting a correction back through the generic restore route: %v — the receipt "+
			"said this line was undoable, and pressing Undo must not disagree", err)
	}
	if entry.UndidAuditLogID == nil || *entry.UndidAuditLogID != auditID {
		t.Errorf("the restore entry names %v as undone, want the correction's own audit row %s",
			entry.UndidAuditLogID, auditID)
	}

	restored := e.readSwept(t, deal)
	if restored.expectedClose == nil || !restored.expectedClose.Equal(wasClosing) {
		t.Errorf("restored close date = %v, want the original %s",
			restored.expectedClose, wasClosing.Format(time.DateOnly))
	}

	// A second Undo on the same route now finds the correction already
	// reversed rather than falling into the generic evaluator a second time
	// — and says so with the SAME word and the SAME wire status every other
	// refusal on this route carries, not a 500: a correction's own refusal
	// must be as legible as the generic evaluator's.
	_, err = seam.Restore(e.Admin(), "deal", deal, auditID, currentVersion(t, e.Env, "deal", deal))
	var refused RefusedRestore
	if !errors.As(err, &refused) {
		t.Fatalf("second restore of an already-reversed correction = %v, want a RefusedRestore", err)
	}
	if refused.Reason != ReasonAlreadyUndone {
		t.Errorf("second restore's reason = %q, want %q", refused.Reason, ReasonAlreadyUndone)
	}
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusConflict {
		t.Errorf("second restore classifies as %+v (ok=%v), want a 409", fault, ok)
	}
}

// TestTheGenericRestoreRouteRefusesACorrectionOverwrittenByALaterEdit is the
// write-side half of TestATakenBackCorrectionRefusesToOverwriteALaterEdit,
// reached through the SAME route the previous test drives — the refusal
// RevertCorrection returns for its own reasons (a later field edit, as
// opposed to the "already reversed" case the seam catches before ever
// calling it) must reach the caller as the same 409 shape too, not the raw
// *deals.CorrectionReversalError a 500 would fall through to.
func TestTheGenericRestoreRouteRefusesACorrectionOverwrittenByALaterEdit(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Corrected then re-dated, undone through the real route", e.early, nil, intp(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	auditID := e.correctionAuditIDFor(t, deal)

	// The rep puts their own date on it — a real answer, not the machine's —
	// so RevertCorrection's own per-field conflict check refuses, not the
	// early "already reversed" shortcut.
	chosen := today().AddDate(0, 0, 45)
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		ExpectedClose: &chosen,
	}); err != nil {
		t.Fatal(err)
	}

	seam := NewRestoreSeam(e.Pool, NewDispatcher(NewProvider(e.Pool),
		NewOverlayProvider(e.Pool, failClosedOverlayMeter(), nil), e.Pool),
		deals.NewStore(e.DB(), DealsInstallation()))
	_, err := seam.Restore(e.Admin(), "deal", deal, auditID, currentVersion(t, e.Env, "deal", deal))
	var refused RefusedRestore
	if !errors.As(err, &refused) {
		t.Fatalf("restore over a later human edit = %v, want a RefusedRestore", err)
	}
	if refused.Reason != ReasonNotRestorableByThisPath {
		t.Errorf("reason = %q, want %q", refused.Reason, ReasonNotRestorableByThisPath)
	}
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusConflict {
		t.Errorf("classifies as %+v (ok=%v), want a 409", fault, ok)
	}

	kept := e.readSwept(t, deal)
	if kept.expectedClose == nil || !kept.expectedClose.Equal(chosen) {
		t.Errorf("the rep's own date was overwritten: %v", kept.expectedClose)
	}
}

// TestTheGenericRestoreRouteRefusesAStaleVersionOnACorrection — the per-field
// conflict check above catches a later edit to a CORRECTED field by VALUE,
// which is exactly what lets an unrelated rename through deliberately. It is
// not a substitute for ifVersion: a caller's If-Match names the record they
// looked at, not any one field, and a stale one must refuse regardless of
// which field moved under it.
func TestTheGenericRestoreRouteRefusesAStaleVersionOnACorrection(t *testing.T) {
	e := setupCloseDate(t)
	deal := e.seedSweepDeal(t, "Corrected, then renamed by someone else", e.early, nil, intp(-12), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	auditID := e.correctionAuditIDFor(t, deal)
	stale := currentVersion(t, e.Env, "deal", deal)

	// An edit to a field the correction never touched — the shape the
	// per-field check deliberately lets an undo through over. The version
	// pin does not make that distinction: it moved, so a caller holding the
	// version from before it must be told so.
	renamed := "Renamed after the correction"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		Name: &renamed,
	}); err != nil {
		t.Fatal(err)
	}

	seam := NewRestoreSeam(e.Pool, NewDispatcher(NewProvider(e.Pool),
		NewOverlayProvider(e.Pool, failClosedOverlayMeter(), nil), e.Pool),
		deals.NewStore(e.DB(), DealsInstallation()))
	_, err := seam.Restore(e.Admin(), "deal", deal, auditID, stale)
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("restore with a stale If-Match = %v, want ErrVersionSkew", err)
	}
}

// receiptLineForDeal finds the done line whose change touched one field.
func receiptLineForDeal(receipt crmcontracts.MagicReceipt, field string) (crmcontracts.MagicLine, bool) {
	for _, line := range receipt.Done {
		if line.After == nil {
			continue
		}
		if _, ok := (*line.After)[field]; ok {
			return line, true
		}
	}
	return crmcontracts.MagicLine{}, false
}
