// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// A person's phone rows: landing them, replacing them, and reading which ones
// they currently hold.
//
// Apart from the address rows next door because the two obey different
// schemas. An address is claimed — uq_person_email_dedupe makes one address
// name exactly one person, so writing one is a claim that can be refused with
// a 409. A number is not: a switchboard reaches several people, and there is
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

// phonePlacement pairs a submitted row with the id of the held row it updates,
// so the placement writes WHERE id = $1 rather than WHERE phone = $2 — the value
// match rewrote every live row of a number at once, collapsing two types onto
// the last (there is no dedupe index on a number, so two live rows of one are a
// valid record).
type phonePlacement struct {
	id  ids.UUID
	row PersonPhoneInput
}

// heldPhone is a live phone row the reconciler may consume: its id and the type
// it currently sits under. The type lets a re-sent row reclaim its OWN row
// rather than the first row of that number — the difference between keeping two
// types of one number distinct and collapsing them onto one id, with the wrong
// row's created_at, source, captured_by and observed_at carried into the survivor.
type heldPhone struct {
	id        ids.UUID
	phoneType string
}

// reconcilePhonePlacements splits a whole-list phone replace into row-level work.
// A submitted row first reclaims a held row of the SAME (number, type), so a
// number held under two types keeps each row's id and provenance when the list is
// re-sent unchanged. Only when no same-type row is left does it fall back to
// consuming any remaining held row of that number in FIFO order — which is how a
// type CHANGE keeps the existing row instead of archiving and re-inserting it. A
// submitted row with no held row left is fresh; a held row nothing consumed is
// archived.
func reconcilePhonePlacements(
	held map[string][]heldPhone,
	submitted []PersonPhoneInput,
) (updates []phonePlacement, fresh []PersonPhoneInput, archive []ids.UUID) {
	remaining := make(map[string][]heldPhone, len(held))
	for number, rows := range held {
		remaining[number] = append([]heldPhone(nil), rows...)
	}
	placed := make([]bool, len(submitted))
	for i, row := range submitted {
		if id, ok := consumeHeldPhone(remaining, row.Phone, row.PhoneType); ok {
			updates = append(updates, phonePlacement{id: id, row: row})
			placed[i] = true
		}
	}
	for i, row := range submitted {
		if placed[i] {
			continue
		}
		if id, ok := consumeHeldPhone(remaining, row.Phone, ""); ok {
			updates = append(updates, phonePlacement{id: id, row: row})
		} else {
			fresh = append(fresh, row)
		}
	}
	for _, rows := range remaining {
		for _, h := range rows {
			archive = append(archive, h.id)
		}
	}
	return updates, fresh, archive
}

// consumeHeldPhone removes one held row of `number` from `remaining` and returns
// its id: the first whose type matches `phoneType`, or — when `phoneType` is
// empty — the first of the number regardless of type. Removing it stops a second
// submitted row from reclaiming the same held id.
func consumeHeldPhone(remaining map[string][]heldPhone, number, phoneType string) (ids.UUID, bool) {
	rows := remaining[number]
	for i, h := range rows {
		if phoneType == "" || h.phoneType == phoneType {
			remaining[number] = append(append([]heldPhone(nil), rows[:i]...), rows[i+1:]...)
			return h.id, true
		}
	}
	return ids.UUID{}, false
}

