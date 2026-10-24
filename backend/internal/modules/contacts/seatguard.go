// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ErrCounterpartyIsSeat marks an address a workspace seat entered or connected.
// A colleague is never a counterparty, so capture creates no contact for it
// whichever seat's mailbox saw the mail.
var ErrCounterpartyIsSeat = errors.New("contacts: counterparty address belongs to a workspace seat")

// contactIsSeatTx reports whether any live address of the contact is a seat
// address. Every writer of an inferred employment edge asks it, because a
// colleague's employer is the workspace and not the company on their mail.
func contactIsSeatTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID) (bool, error) {
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil || len(seats) == 0 {
		return false, err
	}
	var isSeat bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contact_email
		                WHERE contact_id = $1 AND archived_at IS NULL
		                  AND lower(email) = ANY($2))`, contactID, seats.Sorted()).Scan(&isSeat)
	if err != nil {
		return false, fmt.Errorf("contacts: checking whether a contact is a workspace seat: %w", err)
	}
	return isSeat, nil
}
