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

// personalMailResponsibilitySQL keeps the inbound mail this seat answers for:
// the mail addressed to it and to no other seat (soleDirectSeatSQL). With no
// seat named it keeps every mail somebody answers for, so mail nobody owns
// never becomes shared or unassigned work. It runs before WaitingScanCap.
//
// A named seat's read starts from the mail its To line names, which holds
// every mail the seat answers for. The scan then judges that share only.
func personalMailResponsibilitySQL(owner *ids.UUID, identities string, arg func(any) int) string {
	if owner == nil {
		return "(a.kind <> 'email' OR " + soleDirectSeatSQL(identities) + " IS NOT NULL)"
	}
	seat := fmt.Sprintf("$%d::uuid", arg(*owner))
	return fmt.Sprintf("(a.kind = 'email' AND a.id = ANY (%s) AND %s = %s)",
		storekit.DirectMailActivitiesSQL(identities, seat), soleDirectSeatSQL(identities), seat)
}

// mailSeats is whose mail one waiting scan reads before WaitingScanCap. It is
// one seat's own, or the mail one of a set of seats answers for. The zero
// value reads every mail somebody answers for.
type mailSeats struct {
	owner      *ids.UUID
	answeredBy []ids.UUID
	bySeats    bool
}

// responsibilitySQL keeps a set's share of the unnarrowed read. Mail that no
// seat or several seats answer for is nobody's, so no set reaches it.
func (m mailSeats) responsibilitySQL(identities string, arg func(any) int) string {
	if m.owner != nil || !m.bySeats {
		return personalMailResponsibilitySQL(m.owner, identities, arg)
	}
	return fmt.Sprintf("(a.kind <> 'email' OR %s = ANY($%d::uuid[]))", soleDirectSeatSQL(identities), arg(m.answeredBy))
}

// soleDirectSeatSQL decides who answers an inbound mail `a`
// (storekit.SoleDirectMailSeatSQL). The waiting queue and the reminder writer
// both read it.
func soleDirectSeatSQL(identities string) string {
	return storekit.SoleDirectMailSeatSQL("a", identities)
}

func mailSeatAddressesSQL(identities, seat string) string {
	return storekit.MailSeatAddressesSQL(identities, seat)
}

func directMailSeatsSQL(identities string) string {
	return storekit.DirectMailSeatsSQL("a", identities)
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
