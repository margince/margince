// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// writtenToBeforeConnectedTx reports that a seat mailed this address before it
// connected the mailbox the mail was captured from.
//
// A mailbox import reads years of correspondence the company already held.
// Without this, everybody we wrote to back then is a stranger of unknown
// source, owed an Art. 14 notice whose month ran out years ago.
//
// Only OUTBOUND mail counts. Its Date header was written by our own mail
// client; an inbound Date is the sender's claim, and a backdated message
// copying strangers would otherwise excuse their notices. capture_import names
// the seat whose mailbox delivered the message, and that seat's FIRST
// connection is when the mailbox was attached. A seat with no connection
// compares against NULL and matches nothing, which keeps the duty owed.
//
// The settling migration
// (1790844232_mail_older_than_its_mailbox_connection_owes_no_notice) spells
// the same rule in SQL, because a shipped migration cannot call Go.
func writtenToBeforeConnectedTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM activity_participant p
		    JOIN activity a ON a.id = p.activity_id
		    JOIN capture_import ci ON ci.activity_id = a.id
		   WHERE lower(p.address) = lower($1) AND p.role <> 'from'
		     AND a.kind = 'email' AND a.direction = 'outbound'
		     AND a.archived_at IS NULL
		     AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc
		                           WHERE cc.user_id = ci.user_id))`, email).Scan(&held); err != nil {
		return false, fmt.Errorf("contacts: did we write to this address before the mailbox was connected: %w", err)
	}
	return held, nil
}
