// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedCorrectionOfKind writes one close-date correction of the given kind, on a
// day of its own so every call changes the deal.
func seedCorrectionOfKind(t *testing.T, e *configEnv, deal ids.DealID, kind string, day int) ids.UUID {
	t.Helper()
	var correction ids.UUID
	closes := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, day).Format(time.DateOnly)
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
