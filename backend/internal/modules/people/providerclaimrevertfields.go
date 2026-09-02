// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// One revert per field a purchase can fill, each asking the same question of
// the record as it stands now: is this still the value we wrote?
//
// Every statement carries that test in its own WHERE clause rather than reading
// first and deciding in Go. The read-then-decide shape has a window in it, and
// the thing that fits in the window is a colleague's edit being thrown away.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// revertOne clears one filled field if it is still the provider's, and reports
// whether it went.
func revertOne(ctx context.Context, tx pgx.Tx, f appliedField) (bool, error) {
	switch f.table {
	case entityPerson:
		return revertColumn(ctx, tx, f)
	case tablePersonSocial:
		return revertSocialHandle(ctx, tx, f)
	case tablePersonEmail, tablePersonPhone:
		return archiveChildRow(ctx, tx, f)
	case tableRelationship:
		return archiveEmploymentEdge(ctx, tx, f)
	default:
		// A table this build cannot take back. Left standing and counted, for
		// the reason revertColumn gives: failing here would abort the action
		// for everybody and leave the claims and the ledger in place too.
		return false, nil
	}
}

// revertColumn clears person.title while it still says what was written.
func revertColumn(ctx context.Context, tx pgx.Tx, f appliedField) (bool, error) {
	if err := auth.EnsureWritable(ctx, tx, entityPerson, f.subject); err != nil {
		return false, err
	}
	if f.field != fieldTitle || f.value == nil {
		// A person column this build cannot take back. It is left standing and
		// counted rather than raised: an error here aborts the whole action for
		// every contact, and the claims and the ledger would never be deleted
		// either — one unrevertible row would brick a privacy control.
		return false, nil
	}
	tag, err := tx.Exec(ctx,
		`UPDATE person SET title = NULL WHERE id = $1 AND title = $2`, f.subject, *f.value)
	if err != nil {
		return false, fmt.Errorf("people: clearing a bought job title: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// revertSocialHandle removes the profile link while it is still the row this
// installation's purchase wrote.
//
// By ROW ID, not by value. person_social carries no source column, and the
// ordinary save path replaces every one of a contact's social rows — so a rep
// who opened the record and pressed save owns a NEW row carrying the same
// handle string. A value test cannot tell that row from ours and would delete
// somebody's own work on the next "delete bought data". An id survives only
// while nobody has saved.
func revertSocialHandle(ctx context.Context, tx pgx.Tx, f appliedField) (bool, error) {
	if err := auth.EnsureWritable(ctx, tx, entityPerson, f.subject); err != nil {
		return false, err
	}
	if f.rowID == nil {
		// Written before the row id was recorded. Deleting on the value alone
		// is what this function exists not to do, so it stays and says so.
		return false, nil
	}
	tag, err := tx.Exec(ctx,
		`DELETE FROM person_social WHERE id = $1 AND person_id = $2`, *f.rowID, f.subject)
	if err != nil {
		return false, fmt.Errorf("people: removing a bought profile link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, touchRevertedPerson(ctx, tx, f.subject)
}

// archiveChildRow archives a bought address or number while it still carries
// the provider as its source.
//
// Archived rather than deleted, which is what every other retirement of these
// rows does: an address that was on the record is part of what the record used
// to say, and the erasure is the one path that removes it outright.
//
// The source check is the "still ours" test. A row a human has since re-typed
// through the ordinary update path carries their source instead, and stays.
func archiveChildRow(ctx context.Context, tx pgx.Tx, f appliedField) (bool, error) {
	if err := auth.EnsureWritable(ctx, tx, entityPerson, f.subject); err != nil {
		return false, err
	}
	if f.rowID == nil {
		return false, fmt.Errorf("people: a bought %s was recorded without its row", f.field)
	}
	statement := `UPDATE person_email SET archived_at = now()
		 WHERE id = $1 AND person_id = $2 AND source = $3 AND archived_at IS NULL`
	if f.table == tablePersonPhone {
		statement = `UPDATE person_phone SET archived_at = now()
		 WHERE id = $1 AND person_id = $2 AND source = $3 AND archived_at IS NULL`
	}
	tag, err := tx.Exec(ctx, statement, *f.rowID, f.subject, f.provider)
	if err != nil {
		return false, fmt.Errorf("people: archiving a bought %s: %w", f.field, err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, touchRevertedPerson(ctx, tx, f.subject)
}

// archiveEmploymentEdge retires an employment a purchase asserted, while it is
// still the purchase's.
//
// The edge is archived rather than deleted for the reason every relationship
// retirement is: who somebody worked for is history, and history is not undone
// by a settings toggle. What the toggle removes is the ASSERTION this
// installation bought — so the row stops being current and stops being read.
func archiveEmploymentEdge(ctx context.Context, tx pgx.Tx, f appliedField) (bool, error) {
	if f.rowID == nil {
		return false, fmt.Errorf("people: a bought employment was recorded without its row")
	}
	tag, err := tx.Exec(ctx, `
		UPDATE relationship
		   SET archived_at = now(), is_current_primary = false
		 WHERE id = $1 AND person_id = $2 AND source = $3 AND archived_at IS NULL`,
		*f.rowID, f.subject, f.provider)
	if err != nil {
		return false, fmt.Errorf("people: retiring a bought employment: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// touchRevertedPerson bumps the aggregate after a child row went, for an
// ARCHIVED contact as readily as a live one.
//
// touchPerson refuses an archived subject, which is right for every writer that
// adds something: archived means frozen. This one removes, and a purchase on
// somebody archived last week is exactly what an admin means to delete — so a
// refusal here would roll the revert back and leave the bought values standing
// while the action reported success.
//
// The caller already holds this row FOR UPDATE, so no second lock is taken.
func touchRevertedPerson(ctx context.Context, tx pgx.Tx, personID ids.UUID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE person SET updated_at = now() WHERE id = $1`, personID); err != nil {
		return fmt.Errorf("people: bumping the contact a revert changed: %w", err)
	}
	return nil
}
