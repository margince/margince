// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChangeBusinessVisibility sets who may read the messages of an approved company
// business account. It runs the posture change a shared mailbox's PATCH makes, on
// the account's chat seat, so both doors share one rule. The visibility is held
// or shared, and `shared` needs the workspace opt-in. The admin confirms.
//
// A link with no seat is a conflict, since approval makes the seat. A member's
// own account has no team seat, so it is not found here.
func (s *ChannelStore) ChangeBusinessVisibility(ctx context.Context, id ids.UUID, posture string, confirmed bool) (BusinessConnection, error) {
	if err := requireBusinessAdmin(ctx, principal.ActionUpdate); err != nil {
		return BusinessConnection{}, err
	}
	if !confirmed {
		return BusinessConnection{}, &SharedChangeError{}
	}
	if err := refuseSharedVisibility(posture); err != nil {
		return BusinessConnection{}, err
	}
	var out BusinessConnection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		link, _, err := lockBusinessConnection(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := changeSeatPostureTx(ctx, tx, link.SeatID, posture); err != nil {
			return err
		}
		out, _, err = lockBusinessConnection(ctx, tx, id)
		return err
	})
	if err != nil {
		return BusinessConnection{}, businessErr("changing the visibility of", err)
	}
	return out, nil
}

// changeSeatPostureTx moves the chat seat of an approved account to posture, the
// change a shared mailbox's PATCH makes. Both networks' visibility routes call it.
// An account with no seat is a conflict, since approval makes the seat.
func changeSeatPostureTx(ctx context.Context, tx pgx.Tx, seatID *ids.UUID, posture string) error {
	if seatID == nil {
		return fmt.Errorf("approve this business connection before changing its visibility: %w", apperrors.ErrConflict)
	}
	seat, err := scanSharedMailbox(tx.QueryRow(ctx, `SELECT `+sharedMailboxColumns+` FROM capture_connection
	  WHERE id = $1 AND user_id IS NULL FOR UPDATE`, *seatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if seat.Posture == posture {
		return nil
	}
	_, err = changeSharedPostureTx(ctx, tx, seat, posture)
	return err
}
