// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// A contact's addresses, and which of them they are known by.
//
// The second is a DECISION, not a field: a person carries several addresses,
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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReachableEmailOrder is the order a person's live addresses are sent in, and
// the order the ONE address they are reachable at is picked from.
//
// `is_primary DESC` first, then the record's own arrangement. A primary address
// is the one somebody chose; without one, the first the record lists is the one
// a reader would have read first anyway.
//
// Both the `emails` array and `primary_email` come from this, and so does the
// expression the contacts list orders by (person_list.go) — so the address a
// reader sees, the address a page is arranged by, and the address a merge
// screen names a person with are one string rather than three derivations of
// the same question.
// Exported because compose reads it too: the account page's contact cards pick
// an address off the same table, and a second spelling of this ORDER BY is a
// second answer to which address a person is known by.
const ReachableEmailOrder = ` ORDER BY is_primary DESC, position, created_at`

func attachPersonEmails(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Person, personIDs []ids.UUID) error {
	rows, err := tx.Query(ctx,
		`SELECT person_id, id, email, email_type, is_primary, position, source, captured_by
		 FROM person_email WHERE person_id = ANY($1) AND archived_at IS NULL`+
			ReachableEmailOrder, personIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var personID, emailID ids.UUID
		var e crmcontracts.PersonEmail
		var email string
		if err := rows.Scan(&personID, &emailID, &email, &e.EmailType, &e.IsPrimary, &e.Position, &e.Source, &e.CapturedBy); err != nil {
			return err
		}
		e.Id = openapi_types.UUID(emailID)
		e.Email = openapi_types.Email(email)
		p := idx[openapi_types.UUID(personID)]
		if p.Emails == nil {
			p.Emails = &[]crmcontracts.PersonEmail{}
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
