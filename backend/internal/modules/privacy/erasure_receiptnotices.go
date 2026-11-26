// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/noticekeys"
)

// eraseReceiptNotices takes back every missed-receipt notice about the erased
// subject or its leads, and detaches it from the record it named.
//
// The notice names the subject by its target. Its dedupe key is the receipt's
// message id, a threading identity the scrubbed delivery keeps as well.
func eraseReceiptNotices(ctx context.Context, tx pgx.Tx, subject ids.ContactID, leads []ids.UUID, reason string) error {
	return retractReceiptNotices(ctx, tx, subject.UUID, leads, reason, causeContactErasure)
}

// retractReceiptNotices is eraseReceiptNotices for any act that anonymizes a
// subject. A zero contact names only the leads.
func retractReceiptNotices(ctx context.Context, tx pgx.Tx, contact ids.UUID, leads []ids.UUID, reason, cause string) error {
	if leads == nil {
		leads = []ids.UUID{}
	}
	rows, err := tx.Query(ctx, `UPDATE notice
 SET system_text = NULL, target_type = NULL, target_id = NULL, retracted_at = coalesce(retracted_at, now())
 WHERE kind = @notice_kind
   AND ((target_type = 'contact' AND target_id = @erased_contact) OR (target_type = 'lead' AND target_id = ANY(@erased_leads)))
 RETURNING id`, pgx.StrictNamedArgs{"notice_kind": noticekeys.OptOutReceiptMissedKind, "erased_contact": contact, "erased_leads": leads})
	if err != nil {
		return fmt.Errorf("privacy: retracting missed-receipt notices: %w", err)
	}
	cleared, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	return tombstoneCollateralScrubs(ctx, tx, "notice", cleared, reason, cause)
}
