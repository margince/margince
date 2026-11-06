// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"maps"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// correctionWrite is one field correction Margince makes on a locked deal.
type correctionWrite struct {
	deal       ids.DealID
	correction string
	patch      *storekit.Patch
	// auditEvidence goes on the audit row: the Home receipt reads its reason
	// back from there long after the event was consumed.
	auditEvidence map[string]any
	changedFields map[string]any
	evidence      CorrectionEvidence
	runID         *ids.UUID
}

// commitCorrection writes a correction's patch, its audit row, its event and
// its deal_correction row, in the caller's transaction under the deal's row
// lock. The close-date sweep and the nightly verdict both write through it.
func commitCorrection(ctx context.Context, tx pgx.Tx, lock storekit.RowLock, w correctionWrite) error {
	if err := applyDealPatchLocked(ctx, tx, w.patch, lock); err != nil {
		return fmt.Errorf("apply %s patch: %w", w.correction, err)
	}
	auditID, err := storekit.AuditWithEvidence(
		ctx, tx, "update", "deal", w.deal.UUID, w.patch.Before(), w.patch.After(), w.auditEvidence)
	if err != nil {
		return fmt.Errorf("audit %s: %w", w.correction, err)
	}
	changed := maps.Clone(w.changedFields)
	if changed == nil {
		changed = map[string]any{}
	}
	maps.Copy(changed, w.patch.After())
	if err := storekit.EmitEvent(ctx, tx, auditID, w.deal.UUID,
		crmcontracts.PublicEventDealUpdated{ChangedFields: changed}); err != nil {
		return fmt.Errorf("emit %s: %w", w.correction, err)
	}
	// Moved, not After: a reversal that restored a field which never changed
	// would put back a value that is already there.
	_, err = recordCorrection(ctx, tx, DealCorrection{
		DealID: w.deal, AuditLogID: auditID, Correction: w.correction,
		Fields: movedFields(w.patch), Evidence: w.evidence,
	}, w.runID)
	return err
}
