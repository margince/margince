// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Mailbox delivery and record ownership do not establish a duty to reply.
func directMailRecipientSQL(activity, addresses string) string {
	return storekit.DirectMailRecipientSQL(activity, addresses)
}

func personalMailResponsibilitySQL(owner *ids.UUID, identities string, arg func(any) int) string {
	if owner == nil {
		return scopeUnbounded
	}
	addresses := mailSeatAddressesSQL(identities, fmt.Sprintf("$%d::uuid", arg(*owner)))
	return "(a.kind = 'email' AND " + directMailRecipientSQL("a", addresses) + ")"
}

func mailSeatAddressesSQL(identities, seat string) string {
	return storekit.MailSeatAddressesSQL(identities, seat)
}

func directMailSeatsSQL(identities string) string {
	return "ARRAY(SELECT mail_seat.key::uuid FROM jsonb_each(" + identities + ") mail_seat WHERE " +
		directMailRecipientSQL("a", "ARRAY(SELECT jsonb_array_elements_text(mail_seat.value))") + ")"
}

func (s *Store) mailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	if s.ownDomains != nil {
		return s.ownDomains.MailSeatIdentitiesSQL(ctx, tx, bind)
	}
	return storekit.MailSeatIdentitiesSQL(ctx, tx, bind)
}

func scanWaitingRecipients(record pgx.Row) (WaitingReply, error) {
	var recipients []ids.UUID
	row, err := scanWaitingReply(recipientReplyScan{Row: record, recipients: &recipients})
	row.DirectRecipients = recipients
	return row, err
}

type recipientReplyScan struct {
	pgx.Row
	recipients *[]ids.UUID
}

func (r recipientReplyScan) Scan(dest ...any) error {
	return r.Row.Scan(append(dest, r.recipients)...)
}

func waitingReplyForOwner(row WaitingReply, owner *ids.UUID) WaitingReply {
	row.OwnerID = ids.Nil
	row.OwnerKnown = true
	if row.Kind != kindEmail {
		row.DirectRecipients = nil
		return row
	}
	if owner != nil {
		row.OwnerID = *owner
		row.AddressedElsewhere = false
	} else if len(row.DirectRecipients) == 1 {
		row.OwnerID = row.DirectRecipients[0]
	}
	return row
}
