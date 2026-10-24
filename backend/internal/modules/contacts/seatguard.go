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

// ErrCounterpartyIsSeat marks a seat's address. A proven one is refused from
// every seat's mailbox; an alias a seat only typed is refused from that seat's
// own mailbox alone (storekit.SeatAddresses).
var ErrCounterpartyIsSeat = errors.New("contacts: counterparty address belongs to a workspace seat")

// contactIsSeatTx reports whether any live address of the contact is a proven
// seat address. Every writer of an inferred employment edge asks it, because a
// colleague's employer is the workspace and not the company on their mail.
func contactIsSeatTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID) (bool, error) {
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil {
		return false, err
	}
	proven := seats.For(ids.Nil)
	if len(proven) == 0 {
		return false, nil
	}
	var isSeat bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contact_email
		                WHERE contact_id = $1 AND archived_at IS NULL
		                  AND lower(email) = ANY($2))`, contactID, proven).Scan(&isSeat)
	if err != nil {
		return false, fmt.Errorf("contacts: checking whether a contact is a workspace seat: %w", err)
	}
	return isSeat, nil
}
