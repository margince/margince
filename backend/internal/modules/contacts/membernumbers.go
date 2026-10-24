// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The numbers workspace members sign with. A contact's mail quotes them,
// forwards them and lists them beside a colleague's name, so a reading of that
// mail can offer one as the contact's own whatever window it was read from.

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// signedNumber is a number as a signature prints it: a country prefix, then
// digits and the separators values.ParsePhone drops.
var signedNumber = regexp.MustCompile(`(?:\+|00)\d[\d \t().\-/]{6,}\d`)

// MemberNumbersTx reads the numbers, in E.164, that members' email signatures
// state in their phone field or their text. A member this contact IS (one of the
// contact's addresses is the member's login) is left out: their number is
// theirs to have on their own record.
//
// A removed signature still counts. The number was a colleague's, and that
// does not make it the contact's.
func MemberNumbersTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.phone, s.body FROM email_signature s
		  JOIN app_user u ON u.id = s.owner_id
		 WHERE NOT EXISTS (
		   SELECT 1 FROM contact_email ce
		    WHERE ce.contact_id = $1 AND ce.archived_at IS NULL AND lower(ce.email) = lower(u.email))`,
		contactID)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the numbers members sign with: %w", err)
	}
	defer rows.Close()
	numbers := map[string]bool{}
	for rows.Next() {
		var phone, body string
		if err := rows.Scan(&phone, &body); err != nil {
			return nil, fmt.Errorf("contacts: reading the numbers members sign with: %w", err)
		}
		for _, printed := range append(signedNumber.FindAllString(body, -1), phone) {
			if parsed, err := values.ParsePhone(printed); err == nil {
				numbers[parsed.String()] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading the numbers members sign with: %w", err)
	}
	return numbers, nil
}
