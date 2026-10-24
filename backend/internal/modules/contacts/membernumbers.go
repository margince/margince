// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The numbers workspace members sign with. A contact's mail quotes them and
// forwards them. A reading of that mail can offer one as the contact's own.

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// signedNumber is a number as a signature prints it: a country prefix, then
// digits and the separators values.ParsePhone drops.
var signedNumber = regexp.MustCompile(`(?:\+|00)\d[\d \t().\-/]{6,}\d`)

// MemberNumbersTx reads the numbers, in E.164, that members' email signatures
// state in their phone field or their text. A member whose login is one of this
// contact's addresses is left out, because this contact is that member.
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

// IsMemberNumber reports whether a number as a signature printed it is one of
// the members' numbers MemberNumbersTx read. The signature writer and the
// signature repair both ask it.
func IsMemberNumber(members map[string]bool, printed string) bool {
	parsed, err := values.ParsePhone(printed)
	return err == nil && members[parsed.String()]
}

// withoutMemberNumbers drops the numbers a signature read that a member signs
// with. Such a number in a contact's mail is most often our own quoted
// signature in a quote shape textlang.SenderLines misses. Cost: a switchboard
// a member shares is never learned from a signature.
func withoutMemberNumbers(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fields []SignatureField) ([]SignatureField, error) {
	members, err := MemberNumbersTx(ctx, tx, contactID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(slices.Clone(fields), func(f SignatureField) bool { return IsMemberNumber(members, f.Value) }), nil
}
