// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A late receipt keeps only the provider identity needed to undo delivery.
func (s *Store) finishErasedInvitation(ctx context.Context, tx pgx.Tx, row, claimed invitationRow, receipt *connector.CalendarReceipt, deliveryErr error) error {
	complete := claimed.Status == invitationErased && deliveryErr == nil
	if receipt != nil {
		row.Receipt = &connector.CalendarReceipt{EventID: receipt.EventID}
	}
	args := schedulingArgs{}
	_, err := tx.Exec(ctx, `UPDATE meeting_invitation SET receipt=`+args.add(row.Receipt)+`,cleanup_complete=`+args.add(complete)+`,lease_until=NULL,next_attempt_at=`+args.add(s.now().Add(time.Minute))+`,updated_at=now() WHERE activity_id=`+args.add(row.ID), args...)
	return err
}
