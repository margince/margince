// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// closeDayOf is a close date a year out from the test clock, day days apart.
func closeDayOf(t *testing.T, day int) string {
	t.Helper()
	return clocktest.Now(t).AddDate(1, 0, day).Format(time.DateOnly)
}

// seedCorrectionOfKind writes one close-date correction of the given kind, on a
// day of its own so every call changes the deal.
func seedCorrectionOfKind(t *testing.T, e *configEnv, deal ids.DealID, kind string, day int) ids.UUID {
	t.Helper()
	var correction ids.UUID
	closes := closeDayOf(t, day)
	err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		ctx := e.asDealWriter()
		patch := storekit.NewPatch()
		patch.Set("expected_close_date", nil, closes)
		if err := applyDealPatchGuarded(ctx, tx, deal, patch, nil); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "deal", deal.UUID, patch.Before(), patch.After())
		if err != nil {
			return err
		}
		recorded, err := recordCorrection(ctx, tx, DealCorrection{
			DealID: deal, AuditLogID: auditID, Correction: kind, Fields: movedFields(patch),
			Evidence: CorrectionEvidence{RemainingOpenStages: "1", Asking: "a", StandingCloseDate: "b"},
		}, nil)
		correction = recorded.ID
		return err
	})
	if err != nil {
		t.Fatalf("seeding a %s correction: %v", kind, err)
	}
	return correction
}

// A change older than the first page of 20 is still found by what it is. A
// deal page never has to page for the change it is about.
func TestADealsOldestChangeIsFoundByKindBeyondTheFirstPage(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	oldest := seedCorrectionOfKind(t, e, deal, "deal_close_date", 0)
	for day := 1; day <= 24; day++ {
		seedCorrectionOfKind(t, e, deal, "auto_apply", day)
	}

	page, more, err := e.store.DealChanges(e.asDealWriter(), deal, nil, 20, DealChangeFilter{})
	if err != nil || !more || len(page) != 20 {
		t.Fatalf("the unfiltered first page holds %d changes, more=%v (%v), want 20 and more", len(page), more, err)
	}
	found, _, err := e.store.DealChanges(e.asDealWriter(), deal, nil, 20, DealChangeFilter{Kinds: []string{"deal_close_date"}})
	var audit ids.UUID
	if err := e.owner.QueryRow(t.Context(), `SELECT audit_log_id FROM deal_correction WHERE id = $1`, oldest).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if err != nil || len(found) != 1 || found[0].AuditLogID != audit {
		t.Fatalf("filtered by kind the list is %+v (%v), want only the oldest change %s", found, err, audit)
	}
}

// colleagueSetsCloseDate is an edit by a colleague: the patch and its audit row, in
// one transaction, the way every deal write leaves them.
func colleagueSetsCloseDate(t *testing.T, e *configEnv, deal ids.DealID, day int) {
	t.Helper()
	err := e.store.Tx(e.asDealWriter(), func(tx pgx.Tx) error {
		ctx := e.asDealWriter()
		patch := storekit.NewPatch()
		patch.Set("expected_close_date", nil, closeDayOf(t, day))
		if err := applyDealPatchGuarded(ctx, tx, deal, patch, nil); err != nil {
			return err
		}
		_, err := storekit.Audit(ctx, tx, "update", "deal", deal.UUID, patch.Before(), patch.After())
		return err
	})
	if err != nil {
		t.Fatalf("someone setting the close date: %v", err)
	}
}

func auditOf(t *testing.T, e *configEnv, correction ids.UUID) ids.UUID {
	t.Helper()
	var audit ids.UUID
	if err := e.owner.QueryRow(t.Context(), `SELECT audit_log_id FROM deal_correction WHERE id = $1`, correction).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	return audit
}

// Someone sets the date away from Margince's and back to it. That is their
// own choice, so Undo must not put the earlier value over it. The values alone
// cannot tell this from an untouched date.
func TestADateEditedAwayAndBackIsNoLongerUndoable(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	correction := seedCorrectionOfKind(t, e, deal, "deal_close_date", 1)
	audit := auditOf(t, e, correction)
	if review, err := e.store.AppliedChangeReview(e.asDealWriter(), deal, audit); err != nil || !review.CanUndo {
		t.Fatalf("before any edit the review is %+v (%v), want Undo open", review, err)
	}

	colleagueSetsCloseDate(t, e, deal, 2)
	colleagueSetsCloseDate(t, e, deal, 1)

	if review, err := e.store.AppliedChangeReview(e.asDealWriter(), deal, audit); err != nil || review.CanUndo {
		t.Fatalf("after the date went away and back the review is %+v (%v), want no Undo", review, err)
	}
	if _, err := e.store.RevertCorrection(e.asDealWriter(), correction, nil, nil); err == nil {
		t.Fatal("Undo put Margince's earlier value over the last editor's choice")
	}
}

// A later write to another field leaves Undo open, whatever text that field
// holds. A name that reads as a number or a bad date is only a name.
func TestALaterWriteToAnotherFieldKeepsUndo(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	correction := seedCorrectionOfKind(t, e, deal, "deal_close_date", 1)
	audit := auditOf(t, e, correction)
	for _, name := range []string{"1", "1.0", "2026-99-99T99:99 A", clocktest.Now(t).Format(time.RFC3339)} {
		if _, err := e.store.UpdateDeal(e.asDealWriter(), deal, UpdateDealInput{Name: &name}); err != nil {
			t.Fatal(err)
		}
		if review, err := e.store.AppliedChangeReview(e.asDealWriter(), deal, audit); err != nil || !review.CanUndo {
			t.Fatalf("with the deal named %q the review is %+v (%v), want Undo open", name, review, err)
		}
	}
	if _, err := e.store.RevertCorrection(e.asDealWriter(), correction, nil, nil); err != nil {
		t.Fatalf("Undo after unrelated edits: %v", err)
	}
}

// A filtered list pages on its cursor like the whole one, so no change of the
// kind asked for is skipped or repeated.
func TestAFilteredChangeListPagesOnItsCursor(t *testing.T) {
	e := setupConfigEnv(t)
	deal, _, _ := seedOwnedDeal(t, e)
	seedCorrectionOfKind(t, e, deal, "deal_close_date", 0)
	for day := 1; day <= 5; day++ {
		seedCorrectionOfKind(t, e, deal, "auto_apply", day)
	}
	only := DealChangeFilter{Kinds: []string{"auto_apply"}}

	first, more, err := e.store.DealChanges(e.asDealWriter(), deal, nil, 3, only)
	if err != nil || !more || len(first) != 3 {
		t.Fatalf("the first filtered page holds %d changes, more=%v (%v), want 3 and more", len(first), more, err)
	}
	last := first[len(first)-1]
	rest, more, err := e.store.DealChanges(e.asDealWriter(), deal, &ChangeCursor{AppliedAt: last.AppliedAt, AuditLogID: last.AuditLogID}, 3, only)
	if err != nil || more || len(rest) != 2 {
		t.Fatalf("the second filtered page holds %d changes, more=%v (%v), want the remaining 2", len(rest), more, err)
	}
	for _, change := range rest {
		if slices.ContainsFunc(first, func(seen CorrectionReceipt) bool { return seen.AuditLogID == change.AuditLogID }) {
			t.Fatalf("change %s came on both pages", change.AuditLogID)
		}
	}
}
