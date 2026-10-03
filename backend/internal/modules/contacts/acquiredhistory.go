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
// Every clause is evidence a header cannot forge. The provider filed the
// message as sent (counterparty_outbound_attested); this correspondent is its
// counterparty or on its To or Cc line, as receivedBeforeConnectedTx counts a
// fellow recipient; the seat whose mailbox delivered it (capture_import) is the
// sender; and the message is dated before that seat's FIRST connection. The Date header is the
// seat's own mail client's. A seat with no connection compares against NULL
// and matches nothing, which keeps the duty owed.
//
// The settling migration
// (1790844232_mail_older_than_its_mailbox_connection_owes_no_notice) spelled
// the counterparty half in SQL, because a shipped migration cannot call Go; the
// data fix 2026-10-03_mail_sent_from_a_former_address_owes_no_notice spells the
// To and Cc half for mail sent from a seat's other address.
func writtenToBeforeConnectedTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM activity a
		    JOIN capture_import ci ON ci.activity_id = a.id
		    JOIN activity_participant s
		      ON s.activity_id = a.id AND s.role = 'from' AND s.user_id = ci.user_id
		   WHERE (a.counterparty_email = lower(trim($1))
		          OR EXISTS (SELECT 1 FROM activity_participant p
		                      WHERE p.activity_id = a.id AND p.role IN ('to', 'cc')
		                        AND p.address = lower(trim($1))))
		     AND a.kind = 'email' AND a.direction = 'outbound'
		     AND a.counterparty_outbound_attested AND a.archived_at IS NULL
		     AND a.occurred_at < (SELECT min(cc.created_at) FROM capture_connection cc
		                           WHERE cc.user_id = ci.user_id))`, email).Scan(&held); err != nil {
		return false, fmt.Errorf("contacts: did we write to this address before the mailbox was connected: %w", err)
	}
	return held, nil
}

// receivedBeforeConnectedTx reports that this address was on the To or Cc line
// of mail a seat's mailbox already held before that seat connected it.
//
// The reason is the one writtenToBeforeConnectedTx gives. Somebody a customer
// copied on a thread years ago, or a fellow recipient of the same message, sat
// in the company's mailbox long before the CRM read it. Reading that mailbox
// is not a new acquisition of them.
//
// The proof is the PROVIDER's arrival time in that seat's mailbox
// (capture_import.provider_received_at: Gmail's internalDate, Graph's
// receivedDateTime), never the Date header, which the sender writes. The import
// row binds it to the mailbox that delivered the message, and it is compared
// with that same seat's FIRST connection. Bulk mail does not count, as it does
// not for a reply: a list copying everybody is not correspondence the company
// held with them. A row with no provider time, or a seat with no connection,
// compares against NULL and matches nothing, which keeps the duty owed.
//
// The one-time data fix
// (migrations/datafix/1790871111_mail_a_mailbox_already_held_owes_no_notice)
// spells the same rule in SQL for mail captured before the column existed.
func receivedBeforeConnectedTx(ctx context.Context, tx pgx.Tx, email string) (bool, error) {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM activity_participant p
		    JOIN activity a ON a.id = p.activity_id
		    JOIN capture_import ci ON ci.activity_id = a.id
		   WHERE p.address = lower(trim($1)) AND p.role IN ('to', 'cc')
		     AND a.kind = 'email' AND a.direction = 'inbound'
		     AND NOT a.bulk_mail_attested AND a.archived_at IS NULL
		     AND ci.provider_received_at < (SELECT min(cc.created_at) FROM capture_connection cc
		                                     WHERE cc.user_id = ci.user_id))`, email).Scan(&held); err != nil {
		return false, fmt.Errorf("contacts: did a mailbox already hold mail to this address: %w", err)
	}
	return held, nil
}
