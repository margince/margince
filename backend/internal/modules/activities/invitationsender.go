// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditFieldInvitationSender names the seat a write stamped as an invitation's
// sender, in the audit images.
const auditFieldInvitationSender = "invitation_sender"

// StampInvitationSenderTx names seat as the sender of a received invitation
// stored without one. The caller holds the row and decided the seat by
// capture's rule. A row that already names a sender keeps it, and an archived
// row is left as it is.
func StampInvitationSenderTx(ctx context.Context, tx pgx.Tx, activityID, seat, runID ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE activity SET invitation_sender = $2
		 WHERE id = $1 AND invitation_sender IS NULL AND archived_at IS NULL`, activityID, seat)
	if err != nil {
		return false, fmt.Errorf("activities: naming the sender of invitation %s: %w", activityID, err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", activityID,
		map[string]any{auditFieldInvitationSender: nil}, map[string]any{auditFieldInvitationSender: seat},
		map[string]any{evidenceDataTaskRun: runID.String()})
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, activityID, crmcontracts.PublicEventActivityUpdated{}); err != nil {
		return false, err
	}
	return true, nil
}
