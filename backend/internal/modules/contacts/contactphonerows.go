// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A contact's phone rows: landing them, replacing them, and reading which ones
// they currently hold.
//
// Apart from the address rows next door because the two obey different
// schemas. An address is claimed — uq_contact_email_dedupe makes one address
// name exactly one contact, so writing one is a claim that can be refused with
// a 409. A number is not: a switchboard reaches several contacts, and there is
// no dedupe index to violate. What the two DO share is the per-type primary
// slot, and each file spells that ordering where its own write can see it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// replaceContactPhones makes the contact's LIVE numbers mirror the given set, the
// way replaceContactEmails does for addresses.
//
// Archived rather than deleted, for the same reason: contact_phone carries
// archived_at and history, and a hard DELETE would erase the evidence that a
// contact ever held a number.
//
// Two differences from the address side, both because the schema differs. There
// is no cross-contact dedupe index on a number — a switchboard legitimately
// reaches several contacts — so there is no claim to check and no 409 to raise.
// uq_contact_phone_primary is the same shape as its address twin, so the
// demote-before-insert ordering below is load-bearing in exactly the same way:
// a corrected work number arrives wanting primary while the stored one still
// holds the slot, and inserting first makes two live primaries of one type for
// the length of a statement.
//
//nolint:cyclop // the rename added no branch: this body is what it was under the old noun.
func replaceContactPhones(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, source, by string, phones []ContactPhoneInput) error {
	if phones == nil {
		return nil
	}
	// Normalized to E.164 BEFORE anything is compared or written: the held-set
	// below matches on the stored form, and comparing a raw "+49 30 1234" to a
	// stored "+49301234" would re-insert a number the contact already holds.
	if err := parseContactContacts(nil, phones); err != nil {
		return err
	}

	held, err := liveContactPhones(ctx, tx, contactID)
	if err != nil {
		return err
	}

	keep := make([]string, 0, len(phones))
	fresh := make([]ContactPhoneInput, 0, len(phones))
	for _, p := range phones {
		keep = append(keep, p.Phone)
		if !held[p.Phone] {
			fresh = append(fresh, p)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE contact_phone SET archived_at = now()
		  WHERE contact_id = $1 AND archived_at IS NULL AND phone <> ALL($2)`,
		contactID, keep); err != nil {
		return fmt.Errorf("archive contact phones: %w", err)
	}
	// Retained rows are re-placed before the new numbers land, and demotions
	// before promotions within that — see the ordering note above. A swap of
	// which of two same-type numbers is primary travels this loop with both rows
	// retained, so promoting one before demoting the other is two live primaries
	// of one type until the statement ends, which uq_contact_phone_primary refuses.
	place := func(p ContactPhoneInput) error {
		if _, err := tx.Exec(ctx,
			`UPDATE contact_phone SET phone_type = $3, is_primary = $4, position = $5
			  WHERE contact_id = $1 AND phone = $2 AND archived_at IS NULL`,
			contactID, p.Phone, p.PhoneType, p.IsPrimary, p.Position); err != nil {
			if _, ok := storekit.UniqueViolation(err); ok {
				return apperrors.ErrConflict
			}
			return fmt.Errorf("update contact phone placement: %w", err)
		}
		return nil
	}
	for _, p := range phones {
		if held[p.Phone] && !p.IsPrimary {
			if err := place(p); err != nil {
				return err
			}
		}
	}
	for _, p := range phones {
		if held[p.Phone] && p.IsPrimary {
			if err := place(p); err != nil {
				return err
			}
		}
	}
	return insertContactPhones(ctx, tx, contactID, source, by, fresh)
}

// liveContactPhones answers the numbers a contact currently holds, in the stored
// E.164 form. Archived rows are excluded: they are history, and re-inserting
// one would collide with nothing while telling the caller it did.
func liveContactPhones(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (map[string]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT phone FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL`, contactID)
	if err != nil {
		return nil, fmt.Errorf("read contact phones: %w", err)
	}
	defer rows.Close()
	held := map[string]bool{}
	for rows.Next() {
		var phone string
		if err := rows.Scan(&phone); err != nil {
			return nil, fmt.Errorf("scan contact phone: %w", err)
		}
		held[phone] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read contact phones: %w", err)
	}
	return held, nil
}

// insertContactPhones lands the contact's phone rows.
func insertContactPhones(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, source, by string, phones []ContactPhoneInput) error {
	for _, p := range phones {
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_phone (contact_id, phone, phone_type, is_primary, position, source, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			contactID, p.Phone, p.PhoneType, p.IsPrimary, p.Position, source, by); err != nil {
			return fmt.Errorf("insert contact phone: %w", err)
		}
	}
	return nil
}
