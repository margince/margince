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

// reconcilePhonePlacements splits a whole-list phone replace into row-level work:
// each submitted row consumes ONE held row of its number in FIFO order (an
// update to that row's id); a submitted row with no held row left is fresh; a
// held row nothing consumed is archived. Two rows of one number keep their own
// identities because they consume two distinct held ids.
func reconcilePhonePlacements(
	held map[string][]ids.UUID,
	submitted []PersonPhoneInput,
) (updates []phonePlacement, fresh []PersonPhoneInput, archive []ids.UUID) {
	remaining := make(map[string][]ids.UUID, len(held))
	for number, rowIDs := range held {
		remaining[number] = append([]ids.UUID(nil), rowIDs...)
	}
	for _, row := range submitted {
		rowIDs := remaining[row.Phone]
		if len(rowIDs) > 0 {
			updates = append(updates, phonePlacement{id: rowIDs[0], row: row})
			remaining[row.Phone] = rowIDs[1:]
		} else {
			fresh = append(fresh, row)
		}
	}
	for _, rowIDs := range remaining {
		archive = append(archive, rowIDs...)
	}
	return updates, fresh, archive
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

	held, err := heldPhoneIDs(ctx, tx, personID)
	if err != nil {
		return err
	}
	updates, fresh, archive := reconcilePhonePlacements(held, phones)

	// archived_at IS NULL makes this an absolute idempotent transition: a row a
	// concurrent write already archived converges on the same archived_at rather
	// than raced, so a lost race here is not a conflict.
	for _, id := range archive {
		if _, err := tx.Exec(ctx,
			`UPDATE person_phone SET archived_at = now() WHERE id = $1 AND archived_at IS NULL`, id); err != nil {
			return fmt.Errorf("archive person phone: %w", err)
		}
	}
	// Demotions before promotions: a swap of which same-type number is primary
	// travels here with both rows retained, and promoting one before demoting the
	// other is two live primaries of one type until the statement ends, which
	// uq_person_phone_primary refuses.
	if err := placePhones(ctx, tx, updates, false); err != nil {
		return err
	}
	if err := placePhones(ctx, tx, updates, true); err != nil {
		return err
	}
	return insertPersonPhones(ctx, tx, personID, source, by, fresh)
}

// placePhones applies the retained-row placements whose is_primary matches
// `primary`, each keyed on the held row's own id so two rows of one number no
// longer overwrite each other.
//
// archived_at IS NULL refuses a row a concurrent write archived out from under
// this one, and RowsAffected turns that refusal into a CAS: a placement that
// matches zero rows lost that race rather than silently doing nothing to a
// number the held-set said was still live.
func placePhones(ctx context.Context, tx pgx.Tx, updates []phonePlacement, primary bool) error {
	for _, u := range updates {
		if u.row.IsPrimary != primary {
			continue
		}
		tag, err := tx.Exec(ctx,
			`UPDATE person_phone SET phone_type = $2, is_primary = $3, position = $4
			   WHERE id = $1 AND archived_at IS NULL`,
			u.id, u.row.PhoneType, u.row.IsPrimary, u.row.Position)
		if err != nil {
			if _, ok := storekit.UniqueViolation(err); ok {
				return apperrors.ErrConflict
			}
			return fmt.Errorf("update person phone placement: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrConflict
		}
	}
	return nil
}

// heldPhoneIDs answers the ids of a person's live numbers, grouped by the stored
// E.164 form and ordered so the same held row is consumed first each time. A
// number can name several live rows (no dedupe index — a switchboard reaches
// several people), so the value is a slice, not a single id.
func heldPhoneIDs(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (map[string][]ids.UUID, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, phone FROM person_phone
		   WHERE person_id = $1 AND archived_at IS NULL
		   ORDER BY position, created_at`, personID)
	if err != nil {
		return nil, fmt.Errorf("read person phones: %w", err)
	}
	defer rows.Close()
	held := map[string][]ids.UUID{}
	for rows.Next() {
		var id ids.UUID
		var phone string
		if err := rows.Scan(&id, &phone); err != nil {
			return nil, fmt.Errorf("scan person phone: %w", err)
		}
		held[phone] = append(held[phone], id)
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
