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

// personalMailResponsibilitySQL keeps the inbound mail this seat answers for:
// the mail addressed to it and to no other seat (soleDirectSeatSQL).
func personalMailResponsibilitySQL(owner *ids.UUID, identities string, arg func(any) int) string {
	if owner == nil {
		return scopeUnbounded
	}
	return fmt.Sprintf("(a.kind = 'email' AND %s = $%d::uuid)", soleDirectSeatSQL(identities), arg(*owner))
}

// soleDirectSeatSQL decides who answers an inbound mail `a`. It is the seat in
// its To line when it is the only seat there, else nobody. Two
// addressed seats make no automatic duty for either, and no shared one. The
// waiting queue and the reminder writer both read it.
func soleDirectSeatSQL(identities string) string {
	seats := directMailSeatsSQL(identities)
	return "(CASE WHEN cardinality(" + seats + ") = 1 THEN (" + seats + ")[1] END)"
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

// ambiguouslyAddressed is a mail addressed to more than one seat. It stays on
// its record and is nobody's waiting work: soleDirectSeatSQL names no one.
func ambiguouslyAddressed(row WaitingReply) bool {
	return row.Kind == kindEmail && len(row.DirectRecipients) > 1
}
