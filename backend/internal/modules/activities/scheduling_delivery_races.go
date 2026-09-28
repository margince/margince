// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (s *Store) finishSupersededInvitation(ctx context.Context, tx pgx.Tx, row, claimed invitationRow, receipt *connector.CalendarReceipt) error {
	// Cancel can overtake an in-flight save. Its late receipt must still reach
	// the cancellation worker, even if an earlier lookup found no event.
	if row.Command != invitationCancel || claimed.Command == invitationCancel || receipt == nil {
		return nil
	}
	before := row
	row.Receipt, row.Status = receipt, invitationCanceling
	row.Version++
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE meeting_invitation SET receipt=`+args.add(receipt)+`,status=`+args.add(row.Status)+`,version=`+args.add(row.Version)+`,attempts=0,next_attempt_at=`+args.add(s.now())+`,lease_until=NULL WHERE activity_id=`+args.add(row.ID), args...); err != nil {
		return err
	}
	return auditInvitation(ctx, tx, &before, row)
}

func (s *Store) markInvitationAttention(ctx context.Context, observed invitationRow) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		row, err := readInvitation(ctx, tx, observed.ID, true)
		if err != nil {
			return err
		}
		if row.Version != observed.Version || row.Status != invitationConfirmed {
			return nil
		}
		before := row
		row.Status = invitationNeedsAttention
		row.Version++
		args := schedulingArgs{}
		if _, err := tx.Exec(ctx, `UPDATE meeting_invitation SET status=`+args.add(row.Status)+`,version=`+args.add(row.Version)+`,updated_at=now() WHERE activity_id=`+args.add(row.ID), args...); err != nil {
			return err
		}
		return auditInvitation(ctx, tx, &before, row)
	})
}
