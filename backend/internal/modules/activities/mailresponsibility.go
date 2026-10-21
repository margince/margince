// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Mailbox delivery and record ownership do not establish a duty to reply.
func directMailRecipientSQL(activity, addresses string) string {
	clause := `(EXISTS (SELECT 1 FROM activity_participant recipient
 WHERE recipient.activity_id = a.id AND recipient.role = 'to'
 AND lower(recipient.address) = ANY(ADDRESS_LIST))
 OR (a.source_system = '` + connector.EmailSourceSystem + `' AND a.captured_by LIKE 'connector:%'
 AND a.body LIKE E'From:%\nTo:%\n\n%'
 AND substring(lower(split_part(a.body, E'\n', 2)) from 5) = ANY(ADDRESS_LIST)))`
	return strings.ReplaceAll(strings.ReplaceAll(clause, "a.", activity+"."), "ADDRESS_LIST", addresses)
}

func personalMailResponsibilitySQL(owner *ids.UUID, identities string, arg func(any) int) string {
	if owner == nil {
		return scopeUnbounded
	}
	addresses := mailSeatAddressesSQL(identities, fmt.Sprintf("$%d::uuid", arg(*owner)))
	return "(a.kind <> 'email' OR " + directMailRecipientSQL("a", addresses) + ")"
}

func mailSeatAddressesSQL(identities, seat string) string {
	return "ARRAY(SELECT jsonb_array_elements_text(COALESCE(" + identities + " -> (" + seat + ")::text, '[]'::jsonb)))"
}

func directMailSeatsSQL(identities string) string {
	return "ARRAY(SELECT mail_seat.key::uuid FROM jsonb_each(" + identities + ") mail_seat WHERE " +
		directMailRecipientSQL("a", "ARRAY(SELECT jsonb_array_elements_text(mail_seat.value))") + ")"
}

func (s *Store) mailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	if s.ownDomains != nil {
		return s.ownDomains.MailSeatIdentitiesSQL(ctx, tx, bind)
	}
	return "(SELECT coalesce(jsonb_object_agg(id::text, jsonb_build_array(lower(email))), '{}'::jsonb) FROM app_user WHERE " + workforce.LiveMemberSQL("") + ")", nil
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
	if row.Kind != kindEmail {
		return row
	}
	row.OwnerID = ids.Nil
	row.OwnerKnown = true
	if owner != nil {
		row.OwnerID = *owner
		row.AddressedElsewhere = false
	} else if len(row.DirectRecipients) == 1 {
		row.OwnerID = row.DirectRecipients[0]
	}
	return row
}