// replacePersonPhones makes the person's LIVE numbers mirror the given set, the
// way replacePersonEmails does for addresses.
//
// Archived rather than deleted, for the same reason: person_phone carries
// archived_at and history, and a hard DELETE would erase the evidence that a
// person ever held a number.
//
// Two differences from the address side, both because the schema differs. There
// is no cross-person dedupe index on a number — a switchboard legitimately
// reaches several people — so there is no claim to check and no 409 to raise.
// uq_person_phone_primary is the same shape as its address twin, so the
// demote-before-insert ordering below is load-bearing in exactly the same way:
// a corrected work number arrives wanting primary while the stored one still
// holds the slot, and inserting first makes two live primaries of one type for
// the length of a statement.
func replacePersonPhones(ctx context.Context, tx pgx.Tx, personID ids.PersonID, source, by string, phones []PersonPhoneInput) error {
	if phones == nil {
		return nil
	}
	// Normalized to E.164 BEFORE anything is compared or written: the held-set
	// below matches on the stored form, and comparing a raw "+49 30 1234" to a
	// stored "+49301234" would re-insert a number the person already holds.
	if err := parsePersonContacts(nil, phones); err != nil {
		return err
	}

	held, err := heldPhones(ctx, tx, personID)
	if err != nil {
		return err
	}
	updates, fresh, archive := reconcilePhonePlacements(held, phones)

	// One statement archives every held row the reconciler did not reclaim, inside
	// the transaction already holding this person's row lock. archived_at IS NULL
	// makes it an absolute idempotent transition: a row a concurrent write already
	// archived converges on the same archived_at rather than racing, so a lost race
	// here is agreement, not a conflict. person_id is where "the same person row"
	// is spelled — every id came from this person's held-set, and the predicate
	// keeps the write from ever reaching past them.
	if len(archive) > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE person_phone SET archived_at = now()
			   WHERE id = ANY($1) AND person_id = $2 AND archived_at IS NULL`,
			archive, personID); err != nil {
			return fmt.Errorf("archive person phones: %w", err)
		}
	}
	// Demotions before promotions: a swap of which same-type number is primary
	// travels here with both rows retained, and promoting one before demoting the
	// other is two live primaries of one type until the statement ends, which
	// uq_person_phone_primary refuses.
	if err := placePhones(ctx, tx, personID, updates, false); err != nil {
		return err
	}
	if err := placePhones(ctx, tx, personID, updates, true); err != nil {
		return err
	}
	return insertPersonPhones(ctx, tx, personID, source, by, fresh)
}

// placePhones applies the retained-row placements whose is_primary matches
// `primary`, each keyed on the held row's own id so two rows of one number no
// longer overwrite each other. person_id pins each write to this person — the id
// already identifies the row, but the predicate is where the reach "on the SAME
// person row" is stated in SQL rather than left to a comment.
//
// archived_at IS NULL refuses a row a concurrent write archived out from under
// this one, and RowsAffected turns that refusal into a CAS: a placement that
// matches zero rows lost that race rather than silently doing nothing to a
// number the held-set said was still live.
func placePhones(ctx context.Context, tx pgx.Tx, personID ids.PersonID, updates []phonePlacement, primary bool) error {
	for _, u := range updates {
		if u.row.IsPrimary != primary {
			continue
		}
		tag, err := tx.Exec(ctx,
			`UPDATE person_phone SET phone_type = $3, is_primary = $4, position = $5
			   WHERE id = $1 AND person_id = $2 AND archived_at IS NULL`,
			u.id, personID, u.row.PhoneType, u.row.IsPrimary, u.row.Position)
		if err != nil {
			if _, ok := storekit.UniqueViolation(err); ok {
				return apperrors.ErrConflict
			}
			return fmt.Errorf("placing person phone: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrConflict
		}
	}
	return nil
}

// heldPhones answers a person's live phone rows, grouped by the stored E.164 form
// and ordered so the same held row is consumed first on every re-send. A number
// can name several live rows (no dedupe index — a switchboard reaches several
// people), so the value is a slice. The order is total: position and created_at
// alone tie for two rows of one number written by one CreatePerson in a single
// transaction — they share the transaction timestamp and carry a client-supplied
// position — so id breaks the tie and keeps consumption stable between saves.
func heldPhones(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (map[string][]heldPhone, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, phone, phone_type FROM person_phone
		   WHERE person_id = $1 AND archived_at IS NULL
		   ORDER BY position, created_at, id`, personID)
	if err != nil {
		return nil, fmt.Errorf("read person phones: %w", err)
	}
	defer rows.Close()
	held := map[string][]heldPhone{}
	for rows.Next() {
		var h heldPhone
		var phone string
		if err := rows.Scan(&h.id, &phone, &h.phoneType); err != nil {
			return nil, fmt.Errorf("scan person phone: %w", err)
		}
		held[phone] = append(held[phone], h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read person phones: %w", err)
	}
	return held, nil
}

// insertPersonPhones lands the person's phone rows.
func insertPersonPhones(ctx context.Context, tx pgx.Tx, personID ids.PersonID, source, by string, phones []PersonPhoneInput) error {
	for _, p := range phones {
		if _, err := tx.Exec(ctx,
			`INSERT INTO person_phone (person_id, phone, phone_type, is_primary, position, source, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			personID, p.Phone, p.PhoneType, p.IsPrimary, p.Position, source, by); err != nil {
			return fmt.Errorf("insert person phone: %w", err)
		}
	}
	return nil
}
