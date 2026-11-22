// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether a chat screenshot shows the contact or lead it was pasted on. The
// header's name or number is compared with the record's name parts and phones.
// One agreeing part is a match, and nothing read is no warning. Anything else
// shows someone else, and the user confirms it before it files. A deal has no
// single counterparty, so a chat on a deal is never flagged.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// phoneTailDigits is how many trailing digits two numbers must share when one
// of them carries no country prefix. A chat header often prints the national
// form of a number the contact has stored in E.164.
const phoneTailDigits = 8

// OtherChatCounterparty is what a chat reading's header shows when it names
// someone other than the contact or lead the screenshot hangs on, and ""
// otherwise. A file on any other record has nobody to compare with.
func (s *Store) OtherChatCounterparty(ctx context.Context, read ExtractionRead) (string, error) {
	if read.Question != ExtractionQuestionChatCapture || read.Live() {
		return "", nil
	}
	var shows string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		parentType, err := resolveAttachmentParent(ctx, tx, read.AttachmentID, principal.ActionRead)
		if err != nil {
			return err
		}
		parentID, err := attachmentParentID(ctx, tx, read.AttachmentID)
		if err != nil {
			return err
		}
		shows, err = otherChatCounterparty(ctx, tx, parentType, parentID, read)
		return err
	})
	return shows, err
}

func otherChatCounterparty(ctx context.Context, tx pgx.Tx, parentType string, parentID ids.UUID, read ExtractionRead) (string, error) {
	name, phone := chatHeader(read)
	if (name == "" && phone == "") || (parentType != linkEntityContact && parentType != linkEntityLead) {
		return "", nil
	}
	names, phones, err := chatParentIdentity(ctx, tx, parentType, parentID)
	if err != nil {
		return "", err
	}
	if chatShowsContact(name, phone, names, phones) {
		return "", nil
	}
	return strings.Join(slices.DeleteFunc([]string{name, phone}, func(v string) bool { return v == "" }), ", "), nil
}

// chatParentIdentity is the names and phones a chat header is compared with.
// A lead stores one name and at most one phone.
func chatParentIdentity(ctx context.Context, tx pgx.Tx, parentType string, parentID ids.UUID) (names, phones []string, err error) {
	if parentType == linkEntityLead {
		if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
			return nil, nil, err
		}
		var fullName, phone *string
		if err := tx.QueryRow(ctx, `SELECT full_name, phone FROM lead WHERE id = $1`, parentID).Scan(&fullName, &phone); err != nil {
			return nil, nil, fmt.Errorf("activities: reading the lead a chat is filed on: %w", err)
		}
		if phone != nil {
			phones = []string{*phone}
		}
		return []string{deref(fullName)}, phones, nil
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, nil, err
	}
	var fullName, firstName, lastName *string
	if err := tx.QueryRow(ctx, `SELECT full_name, first_name, last_name FROM contact WHERE id = $1`, parentID).
		Scan(&fullName, &firstName, &lastName); err != nil {
		return nil, nil, fmt.Errorf("activities: reading the contact a chat is filed on: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT phone FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL`, parentID)
	if err != nil {
		return nil, nil, fmt.Errorf("activities: reading the contact's phones: %w", err)
	}
	phones, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, fmt.Errorf("activities: reading the contact's phones: %w", err)
	}
	return []string{deref(fullName), deref(firstName), deref(lastName)}, phones, nil
}

// chatHeader is the name and number the reading grounded from the header.
func chatHeader(read ExtractionRead) (name, phone string) {
	for _, f := range read.Fields {
		if f.Omitted {
			continue
		}
		switch f.Field {
		case ChatFieldName:
			name = strings.TrimSpace(f.Value)
		case ChatFieldPhone:
			phone = strings.TrimSpace(f.Value)
		}
	}
	return name, phone
}

// chatShowsContact reports whether the header's name or number agrees with
// the contact. Called only when the header shows at least one of them.
func chatShowsContact(name, phone string, contactNames, contactPhones []string) bool {
	if phone != "" && slices.ContainsFunc(contactPhones, func(stored string) bool { return phonesAgree(phone, stored) }) {
		return true
	}
	if name == "" {
		return false
	}
	known := map[string]bool{}
	for _, n := range contactNames {
		for _, part := range nameParts(n) {
			known[part] = true
		}
	}
	return slices.ContainsFunc(nameParts(name), func(part string) bool { return known[part] })
}

// nameParts are a name's words, folded, of two letters or more: "Dr." and an
// initial agree with too many names to mean anything.
func nameParts(name string) []string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return slices.DeleteFunc(words, func(w string) bool { return utf8.RuneCountInString(w) < 2 })
}

// phonesAgree compares two numbers as E.164 when both carry a country prefix,
// and by their trailing digits when either does not.
func phonesAgree(shown, stored string) bool {
	a, errA := values.ParsePhone(shown)
	b, errB := values.ParsePhone(stored)
	if errA == nil && errB == nil {
		return a.String() == b.String()
	}
	da, db := digitsOf(shown), digitsOf(stored)
	if len(da) < phoneTailDigits || len(db) < phoneTailDigits {
		return false
	}
	return da[len(da)-phoneTailDigits:] == db[len(db)-phoneTailDigits:]
}

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
