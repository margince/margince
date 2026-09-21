// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A contact's addresses, and which of them they are known by.
//
// The second is a DECISION, not a field: a contact carries several addresses,
// each with a position and a primary flag, some retired. The decision is the
// ORDER — live, primary first, then the record's own arrangement — so whoever
// spells that order has answered it. It is spelled here, once, and the row
// read, the list's sort, the merge screen and the account page's contact cards
// all read it rather than answering again.

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/contactaddress"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReachableEmailOrder is this module's name for contactaddress.ReachableOrder,
// kept because compose and four files here already read it under this name.
//
// The order itself moved to shared/kernel when activities and consent turned
// out to be asking the same question and unable to reach it: a module never
// imports a sibling, so each had spelled its own, and the spellings disagreed.
const ReachableEmailOrder = contactaddress.ReachableOrder

func attachContactEmails(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Contact, contactIDs []ids.UUID) error {
	rows, err := tx.Query(ctx,
		`SELECT contact_id, id, email, email_type, is_primary, position, source, captured_by
		 FROM contact_email WHERE contact_id = ANY($1) AND archived_at IS NULL`+
			ReachableEmailOrder, contactIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID, emailID ids.UUID
		var e crmcontracts.ContactEmail
		var email string
		if err := rows.Scan(&contactID, &emailID, &email, &e.EmailType, &e.IsPrimary, &e.Position, &e.Source, &e.CapturedBy); err != nil {
			return err
		}
		e.Id = openapi_types.UUID(emailID)
		e.Email = openapi_types.Email(email)
		p := idx[openapi_types.UUID(contactID)]
		if p.Emails == nil {
			p.Emails = &[]crmcontracts.ContactEmail{}
		}
		// The FIRST row is the one they are reachable at, because the order
		// above is the choosing rule. Nothing decides a second time.
		if p.PrimaryEmail == nil {
			reachable := e.Email
			p.PrimaryEmail = &reachable
		}
		*p.Emails = append(*p.Emails, e)
	}
	return rows.Err()
}
