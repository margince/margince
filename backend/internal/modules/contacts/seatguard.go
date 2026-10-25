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
	subject := ids.ContactID{UUID: contactID}
	seats, err := seatContactsTx(ctx, tx, []ids.ContactID{subject})
	return seats[subject], err
}

// seatContactsTx is contactIsSeatTx for many contacts at once. It reads the
// seat set under the shared lock of every address it asks about. An alias
// claim in flight either commits first and is seen, or waits for this write.
func seatContactsTx(ctx context.Context, tx pgx.Tx, contactIDs []ids.ContactID) (map[ids.ContactID]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT contact_id, lower(email) FROM contact_email
		 WHERE contact_id = ANY($1) AND archived_at IS NULL`, contactIDs)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading addresses to check against the workspace seats: %w", err)
	}
	type held struct {
		contact ids.ContactID
		address string
	}
	addresses, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (held, error) {
		var one held
		return one, row.Scan(&one.contact, &one.address)
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading addresses to check against the workspace seats: %w", err)
	}
	asked := make([]string, len(addresses))
	for i, one := range addresses {
		asked[i] = one.address
	}
	seats, err := storekit.SeatAddressesFor(ctx, tx, asked...)
	if err != nil {
		return nil, err
	}
	out := map[ids.ContactID]bool{}
	for _, one := range addresses {
		if seats.Covers(one.address, ids.Nil) {
			out[one.contact] = true
		}
	}
	return out, nil
}
